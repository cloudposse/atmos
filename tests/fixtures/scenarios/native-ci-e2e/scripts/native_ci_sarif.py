#!/usr/bin/env python3
"""Emit deterministic SARIF for the native CI E2E fixture."""

from __future__ import annotations

import json
import os
from pathlib import Path


RULE_ID = "ATMOS_NATIVE_CI_E2E"


def repo_relative_component_file() -> str:
    component_path = Path(os.environ.get("ATMOS_COMPONENT_PATH", ".")).resolve()
    target = component_path / "main.tf"
    workspace = os.environ.get("GITHUB_WORKSPACE")
    if workspace:
        try:
            return str(target.relative_to(Path(workspace).resolve()))
        except ValueError:
            pass
    return str(target)


def main() -> int:
    output = os.environ.get("ATMOS_OUTPUT_FILE")
    if not output:
        return 0

    sarif = {
        "version": "2.1.0",
        "$schema": "https://json.schemastore.org/sarif-2.1.0.json",
        "runs": [
            {
                "tool": {
                    "driver": {
                        "name": "native-ci-e2e-hook",
                        "rules": [
                            {
                                "id": RULE_ID,
                                "shortDescription": {
                                    "text": "Native CI hook annotation smoke test"
                                },
                                "defaultConfiguration": {"level": "warning"},
                                "properties": {"severity": "LOW"},
                            }
                        ],
                    }
                },
                "results": [
                    {
                        "ruleId": RULE_ID,
                        "level": "warning",
                        "message": {
                            "text": "Native CI hook annotation smoke test"
                        },
                        "locations": [
                            {
                                "physicalLocation": {
                                    "artifactLocation": {
                                        "uri": repo_relative_component_file()
                                    },
                                    "region": {"startLine": 1},
                                }
                            }
                        ],
                        "properties": {"severity": "LOW"},
                    }
                ],
            }
        ],
    }

    with open(output, "w", encoding="utf-8") as fp:
        json.dump(sarif, fp)
        fp.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
