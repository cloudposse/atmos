"""Replace the recording workspace with a stable demo path."""

import json
import re
from pathlib import Path

path = Path("../../website/static/casts/cli/command-discovery.cast")
project = Path("../../.context/cast-fixtures/command-discovery/project-path.txt").read_text().strip()
events = [json.loads(line) for line in path.read_text().splitlines()]
output = [event for event in events[1:] if event[1] in ("o", "e")]
text = "".join(event[2] for event in output)
# The success message can wrap a long checkout path across styled lines and
# separate output events. Replace only that exact path, preserving event timing.
gap = r"(?:\x1b\[[0-?]*[ -/]*[@-~]|\r?\n[ \t]*)*"
matches = list(re.finditer(gap.join(re.escape(char) for char in project), text))
offset = 0
for event in output:
    original = event[2]
    end = offset + len(original)
    cursor = offset
    chunks = []
    for match in matches:
        if match.end() <= offset or match.start() >= end:
            continue
        start = max(offset, match.start())
        chunks.append(original[cursor - offset:start - offset])
        if match.start() >= offset:
            chunks.append("/demo")
        cursor = min(end, match.end())
    chunks.append(original[cursor - offset:])
    event[2] = "".join(chunks)
    offset = end
path.write_text("\n".join(json.dumps(event, separators=(",", ":")) for event in events) + "\n")
