package ui

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// TestFormatSimpleValue_DefaultType covers the default branch of formatSimpleValue: a type
// that is none of string/bool/float64/map/slice must fall through to plain %v formatting.
func TestFormatSimpleValue_DefaultType(t *testing.T) {
	result := formatSimpleValue(42, false)
	assert.Equal(t, "42", result)
}

// TestFormatSimpleComplexValue_MarshalError verifies formatSimpleValue falls back to
// "(complex)" instead of propagating a JSON marshal error - NaN cannot be JSON-encoded.
func TestFormatSimpleComplexValue_MarshalError(t *testing.T) {
	result := formatSimpleValue(map[string]interface{}{"x": math.NaN()}, false)
	assert.Equal(t, "(complex)", result)
}

// TestFormatSimpleComplexValue_Truncation verifies long complex values are truncated to the
// display width with an ellipsis, rather than overflowing the tree layout.
func TestFormatSimpleComplexValue_Truncation(t *testing.T) {
	big := map[string]interface{}{}
	for i := 0; i < 20; i++ {
		big[fmt.Sprintf("key%d", i)] = i
	}

	result := formatSimpleValue(big, false)

	assert.True(t, strings.HasSuffix(result, "..."), "long complex values must be truncated with an ellipsis")
	assert.Len(t, result, 40)
}

// TestValuesEqual_MarshalError verifies valuesEqual treats a JSON marshal failure (e.g. an
// unencodable channel value) as "not equal" instead of panicking.
func TestValuesEqual_MarshalError(t *testing.T) {
	a := make(chan int)
	b := make(chan int)
	assert.False(t, valuesEqual(a, b))
}

// TestSafeFdToInt covers both the normal conversion path and the defensive overflow guard.
func TestSafeFdToInt(t *testing.T) {
	tests := []struct {
		name     string
		fd       uintptr
		expected int
	}{
		{"typical stdout fd", 1, 1},
		{"zero fd", 0, 0},
		{"overflow beyond MaxInt returns -1", uintptr(math.MaxInt) + 1, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, safeFdToInt(tt.fd))
		})
	}
}

// TestGetContrastTextColor_LowercaseHex verifies lowercase hex digits (a-f) parse identically
// to uppercase - parseHexComponent has separate switch branches for each case.
func TestGetContrastTextColor_LowercaseHex(t *testing.T) {
	assert.Equal(t, getContrastTextColor("#FF0000"), getContrastTextColor("#ff0000"))
	assert.Equal(t, theme.ColorWhite, getContrastTextColor("#0000ff"))
}

// TestGetContrastTextColor_InvalidHexCharacter verifies a correctly-sized (6 char) hex string
// containing a non-hex character falls back to white rather than propagating the parse error.
func TestGetContrastTextColor_InvalidHexCharacter(t *testing.T) {
	assert.Equal(t, theme.ColorWhite, getContrastTextColor("#GGGGGG"))
}

// TestParseHexComponent covers digit, lowercase, uppercase, and invalid-character branches.
func TestParseHexComponent(t *testing.T) {
	tests := []struct {
		name      string
		hex       string
		expected  int64
		expectErr bool
	}{
		{"digits only", "42", 0x42, false},
		{"lowercase letters", "ff", 0xff, false},
		{"uppercase letters", "FF", 0xff, false},
		{"mixed case", "aB", 0xab, false},
		{"invalid character", "zz", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseHexComponent(tt.hex)
			if tt.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestIsComplexValue covers the map/slice true branches alongside the existing nil/default
// coverage, since only the false paths were exercised before.
func TestIsComplexValue(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected bool
	}{
		{"nil", nil, false},
		{"map", map[string]interface{}{"a": 1}, true},
		{"slice", []interface{}{1, 2}, true},
		{"string", "hello", false},
		{"number", float64(5), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isComplexValue(tt.value))
		})
	}
}

// TestCollapseIfNeeded_NoCollapseWhenMaxLinesUnset verifies the default (maxLines <= 0) means
// "show everything", matching Terraform's own uncollapsed diff behavior.
func TestCollapseIfNeeded_NoCollapseWhenMaxLinesUnset(t *testing.T) {
	lines := []string{"a", "b", "c"}
	result := collapseIfNeeded(lines, 0)
	assert.Equal(t, lines, result)
}

// TestCollapseIfNeeded_NoCollapseWhenUnderLimit verifies lines already within the limit pass
// through unchanged.
func TestCollapseIfNeeded_NoCollapseWhenUnderLimit(t *testing.T) {
	lines := []string{"a", "b"}
	result := collapseIfNeeded(lines, 5)
	assert.Equal(t, lines, result)
}

// TestCollapseIfNeeded_CollapsesLongOutput verifies the head/tail collapse math: for
// maxLines=10, showHead=6 and showTail=3, with an "omitted" marker line in between.
func TestCollapseIfNeeded_CollapsesLongOutput(t *testing.T) {
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = fmt.Sprintf("line%d", i)
	}

	result := collapseIfNeeded(lines, 10)

	require.Len(t, result, 6+1+3)
	assert.Equal(t, "line0", result[0])
	assert.Equal(t, "line5", result[5])
	assert.Contains(t, result[6], "omitted")
	assert.Equal(t, "line17", result[7])
	assert.Equal(t, "line19", result[9])
}

// TestCollapseIfNeeded_TooFewLinesToCollapse verifies the safety guard: when showHead+showTail
// would meet or exceed the actual line count, collapsing is skipped entirely rather than
// producing an "omitted" marker that hides zero or negative lines.
func TestCollapseIfNeeded_TooFewLinesToCollapse(t *testing.T) {
	lines := []string{"a", "b"}
	result := collapseIfNeeded(lines, 1)
	assert.Equal(t, lines, result)
}

// TestFormatComplexValue_Nil verifies a nil value renders as no lines at all.
func TestFormatComplexValue_Nil(t *testing.T) {
	assert.Nil(t, formatComplexValue(nil, nil))
}

// TestFormatComplexValue_ValidMap verifies a map renders as indented JSON split into lines,
// using the shared Atmos JSON formatter.
func TestFormatComplexValue_ValidMap(t *testing.T) {
	result := formatComplexValue(map[string]interface{}{"key": "value"}, nil)
	assert.Equal(t, []string{"{", `   "key": "value"`, "}"}, result)
}

// TestFormatComplexValue_MarshalError verifies a value that cannot be JSON-marshaled (a
// channel) falls back to a single %v-formatted line instead of dropping the value entirely.
func TestFormatComplexValue_MarshalError(t *testing.T) {
	value := map[string]interface{}{"ch": make(chan int)}

	result := formatComplexValue(value, nil)

	require.Len(t, result, 1)
	assert.Contains(t, result[0], "ch")
}

// TestFormatComplexValue_WithAtmosConfig exercises the atmosConfig != nil branch. In a non-TTY
// test process without ForceColor, HighlightCodeWithConfig returns the code unchanged, so the
// output must match the no-highlighting case exactly.
func TestFormatComplexValue_WithAtmosConfig(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}

	result := formatComplexValue(map[string]interface{}{"key": "value"}, cfg)

	assert.Equal(t, []string{"{", `   "key": "value"`, "}"}, result)
}
