#!/usr/bin/env python3
"""
hook_marker.py — appends the firing hook's event name to a marker file.

Used by the `demo` component's `hooks:` block (see
stacks/catalog/demo.yaml) to live-verify which aws/cloudformation verbs
actually fire hook events. Atmos wires only `diff`/`apply`/`delete`
(plus their `plan`/`deploy` aliases) to hook events today — every other
verb (`validate`, `output`, `changeset *`, `drift *`, `get *`, `fmt`,
`tree`, `logs`, `watch`, `stackset *`, `list`, `backend *`, `source *`)
does not, so running every verb and diffing this marker's contents
against that claim is a direct, executable check rather than trusting
the docs/code comment.

The marker path is read from ATMOS_HOOK_MARKER_FILE (set by the caller
running the verification), defaulting to a fixed temp path so the hook
still does something sane if invoked without it.
"""

import os
import sys

DEFAULT_MARKER_FILE = "/tmp/atmos-cfn-hooks-marker.log"


def main() -> int:
    if len(sys.argv) < 2:
        print("hook_marker.py: expected the event name as argv[1]", file=sys.stderr)
        return 1

    event = sys.argv[1]
    marker_file = os.environ.get("ATMOS_HOOK_MARKER_FILE", DEFAULT_MARKER_FILE)
    with open(marker_file, "a", encoding="utf-8") as fp:
        fp.write(event + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
