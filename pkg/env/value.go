package env

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Format strings for numeric types.
const (
	fmtInt   = "%d"
	fmtFloat = "%v"
	fmtBool  = "%t"
)

// ValueToString converts any value to its string representation.
// Complex types (maps, slices) are JSON-encoded.
// Boolean values are converted to "true" or "false".
// Numeric values are converted using their default string representation.
// Nil values return an empty string.
func ValueToString(value any) string {
	defer perf.Track(nil, "env.ValueToString")()

	if value == nil {
		return ""
	}

	switch v := value.(type) {
	case string:
		return v
	case bool:
		return fmt.Sprintf(fmtBool, v)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprintf(fmtInt, v)
	case float32:
		return formatFloat32(v)
	case float64:
		return formatFloat64(v)
	default:
		return formatJSON(v)
	}
}

// formatFloat32 formats a float32, using integer format for whole numbers.
func formatFloat32(v float32) string {
	if v == float32(int64(v)) {
		return fmt.Sprintf(fmtInt, int64(v))
	}
	return fmt.Sprintf(fmtFloat, v)
}

// formatFloat64 formats a float64, using integer format for whole numbers.
func formatFloat64(v float64) string {
	if v == float64(int64(v)) {
		return fmt.Sprintf(fmtInt, int64(v))
	}
	return fmt.Sprintf(fmtFloat, v)
}

// formatJSON encodes a value as JSON, falling back to %v format on error.
func formatJSON(v any) string {
	jsonBytes, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf(fmtFloat, v)
	}
	return string(jsonBytes)
}

// EscapeSingleQuotes escapes single quotes for safe single-quoted shell literals.
// The pattern ' -> '\” closes the current quote, inserts an escaped quote,
// and reopens the quote.
func EscapeSingleQuotes(s string) string {
	defer perf.Track(nil, "env.EscapeSingleQuotes")()

	return strings.ReplaceAll(s, "'", "'\\''")
}
