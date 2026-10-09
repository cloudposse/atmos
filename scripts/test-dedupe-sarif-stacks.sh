#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work_dir="$(mktemp -d)"
trap 'rm -rf "$work_dir"' EXIT
filter="$root/scripts/dedupe-sarif-stacks.jq"

# Include equivalent objects with different key order, distinct metadata, and
# reversed frames. Only the complete duplicate stack may be removed.
cat > "$work_dir/input.json" <<'JSON'
{
	"version": "2.1.0",
	"runs": [
		{
			"tool": {"driver": {"name": "govulncheck"}},
			"results": [
				{
					"ruleId": "GO-TEST",
					"level": "error",
					"message": {"text": "Keep this finding"},
					"stacks": [
						{"message": {"text": "trace"}, "frames": [{"module": "b"}, {"module": "a"}]},
						{"frames": [{"module": "b"}, {"module": "a"}], "message": {"text": "trace"}},
						{"message": {"text": "other"}, "frames": [{"module": "b"}, {"module": "a"}]},
						{"message": {"text": "trace"}, "frames": [{"module": "a"}, {"module": "b"}]}
					],
					"codeFlows": [{"message": {"text": "unchanged"}}]
				},
				{"ruleId": "NO-STACKS", "message": {"text": "Keep this too"}},
				{"ruleId": "EMPTY-STACKS", "stacks": []}
			]
		},
		{"results": [{"ruleId": "SECOND-RUN", "stacks": [{"frames": []}, {"frames": []}]}]},
		{"results": []},
		{"tool": {"driver": {"name": "no-results"}}}
	]
}
JSON

jq -f "$filter" "$work_dir/input.json" > "$work_dir/output.json"
# Compare the entire document, not just stack counts: no finding, property,
# distinct stack, or frame ordering may change.
jq -e --slurpfile actual "$work_dir/output.json" '
	del(.runs[0].results[0].stacks[1], .runs[1].results[0].stacks[1]) == $actual[0]
' "$work_dir/input.json" > /dev/null

# Running the filter again is a no-op, including for results without stacks.
jq -f "$filter" "$work_dir/output.json" > "$work_dir/twice.json"
cmp "$work_dir/output.json" "$work_dir/twice.json"
printf '{"version":"2.1.0","runs":[]}' | jq -e -f "$filter" > /dev/null

# Malformed output must fail rather than silently dropping scanner findings.
if printf 'invalid json' | jq -f "$filter" > /dev/null 2>&1; then
	echo "expected malformed JSON to fail" >&2
	exit 1
fi

echo "SARIF stack deduplication regression tests passed."
