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
