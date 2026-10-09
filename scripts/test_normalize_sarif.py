"""Regression tests for GitHub's rejection of duplicate SARIF stack entries."""

import copy
import json
import tempfile
import unittest
from pathlib import Path

from normalize_sarif import normalize, normalize_file


class NormalizeSarifTests(unittest.TestCase):
    """Keep all findings and distinct call stacks when normalizing a report."""

    def test_duplicate_stacks_preserve_findings_and_distinct_paths(self):
        first = {"message": {"text": "one"}, "frames": [{"location": {"id": 1}}]}
        same = {"frames": [{"location": {"id": 1}}], "message": {"text": "one"}}
        second = {"message": {"text": "two"}, "frames": [{"location": {"id": 2}}]}
        result = {"ruleId": "GO-2026-example", "stacks": [first, same, second, first]}
        report = {
            "version": "2.1.0",
            "runs": [
                {"tool": {"driver": {"name": "govulncheck"}}, "results": [result]},
                {"results": [{"ruleId": "other", "stacks": [second, second]}]},
            ],
        }
        expected = copy.deepcopy(report)
        expected["runs"][0]["results"][0]["stacks"] = [first, second]
        expected["runs"][1]["results"][0]["stacks"] = [second]
        self.assertEqual(normalize(report), expected)
        self.assertEqual(normalize(report), expected)

    def test_empty_and_already_unique_reports_are_unchanged(self):
        for report in (
            {},
            {"runs": []},
            {"runs": [{"results": []}]},
            {"runs": [{"results": [{"ruleId": "example"}]}]},
            {"runs": [{"results": [{"stacks": []}]}]},
            {"runs": [{"results": [{"stacks": [{"frames": []}]}]}]},
        ):
            with self.subTest(report=report):
                self.assertEqual(normalize(copy.deepcopy(report)), report)

    def test_file_round_trip_preserves_metadata_and_numbers(self):
        report = {"version": "2.1.0", "properties": {"text": "é", "large": 2**64}}
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "report.sarif"
            path.write_text(json.dumps(report), encoding="utf-8")
            normalize_file(path)
            self.assertEqual(json.loads(path.read_text(encoding="utf-8")), report)

    def test_malformed_input_is_rejected_without_overwriting_it(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "report.sarif"
            original = '{"runs": ['
            path.write_text(original, encoding="utf-8")
            with self.assertRaises(json.JSONDecodeError):
                normalize_file(path)
            self.assertEqual(path.read_text(encoding="utf-8"), original)


if __name__ == "__main__":
    unittest.main()
