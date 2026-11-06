"""Exercise the checked-in CLI recorder without external tools or accounts."""

import contextlib
import errno
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import textwrap
import unittest
from unittest import mock

sys.path.insert(0, str(Path(__file__).parent))
from cast_checks import assert_recorded_command  # noqa: E402


class RecordedCommandTest(unittest.TestCase):
    def check(self, header, command):
        path = Path(self.enterContext(tempfile.TemporaryDirectory())) / "x.cast"
        path.write_text(json.dumps(header) + "\n")
        assert_recorded_command(path, command)

    def test_accepts_command_with_program_name(self):
        self.check({"version": 3, "command": "atmos --help"}, "atmos --help")
        self.check({"version": 3, "command": "atmos ai ask --help"}, "atmos ai ask --help")

    def test_accepts_widened_help_flag(self):
        self.check(
            {"version": 3, "command": "atmos aws cloudformation source pull --help=all"},
            "atmos aws cloudformation source pull --help",
        )

    def test_rejects_command_without_program_name(self):
        with self.assertRaisesRegex(SystemExit, "must be 'atmos --help'"):
            self.check({"version": 3, "command": "--help"}, "atmos --help")
        with self.assertRaisesRegex(SystemExit, "must be 'atmos ai --help'"):
            self.check({"version": 3, "command": "ai --help"}, "atmos ai --help")

    def test_rejects_missing_command(self):
        with self.assertRaisesRegex(SystemExit, "no recorded command"):
            self.check({"version": 3, "title": "atmos --help"}, "atmos --help")

    def test_help_widening_does_not_hide_other_mismatches(self):
        with self.assertRaises(SystemExit):
            self.check({"version": 3, "command": "atmos plugin --help=all"}, "atmos plan --help")


class ScreengrabRecorderTest(unittest.TestCase):
    def setUp(self):
        manifest = Path(__file__).parent / "atmos.d/screengrabs/cli.yaml"
        source = manifest.read_text().split("        name: record\n", 1)[1]
        source = source.split("        script: |\n", 1)[1]
        source = textwrap.dedent(source.split("\n      - type:", 1)[0])
        placeholder = "{{ .Flags.filter | quote }}"
        self.assertEqual(source.count(placeholder), 1)
        self.script = compile(
            source.replace(placeholder, repr("atmos policy check --help")),
            str(manifest), "exec",
        )
        self.root = Path(self.enterContext(tempfile.TemporaryDirectory())).resolve()
        working = self.root / "demo/casts"
        working.mkdir(parents=True)
        self.enterContext(contextlib.chdir(working))
        self.enterContext(mock.patch("sys.path", list(sys.path)))
        self.output = self.root / "website/static/casts/screengrabs"
        self.output.mkdir(parents=True)
        self.destination = self.output / "atmos-policy-check--help.cast"
        self.destination.write_text("previous recording\n")
        self.argv = []

    def record(self, argv, **kwargs):
        self.argv.append(argv)
        self.assertEqual(argv[:4], ["atmos", "policy", "check", "--help"])
        self.assertEqual(kwargs["cwd"], self.root / ".context/cast-fixtures/demo-stacks")
        temporary = Path(argv[-1].removeprefix("--cast="))
        temporary.write_text(
            json.dumps({"version": 2, "width": 90, "height": 36}) + "\n"
            + json.dumps([0, "o", f"Recorded from {self.root}"]) + "\n"
        )
        return subprocess.CompletedProcess(argv, 0, "", "")

    def test_publish_on_destination_filesystem_and_rerun(self):
        replace = os.replace

        def same_filesystem_replace(source, destination):
            # Model a runner where the system temp directory is on another mount.
            if not Path(source).is_relative_to(self.output):
                raise OSError(errno.EXDEV, "Invalid cross-device link")
            replace(source, destination)

        with mock.patch("subprocess.run", side_effect=self.record), mock.patch(
            "os.replace", side_effect=same_filesystem_replace
        ), contextlib.redirect_stdout(io.StringIO()):
            exec(self.script, {})
            first = self.destination.read_text()
            exec(self.script, {})

        self.assertEqual(len(self.argv), 2)
        self.assertEqual(self.destination.read_text(), first)
        self.assertIn("/absolute/path/to/repo", first)
        self.assertNotIn(str(self.root), first)
        self.assertEqual(list(self.output.iterdir()), [self.destination])

    def test_failed_recording_preserves_existing_cast_and_cleans_temporary_files(self):
        def failed_recording(argv, **kwargs):
            self.record(argv, **kwargs)
            return subprocess.CompletedProcess(argv, 1, "", "recorder failed")

        with (
            mock.patch("subprocess.run", side_effect=failed_recording),
            mock.patch("os.replace") as replace,
            contextlib.redirect_stdout(io.StringIO()),
            contextlib.redirect_stderr(io.StringIO()),
        ):
            with self.assertRaisesRegex(SystemExit, "screengrab command failed"):
                exec(self.script, {})

        replace.assert_not_called()
        self.assertEqual(self.destination.read_text(), "previous recording\n")
        self.assertEqual(list(self.output.iterdir()), [self.destination])


if __name__ == "__main__":
    unittest.main()
