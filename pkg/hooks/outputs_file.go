package hooks

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"

	errUtils "github.com/cloudposse/atmos/errors"
)

// ReadOutputsFile reads the file pointed to by ATMOS_OUTPUTS and returns its
// contents as a map. Two formats are accepted, auto-detected by the first
// non-whitespace byte:
//
//   - JSON object (starts with `{`): { "agent_id": "...", "env_id": "..." }
//   - Shell KEY=VALUE lines (one per line):  agent_id=...\nenv_id=...
//
// JSON allows nested values and typed primitives. The shell form is for cheap
// custom commands that just `echo KEY=VALUE >> "$ATMOS_OUTPUTS"` — values are
// always strings in that form.
//
// An empty or missing file is not an error: an empty map is returned. Callers
// upstream report "output X not found" when they try to look up a specific key.
func ReadOutputsFile(path string) (map[string]any, error) {
	if path == "" {
		return map[string]any{}, nil
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("%w: %s: %w", errUtils.ErrReadOutputsFile, path, err)
	}

	if len(strings.TrimSpace(string(raw))) == 0 {
		return map[string]any{}, nil
	}

	if isJSONObject(raw) {
		return parseJSONOutputs(raw)
	}
	return parseKeyValueOutputs(raw)
}

// isJSONObject returns true if the first non-whitespace byte of `raw` is `{`.
func isJSONObject(raw []byte) bool {
	for _, r := range string(raw) {
		if unicode.IsSpace(r) {
			continue
		}
		return r == '{'
	}
	return false
}

func parseJSONOutputs(raw []byte) (map[string]any, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("%w: invalid JSON: %w", errUtils.ErrReadOutputsFile, err)
	}
	return m, nil
}

// parseKeyValueOutputs parses simple `KEY=VALUE` lines. Blank lines and lines
// starting with `#` are skipped. Surrounding double or single quotes around the
// value are stripped. Anything more elaborate (multi-line values, escapes)
// belongs in JSON.
func parseKeyValueOutputs(raw []byte) (map[string]any, error) {
	out := map[string]any{}
	for lineno, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		eq := strings.IndexByte(trimmed, '=')
		if eq <= 0 {
			return nil, fmt.Errorf(
				"%w: line %d: expected KEY=VALUE, got %q",
				errUtils.ErrReadOutputsFile, lineno+1, line,
			)
		}
		key := strings.TrimSpace(trimmed[:eq])
		val := strings.TrimSpace(trimmed[eq+1:])
		val = strings.Trim(val, `"'`)
		out[key] = val
	}
	return out, nil
}
