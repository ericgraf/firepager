#!/usr/bin/env python3
"""Shared-IQ POCSAG receiver for the opt-in high-power SDR mode.

The decoder intentionally uses only the Python standard library.  RTL-SDR
unsigned IQ is phase-differentiated into narrow-band 2-FSK, symbol timing is
searched for each supported POCSAG rate, and BCH validation is required before
anything is emitted to the client spool.  This is not a general purpose DSP
receiver: very weak, badly tuned, or heavily multipath signals may need the
legacy ``multimon-ng`` path.
"""

import argparse
import json
import math
import subprocess
import sys


POCSAG_SYNC = 0x7CD215D8
POCSAG_BCH = 0x769
POCSAG_BAUDS = (512, 1200, 2400)
IDLE_CODEWORD = 0x7A89C197


def _bch_ok(word):
    """Check the shortened BCH(31,21) remainder and even overall parity."""
    value = (word & 0xFFFFFFFE) >> 1
    for bit in range(30, 9, -1):
        if value & (1 << bit):
            value ^= POCSAG_BCH << (bit - 10)
    return (value & 0x3FF) == 0 and word.bit_count() % 2 == 0


def decode_codeword(word):
    """Return a corrected word, or ``None``; POCSAG permits two bit errors."""
    word &= 0xFFFFFFFF
    if _bch_ok(word):
        return word
    for bit in range(32):
        candidate = word ^ (1 << bit)
        if _bch_ok(candidate):
            return candidate
    for first in range(31):
        for second in range(first + 1, 32):
            candidate = word ^ (1 << first) ^ (1 << second)
            if _bch_ok(candidate):
                return candidate
    return None


def _iq_discriminator(data):
    """Return phase differences for unsigned interleaved RTL-SDR IQ."""
    values = []
    previous_i = previous_q = None
    for index in range(0, len(data) - 1, 2):
        i = data[index] - 127.5
        q = data[index + 1] - 127.5
        if previous_i is not None:
            values.append(math.atan2(i * previous_q - q * previous_i,
                                     i * previous_i + q * previous_q))
        previous_i, previous_q = i, q
    return values


def _word(bits, start):
    value = 0
    for bit in bits[start:start + 32]:
        value = (value << 1) | bit
    return value


def _decode_words(bits, sync_at, baud):
    words = []
    for index in range(sync_at + 32, len(bits) - 31, 32):
        raw = _word(bits, index)
        corrected = decode_codeword(raw)
        if corrected is None:
            break
        words.append(corrected)
        if len(words) >= 64:
            break
    if not words:
        return None
    address = None
    function = None
    message_bits = []
    text = ""
    for frame, codeword in enumerate(words):
        if codeword == IDLE_CODEWORD:
            if address is not None and message_bits:
                break
            continue
        if not (codeword & 0x80000000):
            if address is not None and message_bits:
                break
            # 18 address bits followed by the two function bits.  The batch
            # frame number is the low three capcode bits.
            address = ((codeword >> 13) & 0x3FFFF) * 8 + ((frame // 2) & 7)
            function = (codeword >> 11) & 3
            message_bits = []
        elif address is not None:
            message_bits.extend((codeword >> bit) & 1 for bit in range(30, 10, -1))
            while len(message_bits) >= 7:
                value = 0
                for bit in message_bits[:7]:
                    value = (value << 1) | bit
                del message_bits[:7]
                if value in (0, 0x0A, 0x0D):
                    if value == 0:
                        break
                elif 0x20 <= value <= 0x7E:
                    text += chr(value)
    if address is None or not text:
        return None
    return {"baud": baud, "address": address, "function": function,
            "text": text.rstrip(), "sync_at": sync_at}


def _sample_bits(discriminator, sample_rate, baud, offset, invert=False):
    samples_per_symbol = sample_rate / float(baud)
    bits = []
    position = float(offset)
    while position + samples_per_symbol <= len(discriminator):
        end = position + samples_per_symbol
        first, last = int(position), int(end)
        window = discriminator[first:last] or [0.0]
        bit = 1 if sum(window) >= 0 else 0
        bits.append(1 - bit if invert else bit)
        position = end
    return bits


def decode_iq(data, sample_rate):
    """Decode complete IQ bytes, returning validated frames and signal stats."""
    if sample_rate <= 0 or len(data) < 64 or len(data) % 2:
        return [], {"validated": False, "baud": None}
    discriminator = _iq_discriminator(data)
    frames = []
    for baud in POCSAG_BAUDS:
        sps = sample_rate / float(baud)
        # 64 timing hypotheses are enough for the wide RTL-SDR rates while
        # keeping CPU bounded; each symbol is integrated, not point sampled.
        phases = max(1, min(64, int(math.ceil(sps))))
        for phase_index in range(phases):
            offset = phase_index * sps / phases
            for invert in (False, True):
                bits = _sample_bits(discriminator, sample_rate, baud, offset, invert)
                for index in range(len(bits) - 31):
                    if _word(bits, index) != POCSAG_SYNC:
                        continue
                    frame = _decode_words(bits, index, baud)
                    if frame:
                        frames.append(frame)
                        break
                if frames and frames[-1]["baud"] == baud:
                    break
            if frames and frames[-1]["baud"] == baud:
                break
    unique = []
    seen = set()
    for frame in frames:
        key = (frame["baud"], frame["address"], frame["function"], frame["text"])
        if key not in seen:
            seen.add(key)
            unique.append(frame)
    return unique, {"validated": bool(unique),
                    "baud": unique[0]["baud"] if unique else None}


class POCSAGDecoder:
    """Small streaming facade; blocks may split anywhere in an IQ capture."""
    def __init__(self, sample_rate):
        self.sample_rate = sample_rate
        self._data = b""
        self._seen = set()

    def feed(self, data):
        self._data += data
        # Retain enough history for a full page but bound memory on a silent SDR.
        self._data = self._data[-2 * self.sample_rate:]
        frames, stats = decode_iq(self._data, self.sample_rate)
        fresh = []
        for frame in frames:
            key = (frame["baud"], frame["address"], frame["function"], frame["text"])
            if key not in self._seen:
                self._seen.add(key)
                fresh.append(frame)
        return fresh, stats


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
               min_prominence_db=6.0, validated=False):
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
    result = {"ready": True, "noise_floor_db": round(floor, 2),
             "waterfall_db": [round(value, 2) for value in db],
             "offsets_hz": offsets, "fsk_lobes": lobes,
             "calibration_candidate": None}
    # A pair of spectral lobes alone is not a calibration reference.  Permit
    # the candidate only after the BCH/sync decoder has accepted a frame.
    if validated and lobes is not None:
        result["calibration_candidate"] = {
            "center_offset_hz": lobes["center_offset_hz"],
            "ppm": lobes["center_offset_hz"] / float(channel_hz) * 1000000,
            "reason": "validated_pocsag_frame",
        }
    return result


def format_frame(frame):
    """Use the same raw line shape produced by multimon-ng's alpha decoder."""
    return ("POCSAG{baud}: Address: {address:6d}  Function: {function}  "
            "Alpha:   {text}<EOT><NUL>").format(**frame)


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
                      "decoder": "stdlib_2fsk_pocsag",
                      "pocsag_decode": "512/1200/2400_bch_alpha",
                      "calibration": "withheld_until_repeatable_reference"}), flush=True)
    decoder = POCSAGDecoder(args.sample_rate)
    try:
        while True:
            block = capture.stdout.read(args.block_bytes)
            if not block:
                detail = capture.stderr.read().decode("utf-8", "replace").strip()
                print("HIGH_POWER_FALLBACK capture stopped{}".format(
                    ": " + detail if detail else ""), flush=True)
                return 2
            frames, decode_stats = decoder.feed(block)
            result = analyze_iq(block, args.sample_rate, args.frequency,
                                args.span_hz, args.bins, args.min_prominence_db,
                                validated=decode_stats["validated"])
            result["pocsag"] = decode_stats
            print("SDR_DIAGNOSTIC " + json.dumps(result, separators=(",", ":")), flush=True)
            for frame in frames:
                print(format_frame(frame), flush=True)
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
