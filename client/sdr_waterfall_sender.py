#!/usr/bin/env python3
"""Publish periodic measured RTL-SDR waterfall snapshots over RabbitMQ.

This process is intended for a dedicated spectrum-only receiver. It owns the
RTL-SDR through one rtl_power process, retains at most one short window in
memory, and sends each snapshot to the durable receiver_waterfalls queue.
It does not decode pages, retune, or calibrate the receiver.
"""

from __future__ import annotations

import argparse
import csv
import json
import logging
import math
import os
import signal
import subprocess
import sys
import time
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional

import pika

try:
    from sdr_calibrate import SDRDeviceLock
except ImportError:  # Test/minimal environments can inject no lock implementation.
    SDRDeviceLock = None

LOG = logging.getLogger("firepager.sdr_waterfall")
QUEUE = "receiver_waterfalls"
DEFAULT_RANGE_HZ = "155538000:155658000:250"
MAX_ROWS = 120
MAX_BINS = 512
MAX_PAYLOAD_BYTES = 1024 * 1024


def parse_rtl_power_row(line: str) -> Optional[Dict[str, Any]]:
    """Parse one rtl_power CSV line into a timestamped, measured spectrum row."""
    try:
        columns = next(csv.reader([line]))
        if len(columns) < 8:
            return None
        stamp = datetime.strptime(
            columns[0].strip() + " " + columns[1].strip(), "%Y-%m-%d %H:%M:%S"
        ).replace(tzinfo=timezone.utc)
        start_hz = float(columns[2])
        step_hz = float(columns[4])
        power_db = [float(value) for value in columns[6:] if value.strip()]
    except (ValueError, IndexError, StopIteration):
        return None
    if (not math.isfinite(start_hz) or not math.isfinite(step_hz) or step_hz <= 0
            or not power_db
            or any(not math.isfinite(value) for value in power_db)):
        return None
    # rtl_power may emit one extra edge bin when it rounds the requested
    # maximum bin size to its FFT grid. Keep the leftmost bounded spectrum.
    power_db = power_db[:MAX_BINS]
    return {
        "captured_at": stamp.isoformat().replace("+00:00", "Z"),
        "start_hz": start_hz,
        "step_hz": step_hz,
        "power_db": power_db,
    }


def make_snapshot(receiver_id: str, rows: List[Dict[str, Any]]) -> Dict[str, Any]:
    """Build a bounded batch with a single consistent frequency grid."""
    if not rows:
        raise ValueError("snapshot requires at least one measured row")
    first = rows[0]
    expected_shape = (first["start_hz"], first["step_hz"], len(first["power_db"]))
    for row in rows:
        shape = (row["start_hz"], row["step_hz"], len(row["power_db"]))
        if shape != expected_shape:
            raise ValueError("spectrum rows do not share one frequency grid")
    snapshot = {
        "receiver_id": receiver_id,
        "captured_at": rows[-1]["captured_at"],
        "start_hz": first["start_hz"],
        "step_hz": first["step_hz"],
        "rows": [{"captured_at": row["captured_at"], "power_db": row["power_db"]}
                 for row in rows[-MAX_ROWS:]],
    }
    body = json.dumps(snapshot, separators=(",", ":"), allow_nan=False).encode("utf-8")
    if len(body) > MAX_PAYLOAD_BYTES:
        raise ValueError("waterfall snapshot exceeds the 1 MiB transport limit")
    return snapshot


def publish_snapshot(snapshot: Dict[str, Any], broker_url: str) -> None:
    """Publish one persistent snapshot with broker confirmation."""
    if not broker_url:
        raise ValueError("RABBITMQ_URL is required")
    parameters = pika.URLParameters(broker_url)
    parameters.socket_timeout = 10
    connection = pika.BlockingConnection(parameters)
    try:
        channel = connection.channel()
        channel.queue_declare(queue=QUEUE, durable=True)
        channel.confirm_delivery()
        body = json.dumps(snapshot, separators=(",", ":"), allow_nan=False).encode("utf-8")
        if len(body) > MAX_PAYLOAD_BYTES:
            raise ValueError("waterfall snapshot exceeds the 1 MiB transport limit")
        confirmed = channel.basic_publish(
            exchange="", routing_key=QUEUE, body=body,
            properties=pika.BasicProperties(
                delivery_mode=2, content_type="application/json",
                message_id="{}:{}".format(snapshot["receiver_id"], snapshot["captured_at"]),
            ), mandatory=True,
        )
        if confirmed is False:
            raise RuntimeError("RabbitMQ publisher confirmation was negative")
    finally:
        connection.close()


def rtl_power_command(args: argparse.Namespace) -> List[str]:
    """Construct one fixed-tune, manual-gain spectrum scan."""
    return [
        "rtl_power", "-f", args.frequency_range, "-i", str(args.integration_seconds),
        "-1", "-g", str(args.gain), "-p", str(args.ppm), "-d", str(args.device), "-",
    ]


def run(args: argparse.Namespace) -> int:
    broker_url = os.environ.get("RABBITMQ_URL", "")
    if not broker_url:
        LOG.error("RABBITMQ_URL is required; no snapshot was sent")
        return 2

    stopping = False

    def stop(_signum: int, _frame: Any) -> None:
        nonlocal stopping
        stopping = True

    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)
    command = rtl_power_command(args)
    lock = None
    if SDRDeviceLock is not None:
        try:
            lock = SDRDeviceLock(os.environ.get("SDR_LOCK_PATH") or None)
            lock.acquire(timeout=0)
        except Exception as exc:
            LOG.error("RTL-SDR is already owned (%s); refusing to start a second reader",
                      type(exc).__name__)
            return 3
    LOG.info("Starting fixed-tune spectrum capture for %s", args.receiver_id)
    LOG.info("Band=%s integration=%ss upload_interval=%ss", args.frequency_range,
             args.integration_seconds, args.interval_seconds)
    rows: List[Dict[str, Any]] = []
    next_publish = time.monotonic() + args.interval_seconds
    try:
        while not stopping:
            try:
                result = subprocess.run(
                    command, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                    text=True, encoding="utf-8", errors="replace",
                    timeout=max(30, args.integration_seconds * 6), check=False,
                )
            except subprocess.TimeoutExpired:
                LOG.warning("rtl_power scan timed out; will retry next interval")
                result = None
            except OSError as exc:
                LOG.error("Could not run rtl_power (%s)", type(exc).__name__)
                return 2

            if result is not None:
                if result.returncode != 0:
                    LOG.warning("rtl_power scan failed with status %s", result.returncode)
                else:
                    for line in result.stdout.splitlines():
                        row = parse_rtl_power_row(line)
                        if row is None:
                            continue
                        if rows and (row["start_hz"], row["step_hz"], len(row["power_db"])) != (
                                rows[0]["start_hz"], rows[0]["step_hz"], len(rows[0]["power_db"])):
                            rows.clear()
                        rows.append(row)
                    rows = rows[-MAX_ROWS:]

            if time.monotonic() >= next_publish:
                if rows:
                    snapshot = make_snapshot(args.receiver_id, rows)
                    try:
                        publish_snapshot(snapshot, broker_url)
                        LOG.info("Published waterfall snapshot: rows=%d bins=%d",
                                 len(snapshot["rows"]), len(snapshot["rows"][0]["power_db"]))
                        rows.clear()
                    except Exception as exc:
                        # Keep the measured batch and retry it at the next
                        # cadence; never log broker URL or exception text.
                        LOG.warning("Snapshot upload failed (%s); retaining rows for retry",
                                    type(exc).__name__)
                else:
                    LOG.warning("No complete rtl_power rows at snapshot interval")
                next_publish = time.monotonic() + args.interval_seconds
            if not stopping:
                time.sleep(0.1)
    finally:
        if lock:
            lock.release()
    return 0


def parse_args(argv: Optional[List[str]] = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--receiver-id", default="remote-pi-sdr")
    parser.add_argument("--frequency-range", default=DEFAULT_RANGE_HZ)
    parser.add_argument("--interval-seconds", type=int, default=300)
    parser.add_argument("--integration-seconds", type=int, default=5)
    parser.add_argument("--gain", default="49.6")
    parser.add_argument("--ppm", type=int, default=-5)
    parser.add_argument("--device", type=int, default=0)
    args = parser.parse_args(argv)
    try:
        low, high, step = (float(value) for value in args.frequency_range.split(":"))
    except (ValueError, AttributeError):
        parser.error("frequency range must be lower:upper:step in Hz")
    if (not (0 < low < high) or step <= 0 or high - low > 2_000_000
            or args.interval_seconds < args.integration_seconds
            or args.integration_seconds < 1 or args.device < 0):
        parser.error("invalid capture range, cadence, integration or device index")
    try:
        gain = float(args.gain)
    except ValueError:
        parser.error("manual gain must be numeric")
    if not math.isfinite(gain) or not (0 <= gain <= 100):
        parser.error("manual gain must be finite and in [0, 100]")
    return args


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
    sys.exit(run(parse_args()))
