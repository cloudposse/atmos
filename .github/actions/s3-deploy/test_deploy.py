import importlib.util
import json
from pathlib import Path
import tempfile
import unittest
from unittest import mock


SCRIPT = Path(__file__).with_name("deploy.py")
SPEC = importlib.util.spec_from_file_location("s3_deploy", SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class DeployTest(unittest.TestCase):
    def test_text_content_types_include_utf8(self):
        self.assertEqual(MODULE.content_type("index.html"), "text/html; charset=utf-8")
        self.assertEqual(MODULE.content_type("assets/app.js"), "application/javascript; charset=utf-8")
        self.assertEqual(MODULE.content_type("image.png"), "image/png")

    def test_manifest_is_content_based_and_deterministic(self):
        with tempfile.TemporaryDirectory() as temp_name:
            root = Path(temp_name)
            (root / "nested").mkdir()
            (root / "nested" / "file.txt").write_text("hello\n", encoding="utf-8")
            first = MODULE.build_manifest(root)
            (root / "nested" / "file.txt").touch()
            second = MODULE.build_manifest(root)

        self.assertEqual(first, second)
        self.assertEqual(first["version"], 1)
        self.assertEqual(first["files"]["nested/file.txt"]["size"], 6)

    def test_diff_uploads_changed_and_deletes_only_unprotected_paths(self):
        old = {
            "version": 1,
            "files": {
                "same.txt": {"sha256": "same"},
                "changed.txt": {"sha256": "old"},
                "removed.txt": {"sha256": "gone"},
                "img/demos/demo.mp4": {"sha256": "protected"},
            },
        }
        new = {
            "version": 1,
            "files": {
                "same.txt": {"sha256": "same"},
                "changed.txt": {"sha256": "new"},
                "added.txt": {"sha256": "added"},
            },
        }

        changed, deleted = MODULE.manifest_diff(
            old, new, ("img/demos/*",)
        )

        self.assertEqual(changed, ["added.txt", "changed.txt"])
        self.assertEqual(deleted, ["removed.txt"])

    def test_second_unchanged_deploy_performs_zero_aws_writes(self):
        with tempfile.TemporaryDirectory() as temp_name:
            root = Path(temp_name)
            (root / "index.html").write_text("hello\n", encoding="utf-8")
            manifest = MODULE.build_manifest(root)
            with (
                mock.patch.object(MODULE, "load_remote_manifest", return_value=manifest),
                mock.patch.object(MODULE, "run_aws") as run_aws,
                mock.patch.object(
                    MODULE.sys,
                    "argv",
                    ["deploy.py", str(root), "s3://example/site/"],
                ),
            ):
                result = MODULE.main()

        self.assertEqual(result, 0)
        run_aws.assert_not_called()


if __name__ == "__main__":
    unittest.main()
