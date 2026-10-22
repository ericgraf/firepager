#!/usr/bin/env python3
"""Bounded shared-IQ diagnostics for the opt-in high-power SDR mode.

This module deliberately does not claim to demodulate POCSAG.  It owns one
``rtl_sdr`` process and reports coarse IQ power lobes suitable for observing a
waterfall/FSK channel.  A calibration candidate is emitted only when two
lobes are both prominent; noise never produces a PPM value.
"""

import argparse
import json
import math
import subprocess
import sys


def goertzel_power(samples, frequency_hz, sample_rate):
    """Return normalized power at one frequency for interleaved unsigned IQ."""
    omega = 2.0 * math.pi * frequency_hz / sample_rate
    coefficient = 2.0 * math.cos(omega)
    state1 = state2 = 0.0
    for index in range(0, len(samples) - 1, 2):
        # Mix I/Q to a real projection at the requested offset.
        i = (samples[index] - 127.5) / 127.5
        q = (samples[index + 1] - 127.5) / 127.5
        sample = i * math.cos(omega * index / 2) + q * math.sin(omega * index / 2)
        state0 = sample + coefficient * state1 - state2
        state2, state1 = state1, state0
    return (state1 * state1 + state2 * state2 - coefficient * state1 * state2) / max(1, len(samples) // 2)


def analyze_iq(data, sample_rate, channel_hz, span_hz=12000, bins=25,
               min_prominence_db=6.0):
    """Return a coarse spectrum and a conservative two-lobe assessment."""
    if len(data) < 32 or len(data) % 2:
        return {"ready": False, "reason": "short_iq_block", "calibration_candidate": None}
    offsets = [(-span_hz + 2 * span_hz * i / (bins - 1)) for i in range(bins)]
    powers = [goertzel_power(data, offset, sample_rate) for offset in offsets]
    db = [10 * math.log10(max(power, 1e-12)) for power in powers]
    floor = sorted(db)[len(db) // 2]
    peaks = [(db[i], offsets[i]) for i in range(1, len(db) - 1)
             if db[i] >= db[i - 1] and db[i] >= db[i + 1]
             and db[i] - floor >= min_prominence_db]
    peaks.sort(reverse=True)
    lobes = None
    if len(peaks) >= 2:
        first, second = sorted((peaks[0], peaks[1]), key=lambda item: item[1])
        separation = second[1] - first[1]
        if 2000 <= separation <= 15000:
            lobes = {"lower_offset_hz": first[1], "upper_offset_hz": second[1],
                     "center_offset_hz": (first[1] + second[1]) / 2,
                     "prominence_db": min(first[0], second[0]) - floor}
    return {"ready": True, "noise_floor_db": round(floor, 2),
            "waterfall_db": [round(value, 2) for value in db],
            "offsets_hz": offsets, "fsk_lobes": lobes,
            # Frequency correction needs a repeatable reference and is never
            # inferred from a single raw-IQ block.
            "calibration_candidate": None}


def run(args):
    command = ["rtl_sdr", "-f", str(args.frequency), "-s", str(args.sample_rate),
               "-g", str(args.gain), "-p", str(args.ppm), "-"]
    try:
        capture = subprocess.Popen(command, stdout=subprocess.PIPE,
                                    stderr=subprocess.PIPE)
    except OSError as exc:
        print("HIGH_POWER_FALLBACK rtl_sdr unavailable: {}".format(exc), flush=True)
        return 2
    print(json.dumps({"mode": "high_power", "owner": "shared_iq",
                      "decoder": "bounded_analyzer_only",
                      "pocsag_decode": "not_implemented",
                      "calibration": "withheld_until_repeatable_reference"}), flush=True)
    try:
        while True:
            block = capture.stdout.read(args.block_bytes)
            if not block:
                detail = capture.stderr.read().decode("utf-8", "replace").strip()
                print("HIGH_POWER_FALLBACK capture stopped{}".format(
                    ": " + detail if detail else ""), flush=True)
                return 2
            result = analyze_iq(block, args.sample_rate, args.frequency,
                                args.span_hz, args.bins, args.min_prominence_db)
            print("SDR_DIAGNOSTIC " + json.dumps(result, separators=(",", ":")), flush=True)
    finally:
        capture.terminate()
        try:
            capture.wait(timeout=2)
        except subprocess.TimeoutExpired:
            capture.kill()
            capture.wait()


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--frequency", type=int, required=True)
    parser.add_argument("--sample-rate", type=int, default=240000)
    parser.add_argument("--gain", default="49")
    parser.add_argument("--ppm", default="0")
    parser.add_argument("--span-hz", type=int, default=12000)
    parser.add_argument("--bins", type=int, default=25)
    parser.add_argument("--block-bytes", type=int, default=262144)
    parser.add_argument("--min-prominence-db", type=float, default=6)
    args = parser.parse_args(argv)
    if args.bins < 5 or args.block_bytes < 32 or args.sample_rate <= 0:
        parser.error("invalid high-power analyzer bounds")
    return run(args)


if __name__ == "__main__":
    sys.exit(main())
