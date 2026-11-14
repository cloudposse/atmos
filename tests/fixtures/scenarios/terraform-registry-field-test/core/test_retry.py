"""Retry runner synchronization and early-exit evidence regressions."""
import json
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, patch

from run_retry import retry


class RetryEvidenceTest(unittest.TestCase):
    def run_case(self, backoff):
        with tempfile.TemporaryDirectory() as scratch:
            root = Path(scratch)
            dev = root / "dev.yaml"
            dev.write_text("components: {terraform: {producer: {}}}\n")
            c = SimpleNamespace(root=root, project=root, engine="terraform", binary="atmos",
                                revision="candidate", env={}, check=Mock(), save=Mock())
            proc = Mock(returncode=None if backoff else 2)
            proc.poll.side_effect = lambda: proc.returncode
            proc.send_signal.side_effect = lambda sig: setattr(proc, "returncode", 130)

            def launch(*args, **kwargs):
                kwargs["stdout"].write(b"process evidence\n")
                kwargs["stderr"].write(b"Entering retry backoff\n" if backoff else b"bad configuration\n")
                kwargs["stderr"].flush()
                if backoff:
                    (root / "trace.jsonl").write_text('{"argv":["plan"]}\n')
                return proc

            with patch("run_retry.simple", return_value=dev), patch("run_retry.subprocess.Popen", side_effect=launch):
                retry(c)
            record = json.loads((root / "retry.json").read_text())
            self.assertEqual(record["backoff_confirmed"], backoff)
            self.assertEqual(record["plan_attempts"], int(backoff))
            self.assertEqual((root / "retry.stdout").read_bytes(), b"process evidence\n")
            self.assertTrue((root / "retry.stderr").read_bytes())
            self.assertEqual(c.check.call_args_list[0].args[1], backoff)
            self.assertEqual(c.check.call_args_list[1].args[1], backoff)
            c.save.assert_called_once()
            if backoff:
                proc.send_signal.assert_called_once()
            else:
                proc.send_signal.assert_not_called()

    def test_missing_trace_preserves_failure_evidence(self):
        self.run_case(False)

    def test_signal_requires_explicit_backoff_marker(self):
        self.run_case(True)
