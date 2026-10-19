import unittest

import sdr_calibrate


class CalibrationTests(unittest.TestCase):
    def test_scheduler_jitter_bounds_with_injected_randomness(self):
        class Clock:
            value = 100
            def __call__(self):
                return self.value

        class Rng:
            def uniform(self, low, high):
                self.args = (low, high)
                return high

        clock, rng = Clock(), Rng()
        scheduler = sdr_calibrate.CalibrationScheduler(60, 15, clock=clock, rng=rng)
        due = scheduler.schedule()
        self.assertEqual(due, 175)  # 100 + base 60 + max jitter 15
        self.assertEqual(rng.args, (0, 15))
        self.assertFalse(scheduler.due(174.99))
        self.assertTrue(scheduler.due(175))

    def test_scheduler_backoff_is_bounded_and_repeatable(self):
        class Rng:
            def uniform(self, low, high):
                return 0

        first = sdr_calibrate.CalibrationScheduler(10, 4, clock=lambda: 0,
                                                    rng=Rng(), max_backoff=25)
        second = sdr_calibrate.CalibrationScheduler(10, 4, clock=lambda: 0,
                                                     rng=Rng(), max_backoff=25)
        self.assertEqual(first.failure(0), 10)
        self.assertEqual(first.failure(0), 20)
        self.assertEqual(first.failure(0), 25)
        self.assertEqual(first.failure(0), 25)
        self.assertEqual(second.failure(0), 10)

    def test_parse_and_peak_frequency(self):
        self.assertEqual(sdr_calibrate.parse_station("local=99.9"),
                         ("local", 99_900_000.0))
        sample = "date,time,99900000,100100000,10000,1,-80,-40,-70\n"
        self.assertEqual(sdr_calibrate.strongest_frequency(sample),
                         (99_910_000.0, -40.0))

    def test_calibrate_uses_mocked_rtl_power(self):
        calls = []

        class Result:
            stdout = "date,time,99900000,100100000,10000,1,-80,-40,-70\n"

        def runner(command, **kwargs):
            calls.append(command)
            return Result()

        result = sdr_calibrate.calibrate([("local", 99_900_000)], gain="38",
                                         runner=runner)
        self.assertEqual(result["SDR_GAIN"], "38")
        self.assertAlmostEqual(result["SDR_PPM"], 100.1, places=1)
        self.assertIn("-g", calls[0])
        self.assertIn("38", calls[0])


if __name__ == "__main__":
    unittest.main()
