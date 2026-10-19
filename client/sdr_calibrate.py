#!/usr/bin/env python3
"""Receive-only RTL-SDR calibration using known FM broadcast carriers.

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


def scan(station_hz, gain, ppm=0, runner=subprocess.run, timeout=30):
    """Scan one FM carrier.  ``runner`` is injectable for unit tests."""
    width = 100_000
    start, end = int(station_hz - width), int(station_hz + width)
    command = ["rtl_power", "-f", f"{start}:{end}:1000", "-g", str(gain),
               "-p", str(ppm), "-1"]
    result = runner(command, capture_output=True, text=True, timeout=timeout,
                    check=True)
    return strongest_frequency(result.stdout)


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
    parser.add_argument("--station", action="append", required=True,
                        metavar="NAME=FREQUENCY",
                        help="known FM station, in Hz or MHz; repeatable")
    parser.add_argument("--gain", default="49",
                        help="RTL gain in dB (default: 49; calibrate each dongle consistently)")
    parser.add_argument("--output", help="write JSON calibration file")
    args = parser.parse_args(argv)
    try:
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
