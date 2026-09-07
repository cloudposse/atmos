package secret

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// parseSecretsFile reads KEY=VALUE pairs from a file (or stdin when path is "-"). The format is
// chosen by the format argument: "env" (default), "json", or "yaml". JSON/YAML must be a flat
// object of string values.
func parseSecretsFile(path, format string) (map[string]string, error) {
	defer perf.Track(nil, "secret.parseSecretsFile")()

	var reader io.Reader
	if path == "-" {
		reader = os.Stdin
	} else {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("failed to open %q: %w", path, err)
		}
		defer f.Close()
		reader = f
	}

	switch format {
	case "json":
		return parseJSONSecrets(reader)
	case "env", "":
		return parseEnvSecrets(reader)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedFormat, format)
	}
}

// parseEnvSecrets parses KEY=VALUE lines, ignoring blanks and # comments. An inline comment starts
// at an unquoted # preceded by whitespace, so URLs/fragments and quoted values retain their #.
func parseEnvSecrets(reader io.Reader) (map[string]string, error) {
	out := make(map[string]string)
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(key), "export "))
		value = strings.TrimSpace(stripEnvInlineComment(value))
		out[key] = strings.Trim(value, `"'`)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

//nolint:revive // The state machine is clearer than splitting quote and escape handling across helpers.
func stripEnvInlineComment(value string) string {
	var quote rune
	escaped := false
	for index, char := range value {
		if escaped {
			escaped = false
			continue
		}
		if char == '\\' && quote == '"' {
			escaped = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			}
			continue
		}
		if char == '"' || char == '\'' {
			quote = char
			continue
		}
		if char == '#' && index > 0 && (value[index-1] == ' ' || value[index-1] == '\t') {
			return value[:index]
		}
	}
	return value
}

// parseJSONSecrets parses a flat JSON object of string values.
func parseJSONSecrets(reader io.Reader) (map[string]string, error) {
	var raw map[string]any
	if err := json.NewDecoder(reader).Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to parse JSON secrets: %w", err)
	}
	out := make(map[string]string, len(raw))
	for k, v := range raw {
		out[k] = fmt.Sprintf("%v", v)
	}
	return out, nil
}

// sortedKeys returns the map keys in deterministic order.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
