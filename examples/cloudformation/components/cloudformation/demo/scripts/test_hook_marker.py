"""Exercise the example hook marker without touching a shared marker file."""

import contextlib
import io
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

import hook_marker


class HookMarkerTests(unittest.TestCase):
    def test_missing_event_reports_failure_without_creating_marker(self):
        with tempfile.TemporaryDirectory() as directory:
            marker = Path(directory) / "events.log"
            stderr = io.StringIO()
            with (
                patch("sys.argv", ["hook_marker.py"]),
                patch.dict("os.environ", {"ATMOS_HOOK_MARKER_FILE": str(marker)}),
                contextlib.redirect_stderr(stderr),
            ):
                self.assertEqual(hook_marker.main(), 1)
            self.assertIn("expected the event name", stderr.getvalue())
            self.assertFalse(marker.exists())

    def test_events_append_in_order_without_replacing_existing_entries(self):
        with tempfile.TemporaryDirectory() as directory:
            marker = Path(directory) / "events.log"
            marker.write_text("existing\n", encoding="utf-8")
            events = ["before.aws/cloudformation.apply", "after.aws/cloudformation.apply"]
            for event in events:
                with (
                    patch("sys.argv", ["hook_marker.py", event]),
                    patch.dict("os.environ", {"ATMOS_HOOK_MARKER_FILE": str(marker)}),
                ):
                    self.assertEqual(hook_marker.main(), 0)
            self.assertEqual(marker.read_text(encoding="utf-8"), "existing\n" + "\n".join(events) + "\n")

    def test_unset_environment_uses_default_marker(self):
        with tempfile.TemporaryDirectory() as directory:
            marker = Path(directory) / "default.log"
            with (
                patch("sys.argv", ["hook_marker.py", "before.aws/cloudformation.diff"]),
                patch.dict("os.environ", {}, clear=True),
                patch.object(hook_marker, "DEFAULT_MARKER_FILE", str(marker)),
            ):
                self.assertEqual(hook_marker.main(), 0)
            self.assertEqual(marker.read_text(encoding="utf-8"), "before.aws/cloudformation.diff\n")


if __name__ == "__main__":
    unittest.main()
