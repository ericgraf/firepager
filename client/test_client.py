import os
import signal
import tempfile
import unittest

import client


class ConfirmChannel:
    def __init__(self, result=True):
        self.result = result
        self.calls = []

    def basic_publish(self, **kwargs):
        self.calls.append(kwargs)
        return self.result


class SpoolTests(unittest.TestCase):
    def test_messages_survive_close_and_are_fifo(self):
        with tempfile.TemporaryDirectory() as directory:
            path = os.path.join(directory, "spool.sqlite3")
            first = {"raw_text": "one"}
            spool = client.SQLiteSpool(path)
            first_id = spool.enqueue(first)
            spool.enqueue({"raw_text": "two"})
            spool.close()

            reopened = client.SQLiteSpool(path)
            self.assertEqual(reopened.first(), (first_id, first))
            reopened.acknowledge(first_id)
            self.assertEqual(reopened.first()[1]["raw_text"], "two")
            reopened.close()

    def test_only_confirmed_publish_is_removed(self):
        with tempfile.TemporaryDirectory() as directory:
            spool = client.SQLiteSpool(os.path.join(directory, "spool.sqlite3"))
            spool.enqueue({"raw_text": "page"})
            channel = ConfirmChannel()
            connection = type("Connection", (), {"is_open": True})()
            self.assertTrue(client.drain_spool(spool, connection, channel))
            self.assertEqual(spool.size(), 0)
            self.assertEqual(channel.calls[0]["routing_key"], client.QUEUE)
            spool.close()

    def test_negative_confirmation_keeps_message(self):
        with tempfile.TemporaryDirectory() as directory:
            spool = client.SQLiteSpool(os.path.join(directory, "spool.sqlite3"))
            spool.enqueue({"raw_text": "page"})
            connection = type("Connection", (), {"is_open": True})()
            with self.assertRaises(RuntimeError):
                client.drain_spool(spool, connection, ConfirmChannel(False))
            self.assertEqual(spool.size(), 1)
            spool.close()


class HardeningTests(unittest.TestCase):
    def test_invalid_calibration_environment_is_rejected(self):
        with self.assertRaises(ValueError):
            client.calibration_config({"SDR_CALIBRATE_INTERVAL": "not-a-number"})
        with self.assertRaises(ValueError):
            client.calibration_config({"SDR_CALIBRATE_JITTER": "-1"})
        with self.assertRaises(ValueError):
            client.calibration_config({"SDR_CALIBRATE_MIN_STATIONS": "0"})

    def test_calibration_environment_parses_station_and_schedule(self):
        config = client.calibration_config({
            "SDR_CALIBRATE_STATIONS": "local=99.9,news=101.1",
            "SDR_CALIBRATE_INTERVAL": "60",
            "SDR_CALIBRATE_JITTER": "15",
            "SDR_CALIBRATE_MIN_STATIONS": "2",
            "SDR_CALIBRATE_ON_START": "true",
        })
        self.assertEqual(config["stations"], [("local", 99_900_000.0), ("news", 101_100_000.0)])
        self.assertEqual((config["interval"], config["jitter"], config["min_stations"]), (60.0, 15.0, 2))
        self.assertTrue(config["on_start"])

    def test_calibration_file_is_last_known_good_and_atomic(self):
        with tempfile.TemporaryDirectory() as directory:
            path = os.path.join(directory, "nested", "calibration.json")
            value = {"SDR_PPM": 1.25, "SDR_GAIN": "38", "stations": []}
            client.save_calibration(path, value)
            self.assertEqual(client.load_calibration(path), ("1.25", "38"))
            with open(path, encoding="utf-8") as source:
                self.assertEqual(__import__("json").load(source), value)

            with open(path, "w", encoding="utf-8") as output:
                output.write("not json")
            self.assertIsNone(client.load_calibration(path))

    def test_sdr_command_includes_frequency_correction_and_gain(self):
        old = client.subprocess.Popen
        commands = []

        class Process:
            pass

        client.subprocess.Popen = lambda command, **kwargs: (commands.append(command) or Process())
        try:
            client.start_sdr("155603323", ppm=-2.5, gain="38")
        finally:
            client.subprocess.Popen = old
        self.assertIn("-p -2.5", commands[0])
        self.assertIn("-g 38", commands[0])

    def test_credentials_are_redacted(self):
        self.assertNotIn("secret", client.redact_url("amqp://pager:secret@broker:5672/"))

    def test_process_group_uses_term_then_kill_on_timeout(self):
        class FakeProcess:
            pid = 123
            stdout = None
            waits = 0
            def poll(self):
                return None
            def wait(self, timeout=None):
                self.waits += 1
                if self.waits == 1:
                    raise __import__("subprocess").TimeoutExpired("fake", timeout)
                return 0

        sent = []
        old_killpg = os.killpg
        os.killpg = lambda pid, sig: sent.append((pid, sig))
        try:
            client.terminate_process_group(FakeProcess(), timeout=1)
        finally:
            os.killpg = old_killpg
        self.assertEqual(sent, [(123, signal.SIGTERM), (123, signal.SIGKILL)])


if __name__ == "__main__":
    unittest.main()
