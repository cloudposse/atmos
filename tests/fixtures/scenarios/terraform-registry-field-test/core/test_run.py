"""Runner self-tests that need no Atmos or engine binaries.

Usage: python3 -m unittest discover -s tests/fixtures/scenarios/terraform-registry-field-test/core -p "test_run.py"
"""
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from run import SOURCE, Case, exit_status, reject_nested_output


class FakeCase:
    def __init__(self, name, *results):
        self.root = Path(name)
        self.checks = [{"name": f"check-{i}", "passed": passed, "detail": None} for i, passed in enumerate(results)]


class ExitStatusTest(unittest.TestCase):
    def test_all_checks_pass(self):
        self.assertEqual(exit_status([FakeCase("a", True, True), FakeCase("b", True)]), 0)

    def test_any_failed_check_in_any_case_fails(self):
        self.assertEqual(exit_status([FakeCase("a", True), FakeCase("b", True, False, True)]), 1)

    def test_no_checks_passes(self):
        self.assertEqual(exit_status([FakeCase("a")]), 0)


class NestedOutputTest(unittest.TestCase):
    def assert_rejected(self, root):
        with self.assertRaises(ValueError):
            Case(root, "baseline", "terraform", sys.executable)

    def test_rejects_source_itself_and_descendants(self):
        self.assert_rejected(SOURCE)
        self.assert_rejected(SOURCE / "runs" / "terraform-baseline")
        self.assertFalse((SOURCE / "runs").exists(), "rejection must happen before the case root is created")

    def test_rejects_relative_path_inside_source(self):
        cwd = os.getcwd()
        os.chdir(SOURCE)
        try:
            self.assert_rejected(Path("runs") / "terraform-baseline")
            self.assert_rejected(Path("components") / ".." / "runs")
        finally:
            os.chdir(cwd)
        self.assertFalse((SOURCE / "runs").exists())

    def test_rejects_symlink_into_source(self):
        with tempfile.TemporaryDirectory() as scratch:
            link = Path(scratch) / "fixture"
            link.symlink_to(SOURCE, target_is_directory=True)
            self.assert_rejected(link / "runs" / "terraform-baseline")
        self.assertFalse((SOURCE / "runs").exists())

    def test_allows_sibling_with_shared_prefix(self):
        sibling = SOURCE.parent / (SOURCE.name + "-runs") / "case"
        self.assertEqual(reject_nested_output(sibling), sibling)

    def test_copies_fixture_outside_source(self):
        with tempfile.TemporaryDirectory() as scratch:
            case = Case(Path(scratch) / "runs" / "terraform-baseline", "baseline", "terraform", sys.executable)
            self.assertTrue((case.project / "atmos.yaml").is_file())
            self.assertTrue((case.project / "stacks" / "deploy" / "dev.yaml").is_file())
            self.assertFalse((case.project / "run.py").exists())
            self.assertFalse((case.project / "test_run.py").exists())
            log = subprocess.run(["git", "log", "--oneline"], cwd=case.project, capture_output=True, text=True, check=True)
            self.assertIn("fixture", log.stdout)


if __name__ == "__main__":
    unittest.main()
