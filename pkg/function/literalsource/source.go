// Package literalsource preserves scalar !literal values across stack merging and evaluation.
package literalsource

import (
	"encoding/base64"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// The versioned envelope is a string so existing YAML, merge, and cache code can carry it
// without a custom value type. Its payload contains no template syntax or YAML function call.
const prefix = "ATMOS_LITERAL_V1:"

// Encode preserves the exact scalar text in an internal string envelope.
func Encode(value string) string {
	defer perf.Track(nil, "literalsource.Encode")()

	return prefix + base64.RawURLEncoding.EncodeToString([]byte(value)) + ":END"
}

// Decode recognizes a complete canonical envelope and returns its original text.
func Decode(value string) (string, bool) {
	defer perf.Track(nil, "literalsource.Decode")()

	if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, ":END") {
		return value, false
	}
	payload := strings.TrimSuffix(strings.TrimPrefix(value, prefix), ":END")
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil || base64.RawURLEncoding.EncodeToString(decoded) != payload {
		return value, false
	}
	return string(decoded), true
}

// Restore returns a detached tree with literal envelopes decoded exactly once. Never
// recursively decode a recovered string: it may itself intentionally resemble an envelope.
func Restore(value any) any {
	defer perf.Track(nil, "literalsource.Restore")()

	if !contains(value) {
		return value
	}
	return restore(value)
}

func restore(value any) any {
	switch value := value.(type) {
	case string:
		decoded, _ := Decode(value)
		return decoded
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, child := range value {
			result[key] = restore(child)
		}
		return result
	case []any:
		result := make([]any, len(value))
		for i, child := range value {
			result[i] = restore(child)
		}
		return result
	default:
		return value
	}
}

// RestoreMap is the map-preserving form of Restore.
func RestoreMap(value map[string]any) map[string]any {
	defer perf.Track(nil, "literalsource.RestoreMap")()

	if value == nil {
		return nil
	}
	return Restore(value).(map[string]any)
}

func contains(value any) bool {
	switch value := value.(type) {
	case string:
		return strings.HasPrefix(value, prefix)
	case map[string]any:
		for _, child := range value {
			if contains(child) {
				return true
			}
		}
	case []any:
		for _, child := range value {
			if contains(child) {
				return true
			}
		}
	}
	return false
}
