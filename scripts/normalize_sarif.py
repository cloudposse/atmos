"""Remove duplicate stacks emitted by govulncheck without dropping findings."""

import json
import sys
from pathlib import Path


def normalize(report):
    """Deduplicate each result's stacks, preserving their order and contents."""
    for run in report.get("runs", []):
        for result in run.get("results", []):
            if "stacks" not in result:
                continue
            unique = []
            for stack in result["stacks"]:
                if stack not in unique:
                    unique.append(stack)
            result["stacks"] = unique
    return report


def normalize_file(path):
    """Parse the whole report before replacing it, leaving invalid JSON intact."""
    report = json.loads(path.read_text(encoding="utf-8"))
    normalized = json.dumps(normalize(report), indent=2, ensure_ascii=False)
    path.write_text(normalized + "\n", encoding="utf-8")


if __name__ == "__main__":
    normalize_file(Path(sys.argv[1]))
