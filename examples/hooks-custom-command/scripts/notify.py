#!/usr/bin/env python3
"""
notify.py — example custom hook command.

Atmos exports a small env-var contract to every hook subprocess:

    ATMOS_COMPONENT_PATH  — on-disk path to the terraform module
    ATMOS_STACK           — stack name (e.g., "prod")
    ATMOS_COMPONENT       — component name (e.g., "vpc")
    ATMOS_OUTPUT_FILE     — temp file path the hook should write
                             structured output to (consumed by the
                             kind's ResultHandler if any)
    ATMOS_OUTPUT_DIR      — directory containing ATMOS_OUTPUT_FILE

This script demonstrates the pattern by writing a markdown summary to
ATMOS_OUTPUT_FILE. With `format: markdown` declared on the hook (see
the stack manifest), Atmos renders that summary to the terminal —
nothing else required.

The script is intentionally minimal: real-world custom commands here
might post a Slack/Teams notification, file a Jira ticket, append to a
deployment log, or trigger an internal compliance webhook.
"""

from __future__ import annotations

import datetime
import os
import sys


def env(name: str, default: str = "(unset)") -> str:
    """Read an Atmos env var with a visible placeholder when unset."""
    return os.environ.get(name, default)


def main() -> int:
    out = env("ATMOS_OUTPUT_FILE")
    if not out:
        # Without an output file we have nowhere to put the markdown.
        # Atmos doesn't require us to write one; the kind: command engine
        # will just have an empty Artifact if we skip this. But for the
        # demo to render something, we need the file set.
        print("notify.py: ATMOS_OUTPUT_FILE not set; nothing to render",
              file=sys.stderr)
        return 0

    body = f"""## custom-command demo

Hook fired for **{env("ATMOS_COMPONENT")}** in **{env("ATMOS_STACK")}**.

| Field | Value |
|---|---|
| Component path | `{env("ATMOS_COMPONENT_PATH")}` |
| Output file | `{env("ATMOS_OUTPUT_FILE")}` |
| Output dir | `{env("ATMOS_OUTPUT_DIR")}` |
| Timestamp (UTC) | `{datetime.datetime.now(datetime.timezone.utc).isoformat()}` |

This markdown body is what Atmos rendered to your terminal via
`ui.MarkdownMessage`. The same bytes would flow to Atmos Pro (when
connected) and to PR comments — _format symmetry_ across every consumer.
"""

    with open(out, "w", encoding="utf-8") as fp:
        fp.write(body)

    # Stay silent here — Atmos streams the subprocess stdout straight to
    # the terminal, so any "wrote file" diagnostic is noise that sits
    # awkwardly between the styled "Running hooks" log line and the
    # rendered markdown that follows. The markdown body IS the message.
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
