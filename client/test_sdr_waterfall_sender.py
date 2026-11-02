import json
import unittest

import sdr_waterfall_sender as sender


class WaterfallSenderTests(unittest.TestCase):
    def test_parse_rtl_power_csv_row(self):
        csv_line = "2026-10-31, 18:00:05, 155538000, 155658000, 234.38, 4140, " + ",".join(
            ["-80.0", "-45.0", "-78.0"]
        )
        row = sender.parse_rtl_power_row(csv_line)
        self.assertEqual(row["captured_at"], "2026-10-31T18:00:05Z")
        self.assertEqual(row["start_hz"], 155538000)
        self.assertEqual(row["power_db"], [-80.0, -45.0, -78.0])

    def test_rejects_non_finite_bins_and_bad_rows(self):
        self.assertIsNone(sender.parse_rtl_power_row("header,not,a,spectrum"))
        row = "2026-10-31, 18:00:05, 155538000, 155658000, 250, 1, -80, NaN"
        self.assertIsNone(sender.parse_rtl_power_row(row))

    def test_truncates_rtl_power_rounding_edge_bin(self):
        bins = ",".join(["-80.0"] * 513)
        row = "2026-10-31, 18:00:05, 155538000, 155658000, 234.38, 1, " + bins
        parsed = sender.parse_rtl_power_row(row)
        self.assertEqual(len(parsed["power_db"]), sender.MAX_BINS)

    def test_stream_parser_retains_partial_line_until_newline(self):
        row_text = "2026-10-31, 18:00:05, 155538000, 155658000, 250, 4140, -80, -45\n"
        rows = []
        pending = bytearray()
        split = len(row_text) // 2
        sender.append_spectrum_lines(rows, pending, row_text[:split].encode())
        self.assertEqual(rows, [])
        sender.append_spectrum_lines(rows, pending, row_text[split:].encode())
        self.assertEqual(len(rows), 1)
        self.assertEqual(rows[0]["power_db"], [-80.0, -45.0])
        self.assertEqual(pending, b"")

    def test_snapshot_has_consistent_frequency_grid_and_bounded_rows(self):
        rows = [{"captured_at": "2026-10-31T18:00:%02dZ" % i,
                 "start_hz": 155538000.0, "step_hz": 250.0,
                 "power_db": [-80.0, -45.0]} for i in range(125)]
        snapshot = sender.make_snapshot("remote-pi-sdr", rows)
        self.assertEqual(snapshot["receiver_id"], "remote-pi-sdr")
        self.assertEqual(len(snapshot["rows"]), sender.MAX_ROWS)
        self.assertEqual(snapshot["captured_at"], rows[-1]["captured_at"])
        self.assertEqual(json.loads(json.dumps(snapshot))["start_hz"], 155538000.0)

    def test_snapshot_rejects_mismatched_frequency_grids(self):
        rows = [{"captured_at": "2026-10-31T18:00:00Z", "start_hz": 1.0,
                 "step_hz": 1.0, "power_db": [-80.0]},
                {"captured_at": "2026-10-31T18:00:05Z", "start_hz": 2.0,
                 "step_hz": 1.0, "power_db": [-80.0]}]
        with self.assertRaises(ValueError):
            sender.make_snapshot("remote-pi-sdr", rows)

    def test_persistent_capture_command_uses_manual_receiver_settings(self):
        args = sender.parse_args([])
        command = sender.rtl_power_command(args)
        self.assertEqual(command[:3], ["stdbuf", "-oL", "rtl_power"])
        self.assertIn("-f", command)
        self.assertIn(sender.DEFAULT_RANGE_HZ, command)
        self.assertIn("-i", command)
        self.assertIn("-p", command)
        self.assertIn("-g", command)
        self.assertNotIn("-1", command)
        self.assertEqual(command[-1], "-")


if __name__ == "__main__":
    unittest.main()
