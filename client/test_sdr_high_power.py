import unittest

import sdr_high_power


class HighPowerAnalyzerTests(unittest.TestCase):
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
