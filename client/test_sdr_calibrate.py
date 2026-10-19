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

    def test_calibrate_validates_multiple_mocked_stations(self):
        calls = []
        outputs = {
            "99800000": "date,time,99900000,100100000,10000,1,-80,-40,-70\n",
            "101000000": "date,time,101100000,101300000,10000,1,-80,-40,-70\n",
        }

        class Result:
            def __init__(self, stdout):
                self.stdout = stdout

        def runner(command, **kwargs):
            calls.append(command)
            scan_range = command[command.index("-f") + 1]
            return Result(outputs[scan_range.split(":", 1)[0]])

        result = sdr_calibrate.calibrate(
            [("local", 99_900_000), ("news", 101_100_000)],
            gain="38", runner=runner,
        )
        self.assertEqual([item["station"] for item in result["stations"]], ["local", "news"])
        self.assertAlmostEqual(result["SDR_PPM"], 99.51, places=2)
        self.assertEqual(len(calls), 2)
        sdr_calibrate.validate_result(result, min_stations=2)

    def test_target_calibration_uses_repeatable_prominent_target(self):
        calls = []
        class Result:
            stdout = "date,time,155603223,155603423,10000,1,-80,-80,-80\n"
        def runner(command, **kwargs):
            calls.append(command)
            ppm = float(command[command.index("-p") + 1])
            Result.stdout = ("date,time,155603223,155603423,10000,1,-80,-25,-80\n"
                             if ppm == 2 else
                             "date,time,155603223,155603423,10000,1,-80,-78,-80\n")
            return Result()
        result = sdr_calibrate.calibrate_target(
            155603323, gain="38", candidates=[-1, 0, 1, 2, 3],
            repeats=2, runner=runner)
        self.assertEqual(result["SDR_PPM"], 2.0)
        self.assertEqual(len(calls), 10)

    def test_target_calibration_rejects_nonrepeatable_signal(self):
        class Result:
            stdout = ""
        state = [0]
        def runner(command, **kwargs):
            state[0] += 1
            Result.stdout = ("date,time,155603223,155603423,10000,1,-80,-20,-80\n"
                             if state[0] == 1 else
                             "date,time,155603223,155603423,10000,1,-80,-79,-80\n")
            return Result()
        with self.assertRaises(ValueError):
            sdr_calibrate.calibrate_target(155603323, candidates=[0, 1],
                                           repeats=2, runner=runner)

    def test_rejects_disagreeing_reference_stations(self):
        result = {"SDR_PPM": 0, "SDR_GAIN": "38", "stations": [
            {"ppm": 2.0}, {"ppm": 9.0},
        ]}
        with self.assertRaises(ValueError):
            sdr_calibrate.validate_result(result)

    def test_rejects_implausible_ppm(self):
        result = {"SDR_PPM": 0, "SDR_GAIN": "38", "stations": [
            {"ppm": 201.0},
        ]}
        with self.assertRaises(ValueError):
            sdr_calibrate.validate_result(result)

    def test_scheduler_applies_jitter_after_backoff(self):
        class Rng:
            def uniform(self, low, high):
                return high

        scheduler = sdr_calibrate.CalibrationScheduler(10, 3, clock=lambda: 0,
                                                        rng=Rng(), max_backoff=25)
        self.assertEqual(scheduler.failure(0), 13)
        self.assertEqual(scheduler.failure(0), 23)
        self.assertEqual(scheduler.failure(0), 28)


if __name__ == "__main__":
    unittest.main()
