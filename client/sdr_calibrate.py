#!/usr/bin/env python3
"""Receive-only RTL-SDR calibration using known FM carriers or a pager signal.

No transmitter or network dependency is used.  ``rtl_power`` performs a
short receive scan; the strongest bin near each supplied station is compared
with its published frequency to estimate crystal error.  The resulting
values can be put directly in the client environment.
"""

import argparse
import csv
import json
import statistics
import subprocess
import sys
import random
import math
import errno
import fcntl
import hashlib
import os
import time


class DeviceBusyError(RuntimeError):
    """The configured RTL-SDR is already owned by another process."""


class RTLPowerExecutionError(RuntimeError):
    """rtl_power could not complete a scan (as opposed to no signal)."""


def device_lock_path(environ=None):
    """Return a stable lock path for one physical dongle.

    A serial/device identifier is preferred.  The spool path is the fallback
    because it is normally unique per deployed client and requires no device
    probing (which could itself open the dongle).
    """
    environ = os.environ if environ is None else environ
    explicit = environ.get("SDR_LOCK_PATH")
    if explicit:
        return explicit
    identity = (environ.get("SDR_DEVICE") or environ.get("RTLSDR_DEVICE") or
                environ.get("SDR_SERIAL") or environ.get("RTLSDR_SERIAL") or
                environ.get("SDR_SPOOL_PATH") or environ.get("SPOOL_DB_PATH") or
                "/var/lib/firepager/sdr-spool.sqlite3")
    digest = hashlib.sha256(identity.encode("utf-8")).hexdigest()[:24]
    return os.path.join(environ.get("SDR_LOCK_DIR", "/tmp"),
                        "firepager-sdr-{}.lock".format(digest))


class SDRDeviceLock:
    """An inter-process, non-blocking lock for the RTL-SDR device."""

    def __init__(self, path=None):
        self.path = path or device_lock_path()
        self._file = None

    def acquire(self, timeout=30, retry_delay=0.25, sleep=time.sleep):
        if self._file is not None:
            return True
        directory = os.path.dirname(os.path.abspath(self.path))
        os.makedirs(directory, exist_ok=True)
        self._file = open(self.path, "a+")
        deadline = time.monotonic() + max(0, float(timeout))
        try:
            while True:
                try:
                    fcntl.flock(self._file.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
                    return True
                except OSError as exc:
                    if exc.errno != errno.EWOULDBLOCK:
                        raise
                    if time.monotonic() >= deadline:
                        raise DeviceBusyError("RTL-SDR lock is busy: {}".format(self.path))
                    sleep(min(float(retry_delay), max(0, deadline - time.monotonic())))
        except Exception:
            self.release()
            raise

    def release(self):
        if self._file is not None:
            try:
                fcntl.flock(self._file.fileno(), fcntl.LOCK_UN)
            finally:
                self._file.close()
                self._file = None

    def __enter__(self):
        self.acquire()
        return self

    def __exit__(self, _type, _value, _traceback):
        self.release()


def parse_station(value):
    """Parse NAME=frequency or frequency (Hz, or MHz with a decimal)."""
    if not isinstance(value, str) or not value.strip():
        raise ValueError("station must include a frequency")
    if "=" in value:
        name, value = value.split("=", 1)
    else:
        name = value
    name = name.strip()
    frequency = float(value.strip())
    if not name or not math.isfinite(frequency) or frequency <= 0:
        raise ValueError("station name and frequency must be valid")
    if frequency < 1_000_000:
        frequency *= 1_000_000
    return name, frequency


def strongest_frequency(csv_text):
    """Return (frequency Hz, power dB) from one or more rtl_power CSV rows."""
    best = None
    for row in csv.reader(csv_text.splitlines()):
        if len(row) < 7:
            continue
        try:
            low, step = float(row[2]), float(row[4])
            powers = [float(item) for item in row[6:] if item.strip()]
        except (ValueError, IndexError):
            continue
        if not powers:
            continue
        index = max(range(len(powers)), key=powers.__getitem__)
        candidate = (low + index * step, powers[index])
        if best is None or candidate[1] > best[1]:
            best = candidate
    if best is None:
        raise ValueError("rtl_power produced no usable CSV power bins")
    return best


def _power_bins(csv_text):
    """Return usable ``(frequency, power)`` bins from rtl_power output."""
    bins = []
    for row in csv.reader(csv_text.splitlines()):
        if len(row) < 7:
            continue
        try:
            low, step = float(row[2]), float(row[4])
            powers = [float(item) for item in row[6:] if item.strip()]
        except (ValueError, IndexError):
            continue
        bins.extend((low + index * step, power)
                    for index, power in enumerate(powers))
    return bins


def fsk_pair_score(csv_text, target_hz, band_hz=15_000,
                   min_separation_hz=2_000, max_separation_hz=15_000):
    """Estimate a POCSAG 2-FSK channel from its two frequency lobes.

    POCSAG does not produce one continuous carrier peak.  This finds two
    separated local maxima near the target and uses their midpoint as the
    channel estimate.  A single peak is intentionally rejected.
    """
    bins = _power_bins(csv_text)
    if not bins:
        raise ValueError("rtl_power produced no usable CSV power bins")
    nearby = [(frequency, power) for frequency, power in bins
              if abs(frequency - float(target_hz)) <= band_hz]
    if not nearby:
        raise ValueError("rtl_power produced no bins near the pager target")
    floor = statistics.median(power for _, power in bins)
    peaks = []
    for index in range(1, len(nearby) - 1):
        frequency, power = nearby[index]
        if power >= nearby[index - 1][1] and power >= nearby[index + 1][1]:
            peaks.append((power, frequency))
    pairs = []
    for left_index, left in enumerate(peaks):
        for right in peaks[left_index + 1:]:
            separation = abs(right[1] - left[1])
            if min_separation_hz <= separation <= max_separation_hz:
                lower, upper = sorted((left, right), key=lambda item: item[1])
                center = (lower[1] + upper[1]) / 2
                prominence = min(lower[0], upper[0]) - floor
                pairs.append((prominence, -abs(center - float(target_hz)), lower, upper, center))
    if not pairs:
        raise ValueError("no repeatable two-lobe FSK signal near pager target")
    prominence, _alignment, lower, upper, center = max(pairs)
    return {
        "lower_hz": lower[1],
        "upper_hz": upper[1],
        "center_hz": center,
        "center_error_hz": center - float(target_hz),
        "peak_db": max(lower[0], upper[0]),
        "noise_floor_db": floor,
        "prominence_db": prominence,
        "energy_db": statistics.mean(power for _, power in nearby),
    }


def target_signal_score(csv_text, target_hz, band_hz=25_000):
    """Compatibility wrapper returning a two-lobe POCSAG score."""
    return fsk_pair_score(csv_text, target_hz, band_hz=band_hz)


def estimate_ppm(known_hz, measured_hz):
    """Return the observed oscillator offset (measured minus known), in ppm."""
    return (measured_hz - known_hz) / known_hz * 1_000_000


def applied_correction_ppm(observed_offset_ppm):
    """Convert observed offset to rtl-sdr's correction sign.

    ``rtl_fm -p`` corrects the oscillator error, so a carrier observed high
    requires a negative applied correction (and vice versa).  Keep the raw
    observation in calibration JSON so this sign convention is auditable.
    """
    return -float(observed_offset_ppm)


def _run_scan(command, runner, timeout):
    try:
        result = runner(command, capture_output=True, text=True, timeout=timeout,
                        check=True)
        returncode = getattr(result, "returncode", 0)
        if returncode:
            stderr = getattr(result, "stderr", "") or ""
            raise RTLPowerExecutionError(
                "rtl_power exited with status {}{}".format(
                    returncode, ": {}".format(stderr.strip()) if stderr.strip() else ""))
        return result
    except RTLPowerExecutionError:
        raise
    except (OSError, subprocess.SubprocessError) as exc:
        detail = getattr(exc, "stderr", "") or ""
        raise RTLPowerExecutionError("rtl_power scan failed: {}{}".format(
            exc, ": {}".format(detail.strip()) if detail.strip() else "")) from exc


def _scan_with_lock(command, runner, timeout, lock_path=None, lock_timeout=30,
                    retries=2, retry_delay=0.5, sleep=time.sleep):
    """Run a bounded number of scans, taking the OS lock for every attempt."""
    last_error = None
    for attempt in range(max(1, int(retries) + 1)):
        try:
            lock = SDRDeviceLock(lock_path)
            lock.acquire(timeout=lock_timeout, retry_delay=retry_delay, sleep=sleep)
            try:
                return _run_scan(command, runner, timeout)
            finally:
                lock.release()
        except (DeviceBusyError, RTLPowerExecutionError) as exc:
            last_error = exc
            if attempt >= max(1, int(retries)):
                raise
            sleep(float(retry_delay) * (2 ** attempt))
    raise last_error  # pragma: no cover - loop always returns or raises


def scan(station_hz, gain, ppm=0, runner=subprocess.run, timeout=30,
         lock_path=None, lock_timeout=30, retries=2, retry_delay=0.5,
         sleep=time.sleep):
    """Scan one FM carrier.  ``runner`` is injectable for unit tests."""
    width = 100_000
    start, end = int(station_hz - width), int(station_hz + width)
    command = ["rtl_power", "-f", f"{start}:{end}:1000", "-g", str(gain),
               "-p", str(ppm), "-1"]
    result = _scan_with_lock(command, runner, timeout, lock_path, lock_timeout,
                             retries, retry_delay, sleep)
    return strongest_frequency(result.stdout)


def scan_target(target_hz, gain, ppm=0, runner=subprocess.run, timeout=30,
                lock_path=None, lock_timeout=30, retries=2, retry_delay=0.5,
                sleep=time.sleep):
    """Scan only the known pager band; this is receive-only."""
    width = 100_000
    start, end = int(float(target_hz) - width), int(float(target_hz) + width)
    command = ["rtl_power", "-f", f"{start}:{end}:1000", "-g", str(gain),
               "-p", str(ppm), "-1"]
    result = _scan_with_lock(command, runner, timeout, lock_path, lock_timeout,
                             retries, retry_delay, sleep)
    return target_signal_score(result.stdout, target_hz)


def target_ppm_candidates(start=-8, end=8, step=1):
    """Build a bounded, deterministic candidate list for target calibration."""
    start, end, step = float(start), float(end), float(step)
    if not all(math.isfinite(value) for value in (start, end, step)) or step <= 0 or end < start:
        raise ValueError("invalid target PPM candidate range")
    count = int(math.floor((end - start) / step + 1e-9)) + 1
    if count > 401:
        raise ValueError("target PPM candidate range is too large")
    return [round(start + index * step, 6) for index in range(count)]


def calibrate_target(target_hz, gain="49", candidates=None, repeats=2,
                     min_prominence=6.0, min_margin=1.0, runner=subprocess.run):
    """Find a repeatable PPM correction from the known POCSAG target signal.

    Every candidate owns the dongle only for its short receive scan.  A weak,
    absent, or ambiguous target raises ``ValueError`` and must not be saved.
    """
    candidates = target_ppm_candidates() if candidates is None else list(candidates)
    if len(candidates) < 2 or any(not math.isfinite(float(item)) for item in candidates):
        raise ValueError("at least two valid target PPM candidates are required")
    repeats = int(repeats)
    if repeats < 2:
        raise ValueError("target calibration requires at least two repeats")
    observations = {float(candidate): [] for candidate in candidates}
    winners = []
    for repeat in range(repeats):
        scored = []
        for candidate in candidates:
            score = scan_target(target_hz, gain, ppm=candidate, runner=runner)
            observations[float(candidate)].append(score)
            alignment = abs(float(score["center_error_hz"])) / 1000.0
            scored.append((score["prominence_db"] - alignment, float(candidate)))
        winners.append(max(scored)[1])
    if len(set(winners)) != 1:
        raise ValueError("target signal did not produce a repeatable PPM winner")
    ranked = sorted(
         ((statistics.mean(item["prominence_db"] - abs(item["center_error_hz"]) / 1000.0 for item in values), candidate)
         for candidate, values in observations.items()), reverse=True)
    best_score, best = ranked[0]
    margin = best_score - ranked[1][0]
    if best_score < float(min_prominence):
        raise ValueError("no sufficiently prominent pager signal near target")
    if margin < float(min_margin):
        raise ValueError("target PPM winner lacks sufficient confidence margin")
    result = {
        "SDR_PPM": round(best, 2), "SDR_GAIN": str(gain),
         "target_hz": float(target_hz), "prominence_db": round(best_score, 2),
        "confidence_margin_db": round(margin, 2), "repeats": repeats,
        "target_candidates": [
             {"ppm": candidate, "prominence_db": round(score, 2)}
            for score, candidate in ranked
        ],
    }
    return result


def calibrate(stations, gain="49", runner=subprocess.run):
    observations = []
    for name, known_hz in stations:
        measured_hz, power = scan(known_hz, gain, runner=runner)
        observations.append({
            "station": name, "known_hz": known_hz, "measured_hz": measured_hz,
            "power_db": power,
            "observed_offset_ppm": estimate_ppm(known_hz, measured_hz),
        })
    if not observations:
        raise ValueError("at least one station is required")
    observed_offset = statistics.median(item["observed_offset_ppm"] for item in observations)
    result = {
        # Keep the established operational sign until it is verified against
        # the installed dongle/rtl-sdr build.  The raw observation and the
        # alternative correction sign are recorded explicitly for audit.
        "SDR_PPM": round(observed_offset, 2),
        "observed_offset_ppm": round(observed_offset, 2),
        "applied_correction_ppm": round(applied_correction_ppm(observed_offset), 2),
        "SDR_GAIN": str(gain), "stations": observations,
    }
    validate_result(result)
    return result


def validate_result(result, min_stations=1, max_spread_ppm=3.0, max_abs_ppm=200.0):
    """Reject implausible or contradictory reference measurements.

    A strong but spurious FM peak must not replace a known-good correction.
    Multiple references should agree closely; a single reference is allowed
    for initial setup, while deployments can require two or more.
    """
    if not isinstance(result, dict):
        raise ValueError("calibration result must be an object")
    if min_stations < 1:
        raise ValueError("minimum station count must be positive")
    observations = result.get("stations", [])
    if not isinstance(observations, list):
        raise ValueError("calibration stations must be a list")
    if len(observations) < min_stations:
        raise ValueError("not enough valid FM reference stations")
    try:
        estimates = [float(item.get("observed_offset_ppm", item.get("ppm")))
                     for item in observations]
    except (KeyError, TypeError, ValueError):
        raise ValueError("calibration station estimates are invalid")
    if any(not math.isfinite(value) for value in estimates):
        raise ValueError("calibration station estimates are not finite")
    candidate = statistics.median(estimates)
    if any(abs(value) > max_abs_ppm for value in estimates):
        raise ValueError("FM reference estimate is outside the safe PPM range")
    if any(abs(value - candidate) > max_spread_ppm for value in estimates):
        raise ValueError("FM reference estimates disagree")
    # Preserve the deployed sign convention; see calibrate() for the explicit
    # observed-vs-applied values recorded for later hardware verification.
    result["SDR_PPM"] = round(candidate, 2)
    return result


class CalibrationScheduler:
    """Jittered, deterministic-with-a-RNG schedule for periodic calibration.

    Jitter is added in the range [0, jitter], so a configured base interval is
    always respected.  A failed run backs off exponentially, preventing a
    broken dongle or station from repeatedly interrupting pager reception.
    """

    def __init__(self, interval, jitter=0, clock=None, rng=None, max_backoff=None):
        if (not math.isfinite(float(interval)) or not math.isfinite(float(jitter))
                or interval < 0 or jitter < 0):
            raise ValueError("calibration interval and jitter must be non-negative")
        self.interval = float(interval)
        self.jitter = float(jitter)
        self.clock = clock or __import__("time").monotonic
        self.rng = rng or random.Random()
        self.max_backoff = float(max_backoff or max(interval * 8, interval))
        self.failures = 0
        self.due_at = None

    def _delay(self):
        # The first retry waits one normal interval; later failures double it.
        base = min(self.interval * (2 ** max(self.failures - 1, 0)), self.max_backoff)
        return base + self.rng.uniform(0, self.jitter)

    def schedule(self, now=None):
        self.due_at = (self.clock() if now is None else now) + self._delay()
        return self.due_at

    def due(self, now=None):
        return self.due_at is not None and (self.clock() if now is None else now) >= self.due_at

    def defer(self, delay, now=None):
        """Move a due run out by a short operational quiet period."""
        if not math.isfinite(float(delay)) or delay < 0:
            raise ValueError("calibration defer delay must be non-negative")
        self.due_at = (self.clock() if now is None else now) + float(delay)
        return self.due_at

    def success(self, now=None):
        self.failures = 0
        return self.schedule(now)

    def failure(self, now=None):
        self.failures += 1
        return self.schedule(now)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument("--station", action="append",
                        metavar="NAME=FREQUENCY",
                        help="known FM station, in Hz or MHz; repeatable")
    source.add_argument("--target", type=float, metavar="FREQUENCY",
                        help="known POCSAG target frequency in Hz")
    parser.add_argument("--gain", default="49",
                        help="RTL gain in dB (default: 49; calibrate each dongle consistently)")
    parser.add_argument("--ppm-min", type=float, default=-8)
    parser.add_argument("--ppm-max", type=float, default=8)
    parser.add_argument("--ppm-step", type=float, default=1)
    parser.add_argument("--repeats", type=int, default=2)
    parser.add_argument("--output", help="write JSON calibration file")
    args = parser.parse_args(argv)
    try:
        if args.target is not None:
            result = calibrate_target(
                args.target, args.gain,
                candidates=target_ppm_candidates(args.ppm_min, args.ppm_max, args.ppm_step),
                repeats=args.repeats,
            )
        else:
            result = calibrate([parse_station(item) for item in args.station], args.gain)
    except (OSError, subprocess.CalledProcessError, ValueError) as exc:
        parser.error(str(exc))
    serialized = json.dumps(result, indent=2) + "\n"
    if args.output:
        with open(args.output, "w", encoding="utf-8") as output:
            output.write(serialized)
    print(serialized, end="")
    print("Set SDR_PPM and SDR_GAIN from this output before starting the client.",
          file=sys.stderr)


if __name__ == "__main__":
    main()
