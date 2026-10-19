import hashlib
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock

import configure


class OverlayTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="reserve test ")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.source = self.root / "go" / "src" / "runtime" / "stack.go"
        self.source.parent.mkdir(parents=True)
        self.original = b"// retain this comment\nstackSystem = goos.IsWindows*4096 + otherPlatforms\n"
        self.source.write_bytes(self.original)
        self.environment = self.root / "environment"
        self.environment.write_text("EXISTING=value\n", encoding="utf-8")
        self.env = {"GOVERSION": configure.VERSION, "GOHOSTOS": "windows", "GOHOSTARCH": "amd64",
                    "GOOS": "windows", "GOARCH": "amd64", "GOROOT": str(self.root / "go"),
                    "GOFLAGS": "-trimpath"}
        digest = hashlib.sha256(self.original).hexdigest()
        patch = mock.patch.object(configure, "SOURCE_SHA256", digest)
        patch.start()
        self.addCleanup(patch.stop)

    def apply(self, size=11567):
        return configure.configure(self.env, size, self.root, self.environment)

    def assert_unchanged(self):
        self.assertEqual(self.source.read_bytes(), self.original)
        self.assertEqual(self.environment.read_text(), "EXISTING=value\n")
        self.assertEqual(list(self.root.glob("atmos-windows-runtime-*")), [])

    def test_overlay_preserves_installed_source_and_environment(self):
        overlay = self.apply()
        replacements = json.loads(overlay.read_text())["Replace"]
        self.assertEqual(list(replacements), [str(self.source)])
        replacement = Path(replacements[str(self.source)])
        self.assertEqual(replacement.read_bytes(), self.original.replace(b"4096", b"16384"))
        self.assertEqual(self.source.read_bytes(), self.original)
        self.assertEqual(self.environment.read_text(),
                         f'EXISTING=value\nGOFLAGS=-trimpath "-overlay={overlay.as_posix()}"\n')

    def test_other_go_release_fails_before_writing(self):
        self.env["GOVERSION"] = "go1.27.1"
        with self.assertRaisesRegex(ValueError, "Go version"):
            self.apply()
        self.assert_unchanged()

    def test_source_integrity_mismatch_fails_before_writing(self):
        self.source.write_bytes(self.original + b"// unexpected modification\n")
        with self.assertRaisesRegex(ValueError, "source does not match"):
            self.apply()
        self.assertEqual(self.environment.read_text(), "EXISTING=value\n")
        self.assertEqual(list(self.root.glob("atmos-windows-runtime-*")), [])

    def test_unsupported_platform_fails_before_writing(self):
        for name, value in [("GOHOSTOS", "linux"), ("GOHOSTARCH", "arm64"),
                            ("GOOS", "linux"), ("GOARCH", "386")]:
            with self.subTest(name=name), mock.patch.dict(self.env, {name: value}):
                with self.assertRaisesRegex(ValueError, "Windows/amd64"):
                    self.apply()
                self.assert_unchanged()

    def test_invalid_context_size_fails_before_writing(self):
        for size in [0, -1, configure.RESERVE - configure.DISPATCH_MARGIN + 1]:
            with self.subTest(size=size):
                with self.assertRaisesRegex(ValueError, "context exceeds"):
                    self.apply(size)
                self.assert_unchanged()

    def test_existing_overlay_is_not_overridden(self):
        self.env["GOFLAGS"] = '-trimpath "-overlay=other file.json"'
        with self.assertRaisesRegex(ValueError, "existing Go overlay"):
            self.apply()
        self.assert_unchanged()

    def test_environment_injection_is_rejected(self):
        for flags in ["-trimpath\nOTHER=value", "-trimpath\rOTHER=value"]:
            with self.subTest(flags=flags):
                self.env["GOFLAGS"] = flags
                with self.assertRaisesRegex(ValueError, "Invalid Go environment"):
                    self.apply()
                self.assert_unchanged()

    def test_repeated_configuration_uses_private_directories(self):
        first = self.apply()
        second = self.apply()
        self.assertNotEqual(first.parent, second.parent)
        self.assertTrue(first.exists())
        self.assertEqual(self.source.read_bytes(), self.original)


if __name__ == "__main__":
    unittest.main()
