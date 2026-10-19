"""FirePager SDR client.

Messages are written to a local SQLite spool before they are sent.  A message
is removed only after RabbitMQ publisher confirms it, so a broker restart (or
client restart) cannot silently lose a decoded page.
"""

import json
import logging
import os
import signal
import shlex
import select
import sqlite3
import subprocess
import threading
import time
import tempfile
import math
from datetime import datetime, timezone
from urllib.parse import urlsplit, urlunsplit

import pika

try:
    from sdr_calibrate import (CalibrationScheduler, calibrate, calibrate_target,
                               parse_station, target_ppm_candidates, validate_result,
                               SDRDeviceLock, DeviceBusyError)
except ImportError:  # Allows importing this module in minimal non-SDR tooling.
    CalibrationScheduler = calibrate = calibrate_target = parse_station = None
    target_ppm_candidates = validate_result = None
    SDRDeviceLock = DeviceBusyError = None


logging.basicConfig(level=logging.INFO, format="%(asctime)s - %(levelname)s - %(message)s")
logger = logging.getLogger(__name__)
QUEUE = "pager_messages"


def redact_url(value):
    """Remove passwords (and credentials in exception text) from log output."""
    if not isinstance(value, str):
        return value
    try:
        parsed = urlsplit(value)
        if parsed.password is not None:
            host = parsed.hostname or ""
            if parsed.port:
                host += ":" + str(parsed.port)
            user = parsed.username or "user"
            return urlunsplit((parsed.scheme, f"{user}:***@{host}", parsed.path, parsed.query, parsed.fragment))
    except ValueError:
        pass
    # pika's errors sometimes include the original URL without it being a URL
    return value.replace(os.environ.get("RABBITMQ_PASSWORD", "\0"), "***") if os.environ.get("RABBITMQ_PASSWORD") else value


def safe_error(exc):
    text = redact_url(str(exc))
    configured = os.environ.get("RABBITMQ_URL", "")
    try:
        password = urlsplit(configured).password
        if password:
            text = text.replace(password, "***")
    except ValueError:
        pass
    return text


class SQLiteSpool:
    """Small FIFO, crash-safe outbox backed by SQLite."""

    def __init__(self, path):
        self.path = path
        directory = os.path.dirname(os.path.abspath(path))
        os.makedirs(directory, exist_ok=True)
        self.db = sqlite3.connect(path, timeout=30, check_same_thread=False)
        self.db.execute("PRAGMA journal_mode=WAL")
        self.db.execute("PRAGMA synchronous=FULL")
        self.db.execute("CREATE TABLE IF NOT EXISTS spool (id INTEGER PRIMARY KEY AUTOINCREMENT, payload TEXT NOT NULL)")
        self.db.commit()
        self.lock = threading.Lock()

    def enqueue(self, payload):
        with self.lock:
            cursor = self.db.execute("INSERT INTO spool(payload) VALUES (?)", (json.dumps(payload),))
            self.db.commit()
            return cursor.lastrowid

    def first(self):
        with self.lock:
            row = self.db.execute("SELECT id, payload FROM spool ORDER BY id LIMIT 1").fetchone()
        return (row[0], json.loads(row[1])) if row else None

    def acknowledge(self, message_id):
        with self.lock:
            self.db.execute("DELETE FROM spool WHERE id = ?", (message_id,))
            self.db.commit()

    def size(self):
        with self.lock:
            return self.db.execute("SELECT COUNT(*) FROM spool").fetchone()[0]

    def close(self):
        with self.lock:
            self.db.close()


def connect_rabbitmq(stop_event=None):
    rabbit_url = os.environ.get("RABBITMQ_URL", "amqp://guest:guest@firepager-rmq:5672/")
    parameters = pika.URLParameters(rabbit_url)
    while stop_event is None or not stop_event.is_set():
        try:
            logger.info("Connecting to RabbitMQ at %s", redact_url(rabbit_url))
            connection = pika.BlockingConnection(parameters)
            channel = connection.channel()
            channel.queue_declare(queue=QUEUE, durable=True)
            # basic_publish now waits for a broker ack/nack.  Do this after
            # declaring the queue so every connection has identical setup.
            channel.confirm_delivery()
            logger.info("Successfully connected to RabbitMQ")
            return connection, channel
        except Exception as exc:
            logger.error("Waiting for RabbitMQ: %s", safe_error(exc))
            if stop_event is None:
                time.sleep(5)
            elif stop_event.wait(5):
                return None, None
    return None, None


def publish_confirmed(channel, payload):
    result = channel.basic_publish(
        exchange="", routing_key=QUEUE, body=json.dumps(payload),
        properties=pika.BasicProperties(delivery_mode=2, content_type="application/json"),
    )
    # BlockingChannel normally raises NackError/UnroutableError.  Fake and
    # alternate pika channels may report a negative confirmation instead.
    if result is False:
        raise RuntimeError("RabbitMQ publisher confirmation was negative")


def close_connection(connection):
    if connection is not None:
        try:
            if connection.is_open:
                connection.close()
        except Exception as exc:
            logger.debug("RabbitMQ close failed: %s", safe_error(exc))


def terminate_process_group(process, timeout=5):
    """Stop the SDR pipeline and all of its children, without orphaning rtl_fm."""
    if process is None:
        return
    try:
        # Do not use the leader's exit status as an indication that the shell
        # pipeline is gone: rtl_fm/multimon can outlive the shell leader.
        os.killpg(process.pid, signal.SIGTERM)
        if process.poll() is None:
            process.wait(timeout=timeout)
    except subprocess.TimeoutExpired:
        os.killpg(process.pid, signal.SIGKILL)
        if process.poll() is None:
            process.wait(timeout=timeout)
    except ProcessLookupError:
        pass
    finally:
        if process.stdout:
            process.stdout.close()


def start_sdr(frequency, ppm=None, gain=None):
    """Start the receive-only RTL-SDR -> multimon pipeline.

    ``-p`` corrects the dongle's crystal error and ``-g`` makes the gain
    explicit.  Keeping these in the command (rather than applying them in
    the decoder) also makes the effective tuning visible in process listings.
    """
    ppm = os.environ.get("SDR_PPM", "0") if ppm is None else str(ppm)
    gain = os.environ.get("SDR_GAIN", "49") if gain is None else str(gain)
    sample_rate = os.environ.get("SDR_SAMPLE_RATE", "22050")
    command = (
        f"rtl_fm -f {shlex.quote(str(frequency))} -s {shlex.quote(sample_rate)} "
        f"-p {shlex.quote(ppm)} -g {shlex.quote(gain)} | "
        "multimon-ng -a POCSAG512 -a POCSAG1200 -a POCSAG2400 -f alpha -t raw /dev/stdin"
    )
    logger.info("Starting SDR decoder")
    return subprocess.Popen(
        command, shell=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
        text=True, start_new_session=True, bufsize=1,
    )


def calibration_stations(value):
    return [parse_station(item.strip()) for item in value.split(",") if item.strip()]


def environment_float(name, default, minimum=None, maximum=None, environ=None):
    """Read a finite numeric setting, falling back instead of killing the client."""
    raw = (os.environ if environ is None else environ).get(name)
    if raw is None or not raw.strip():
        return default
    try:
        value = float(raw)
    except (TypeError, ValueError):
        logger.warning("Ignoring invalid %s=%r", name, raw)
        return default
    if (not math.isfinite(value) or (minimum is not None and value < minimum)
            or (maximum is not None and value > maximum)):
        logger.warning("Ignoring out-of-range %s=%r", name, raw)
        return default
    return value


def validate_sdr_setting(value, name, default, minimum=None, maximum=None):
    """Return a safe SDR setting while retaining a usable last-known-good value."""
    try:
        number = float(value)
    except (TypeError, ValueError):
        logger.warning("Ignoring invalid %s=%r", name, value)
        return default
    if (not math.isfinite(number) or (minimum is not None and number < minimum)
            or (maximum is not None and number > maximum)):
        logger.warning("Ignoring out-of-range %s=%r", name, value)
        return default
    return str(value)


def calibration_config(environ=None):
    """Parse and validate calibration-related environment settings.

    Keep this separate from ``main`` so startup configuration can be checked
    without opening a broker connection or starting an SDR process.
    """
    environ = os.environ if environ is None else environ
    stations_value = environ.get("SDR_CALIBRATE_STATIONS", "")
    try:
        stations = calibration_stations(stations_value) if stations_value and parse_station else []
        interval = float(environ.get("SDR_CALIBRATE_INTERVAL", "0"))
        jitter = float(environ.get("SDR_CALIBRATE_JITTER", "0"))
        min_stations = int(environ.get("SDR_CALIBRATE_MIN_STATIONS", "1"))
        target_start = float(environ.get("SDR_CALIBRATE_TARGET_PPM_MIN", "-8"))
        target_end = float(environ.get("SDR_CALIBRATE_TARGET_PPM_MAX", "8"))
        target_step = float(environ.get("SDR_CALIBRATE_TARGET_PPM_STEP", "1"))
        target_repeats = int(environ.get("SDR_CALIBRATE_TARGET_REPEATS", "2"))
    except (TypeError, ValueError) as exc:
        raise ValueError("invalid SDR calibration environment configuration") from exc
    if (not math.isfinite(interval) or not math.isfinite(jitter)
            or interval < 0 or jitter < 0 or min_stations < 1):
        raise ValueError("SDR calibration interval and jitter must be non-negative; minimum stations must be positive")
    if target_repeats < 2 or target_step <= 0 or target_end < target_start:
        raise ValueError("invalid target SDR calibration candidate range or repeats")
    try:
        target_candidates = target_ppm_candidates(target_start, target_end, target_step)
    except ValueError as exc:
        raise ValueError("invalid target SDR calibration candidate range") from exc
    return {
        "stations": stations,
        "interval": interval,
        "jitter": jitter,
        "min_stations": min_stations,
        "quiet_period": environment_float(
            "SDR_CALIBRATE_QUIET_PERIOD", 5, minimum=0, environ=environ),
        "on_start": environ.get("SDR_CALIBRATE_ON_START", "false").lower() == "true",
        "target_on_start": environ.get("SDR_CALIBRATE_TARGET_ON_START", "true").lower() == "true",
        "target_candidates": target_candidates,
        "target_repeats": target_repeats,
        "target_min_prominence": environment_float(
            "SDR_CALIBRATE_TARGET_MIN_PROMINENCE", 6, minimum=0, environ=environ),
        "target_min_margin": environment_float(
            "SDR_CALIBRATE_TARGET_MIN_MARGIN", 1, minimum=0, environ=environ),
    }


def load_calibration(path):
    """Read only a complete, previously persisted calibration result."""
    try:
        with open(path, encoding="utf-8") as source:
            value = json.load(source)
        return str(value["SDR_PPM"]), str(value["SDR_GAIN"])
    except (OSError, ValueError, KeyError, TypeError):
        return None


def save_calibration(path, value):
    """Atomically publish a last-known-good calibration file."""
    directory = os.path.dirname(os.path.abspath(path))
    os.makedirs(directory, exist_ok=True)
    fd, temporary = tempfile.mkstemp(prefix=".sdr-calibration-", dir=directory)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as output:
            json.dump(value, output, indent=2)
            output.write("\n")
        os.replace(temporary, path)
    except Exception:
        try:
            os.unlink(temporary)
        except OSError:
            pass
        raise


class CalibrationAborted(Exception):
    """Calibration was interrupted by client shutdown."""


def perform_calibration(stations, gain, path, stop_event=None):
    """Run one receive-only calibration and persist it only on success."""
    if stop_event is not None and stop_event.is_set():
        raise CalibrationAborted()
    result = calibrate(stations, gain=gain)
    if stop_event is not None and stop_event.is_set():
        raise CalibrationAborted()
    minimum_stations = calibration_config()["min_stations"]
    validate_result(result, min_stations=minimum_stations)
    if stop_event is not None and stop_event.is_set():
        raise CalibrationAborted()
    save_calibration(path, result)
    return str(result["SDR_PPM"]), str(result["SDR_GAIN"])


def perform_target_calibration(target_hz, gain, path, config, stop_event=None):
    """Run target-band calibration and atomically persist only a good result."""
    if stop_event is not None and stop_event.is_set():
        raise CalibrationAborted()
    result = calibrate_target(
        target_hz, gain=gain, candidates=config["target_candidates"],
        repeats=config["target_repeats"],
        min_prominence=config["target_min_prominence"],
        min_margin=config["target_min_margin"],
    )
    if stop_event is not None and stop_event.is_set():
        raise CalibrationAborted()
    save_calibration(path, result)
    return str(result["SDR_PPM"]), str(result["SDR_GAIN"])


def message(client_id, raw_text):
    return {"client_id": client_id, "timestamp": datetime.now(timezone.utc).isoformat(), "raw_text": raw_text}


def drain_spool(spool, connection, channel):
    """Drain in order; leave the first unconfirmed message for retry."""
    while connection is not None and connection.is_open:
        item = spool.first()
        if item is None:
            return True
        item_id, payload = item
        publish_confirmed(channel, payload)
        spool.acknowledge(item_id)
    return False


def main():
    client_id = os.environ.get("CLIENT_ID", os.uname().nodename)
    # ISED TAFL lists Ottawa Fire paging transmitters/receivers at 155.610 MHz.
    frequency = os.environ.get("PAGER_FREQ", "155610000")
    calibration_file = os.environ.get("SDR_CALIBRATION_FILE", "/var/lib/firepager/sdr-calibration.json")
    configured = load_calibration(calibration_file)
    ppm, gain = configured or ("0", "49")
    # Explicit environment values always win over a persisted calibration.
    # Validate after applying overrides: a valid explicit override wins, while
    # an invalid one cannot displace a usable persisted last-known-good value.
    ppm = validate_sdr_setting(ppm, "SDR_PPM", "0", minimum=-200, maximum=200)
    gain = validate_sdr_setting(gain, "SDR_GAIN", "49", minimum=0, maximum=100)
    ppm = validate_sdr_setting(os.environ.get("SDR_PPM", ppm), "SDR_PPM", ppm,
                               minimum=-200, maximum=200)
    gain = validate_sdr_setting(os.environ.get("SDR_GAIN", gain), "SDR_GAIN", gain,
                                minimum=0, maximum=100)
    calibration = calibration_config()
    stations = calibration["stations"]
    periodic_interval = calibration["interval"]
    startup_calibration = calibration["on_start"]
    target_startup_calibration = calibration["target_on_start"]
    scheduler = None
    periodic_enabled = bool(stations and periodic_interval > 0 and CalibrationScheduler)
    if stations and (periodic_enabled or startup_calibration) and CalibrationScheduler:
        scheduler = CalibrationScheduler(
            periodic_interval,
            calibration["jitter"],
        )
    mock_mode = os.environ.get("MOCK_SDR", "false").lower() == "true"
    spool = SQLiteSpool(
        os.environ.get("SDR_SPOOL_PATH", os.environ.get("SPOOL_DB_PATH", "/var/lib/firepager/sdr-spool.sqlite3"))
    )
    stopping = threading.Event()
    calibration_lock = threading.Lock()
    # Held for the complete decoder lifetime; calibration acquires the same
    # OS lock once per rtl_power scan after this is released.
    decoder_lock = SDRDeviceLock() if SDRDeviceLock else None
    last_decoder_activity = time.monotonic()
    decoder_failures = 0
    decoder_retry_at = 0
    connection = channel = process = None

    def stop(_signum, _frame):
        logger.info("Shutdown requested")
        stopping.set()

    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)
    logger.info("Starting FirePager SDR Client (ID: %s)", client_id)
    try:
        if target_startup_calibration and calibrate_target:
            try:
                # This runs before rtl_fm starts, so rtl_power and rtl_fm never
                # own the dongle concurrently.
                ppm, gain = perform_target_calibration(
                    float(frequency), gain, calibration_file, calibration, stopping)
                logger.info("Startup target SDR calibration succeeded: ppm=%s gain=%s", ppm, gain)
            except Exception as exc:
                logger.warning("Startup target SDR calibration failed; using last-known-good values: %s", safe_error(exc))
        if scheduler and startup_calibration:
            try:
                with calibration_lock:
                    ppm, gain = perform_calibration(stations, gain, calibration_file, stopping)
                if periodic_enabled:
                    scheduler.success()
                logger.info("Startup SDR calibration succeeded: ppm=%s gain=%s", ppm, gain)
            except Exception as exc:
                if periodic_enabled:
                    scheduler.failure()
                logger.warning("Startup SDR calibration failed; using last-known-good values: %s", safe_error(exc))
        elif periodic_enabled:
            scheduler.schedule()
        while not stopping.is_set():
            # SDR reception does not depend on broker availability.  Starting
            # it before the RabbitMQ connect loop also avoids losing radio
            # time while a broker is down; the spool still buffers pages.
            if connection is None or not connection.is_open:
                close_connection(connection)
                connection, channel = connect_rabbitmq(stopping)
                if stopping.is_set():
                    break
            try:
                if periodic_enabled and scheduler.due():
                    quiet_for = time.monotonic() - last_decoder_activity
                    if quiet_for < calibration.get("quiet_period", 5):
                        scheduler.defer(calibration.get("quiet_period", 5) - quiet_for)
                        continue
                    # rtl_power and rtl_fm cannot safely share one dongle.
                    # Stop only at this maintenance boundary; the process is
                    # restarted immediately after calibration or failure.
                    terminate_process_group(process)
                    process = None
                    if decoder_lock:
                        decoder_lock.release()
                    try:
                        with calibration_lock:
                            ppm, gain = perform_calibration(stations, gain, calibration_file, stopping)
                        scheduler.success()
                        logger.info("Periodic SDR calibration succeeded: ppm=%s gain=%s", ppm, gain)
                    except Exception as exc:
                        scheduler.failure()
                        logger.warning("Periodic SDR calibration failed; retaining last-known-good values: %s", safe_error(exc))
                drain_spool(spool, connection, channel)
                if mock_mode:
                    raw_line = "POCSAG512: mock page"
                    spool.enqueue(message(client_id, raw_line))
                    drain_spool(spool, connection, channel)
                    stopping.wait(10)
                    continue
                if process is None or process.poll() is not None:
                    now = time.monotonic()
                    if now < decoder_retry_at:
                        stopping.wait(min(1, decoder_retry_at - now))
                        continue
                    try:
                        if decoder_lock:
                            decoder_lock.acquire(timeout=0)
                        process = start_sdr(frequency, ppm=ppm, gain=gain)
                        last_decoder_activity = now
                    except DeviceBusyError:
                        decoder_failures += 1
                        decoder_retry_at = now + min(30, 2 ** min(decoder_failures, 5))
                        logger.warning("RTL-SDR is busy; decoder start deferred")
                        continue
                    except Exception:
                        if decoder_lock:
                            decoder_lock.release()
                        raise
                # Do not leave shutdown blocked forever on a silent SDR pipe.
                ready, _, _ = select.select([process.stdout], [], [], 1)
                if not ready:
                    continue
                line = process.stdout.readline()
                if line:
                    last_decoder_activity = time.monotonic()
                    line = line.strip()
                    lower_line = line.lower()
                    if ("rtl_fm" in lower_line and
                            ("cannot open" in lower_line or "no supported" in lower_line
                             or "no devices" in lower_line)):
                        logger.warning("SDR decoder could not open the device: %s", line)
                        terminate_process_group(process)
                        process = None
                        if decoder_lock:
                            decoder_lock.release()
                        decoder_failures += 1
                        decoder_retry_at = time.monotonic() + min(30, 2 ** min(decoder_failures, 5))
                        continue
                    if line:
                        spool.enqueue(message(client_id, line))
                        drain_spool(spool, connection, channel)
                    # A pipeline can emit its final line while the shell
                    # leader is already exiting. In that case the `elif`
                    # below is skipped; still record the failure and back off
                    # before attempting another decoder start.
                    if process.poll() is not None:
                        terminate_process_group(process)
                        process = None
                        if decoder_lock:
                            decoder_lock.release()
                        decoder_failures += 1
                        decoder_retry_at = time.monotonic() + min(30, 2 ** min(decoder_failures, 5))
                elif process.poll() is not None:
                    terminate_process_group(process)
                    process = None
                    if decoder_lock:
                        decoder_lock.release()
                    decoder_failures += 1
                    decoder_retry_at = time.monotonic() + min(30, 2 ** min(decoder_failures, 5))
                    stopping.wait(min(1, max(0, decoder_retry_at - time.monotonic())))
            except Exception as exc:
                logger.error("Client work failed; retaining spool: %s", safe_error(exc))
                close_connection(connection)
                connection = channel = None
                stopping.wait(1)
    finally:
        terminate_process_group(process)
        if decoder_lock:
            decoder_lock.release()
        if connection is not None and connection.is_open:
            try:
                drain_spool(spool, connection, channel)
            except Exception as exc:
                logger.error("Could not drain spool during shutdown: %s", safe_error(exc))
        close_connection(connection)
        logger.info("Shutdown complete; %d message(s) remain spooled", spool.size())
        spool.close()


if __name__ == "__main__":
    main()
