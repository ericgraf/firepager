import unittest
import math

import sdr_high_power


class HighPowerAnalyzerTests(unittest.TestCase):
    @staticmethod
    def _codeword(data20, message=False):
        value = (0x80000000 if message else 0) | (data20 << 11)
        work = value >> 1
        for bit in range(30, 9, -1):
            if work & (1 << bit):
                work ^= sdr_high_power.POCSAG_BCH << (bit - 10)
        value |= (work & 0x3ff) << 1
        if value.bit_count() & 1:
            value |= 1
        return value

    @classmethod
    def _fixture(cls, text="TEST", baud=2400, sample_rate=24000):
        words = [sdr_high_power.POCSAG_SYNC,
                 cls._codeword((1234 << 2) | 1)]
        payload = [ord(char) for char in text]
        bits = []
        for char in payload:
            bits.extend((char >> bit) & 1 for bit in range(6, -1, -1))
        while len(bits) % 20:
            bits.append(0)
        for start in range(0, len(bits), 20):
            value = 0
            for bit in bits[start:start + 20]:
                value = (value << 1) | bit
            words.append(cls._codeword(value, message=True))
        words.append(sdr_high_power.IDLE_CODEWORD)
        output = bytearray()
        phase = 0.0
        generated_samples = 0
        symbol_number = 0
        for word in words:
            for bit in range(31, -1, -1):
                phase_step = 0.08 if (word >> bit) & 1 else -0.08
                target = round((symbol_number + 1) * sample_rate / float(baud))
                for sample in range(target - generated_samples):
                    output.extend((int(127.5 + 90 * math.cos(phase)) & 255,
                                   int(127.5 + 90 * math.sin(phase)) & 255))
                    phase += phase_step
                generated_samples = target
                symbol_number += 1
        return bytes(output)

    def test_deterministic_iq_fixture_decodes_alpha_frame(self):
        frames, stats = sdr_high_power.decode_iq(self._fixture(), 24000)
        self.assertTrue(stats["validated"])
        self.assertEqual(frames[0]["baud"], 2400)
        self.assertEqual(frames[0]["address"], 1234 * 8)
        self.assertEqual(frames[0]["text"], "TEST")

    def test_all_supported_baud_rates(self):
        for baud in (512, 1200, 2400):
            frames, stats = sdr_high_power.decode_iq(self._fixture("OK", baud), 24000)
            self.assertTrue(stats["validated"], baud)
            self.assertEqual(frames[0]["baud"], baud)
            self.assertEqual(frames[0]["text"], "OK")

    def test_noise_and_malformed_streams_are_rejected(self):
        frames, stats = sdr_high_power.decode_iq(bytes([127, 127]) * 5000, 24000)
        self.assertFalse(frames)
        self.assertFalse(stats["validated"])
        malformed = bytearray(self._fixture())
        # Damage three symbols in one BCH codeword; the two-error correction
        # must not turn a malformed stream into a page.
        for start in (320 + 12, 320 + 32, 320 + 52):
            for index in range(start, start + 20):
                malformed[index] ^= 0x7f
        frames, stats = sdr_high_power.decode_iq(bytes(malformed), 24000)
        self.assertFalse(frames)
        self.assertFalse(stats["validated"])

    def test_short_or_noise_block_never_calibrates(self):
        result = sdr_high_power.analyze_iq(bytes([127, 127]) * 128, 240000, 155610000)
        self.assertIsNone(result["calibration_candidate"])

    def test_analyzer_exposes_waterfall_without_claiming_decode(self):
        result = sdr_high_power.analyze_iq(bytes([127, 127]) * 256, 240000, 155610000)
        self.assertTrue(result["ready"])
        self.assertEqual(len(result["waterfall_db"]), 25)
        self.assertIsNone(result["calibration_candidate"])

    def test_mode_defaults_to_legacy(self):
        import client
        self.assertEqual(client.calibration_config({})["mode"], "legacy")

    def test_high_power_command_uses_one_shared_capture_owner(self):
        import client
        commands = []
        old = client.subprocess.Popen
        client.subprocess.Popen = lambda command, **kwargs: (commands.append(command) or object())
        try:
            client.start_sdr("155610000", ppm=1, gain="38", mode="high_power")
        finally:
            client.subprocess.Popen = old
        self.assertIn("sdr_high_power.py", commands[0])
        self.assertIn("--sample-rate 240000", commands[0])
        self.assertNotIn("multimon-ng", commands[0])


if __name__ == "__main__":
    unittest.main()
