package utils

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestEvaluateYqExpression_PreservesScalarTypes verifies that values selected by a yq expression keep
// their original Go types: strings stay strings (including leading zeros), integers stay integers
// (no float/exponent conversion) and floats stay floats.
func TestEvaluateYqExpression_PreservesScalarTypes(t *testing.T) {
	data := map[string]any{
		"vars": map[string]any{
			"acct_quoted":   "068007702576",
			"acct_int":      123456789012,
			"acct_leading0": 68007702576,
			"max_int64":     int64(math.MaxInt64),
			"min_int64":     int64(math.MinInt64),
			"near_max":      int64(math.MaxInt64 - 1),
			"ratio":         1.5,
			"whole_float":   2.0,
			"numeric_str":   "123456789012",
			"float_str":     "1.50",
			"exp_str":       "1e10",
			"bool_str":      "true",
			"null_str":      "null",
			"plain":         "hello",
			"tilde":         "~",
			"flag":          true,
			"nested": map[string]any{
				"acct":  "000123456789",
				"count": 42,
				"ratio": 0.25,
			},
			"list": []any{"007", 7, 7.5, "x"},
			"maps": []any{
				map[string]any{"id": "012345678901"},
				map[string]any{"id": 12345678901},
			},
		},
	}

	tests := []struct {
		name     string
		query    string
		expected any
	}{
		{"quoted leading-zero string", ".vars.acct_quoted", "068007702576"},
		{"12-digit int", ".vars.acct_int", 123456789012},
		{"11-digit int", ".vars.acct_leading0", 68007702576},
		{"max int64", ".vars.max_int64", int64(math.MaxInt64)},
		{"min int64", ".vars.min_int64", int64(math.MinInt64)},
		{"near max int64", ".vars.near_max", int64(math.MaxInt64 - 1)},
		{"real float", ".vars.ratio", 1.5},
		{"numeric-looking string", ".vars.numeric_str", "123456789012"},
		{"float-looking string", ".vars.float_str", "1.50"},
		{"exponent-looking string", ".vars.exp_str", "1e10"},
		{"bool-looking string", ".vars.bool_str", "true"},
		{"null-looking string", ".vars.null_str", "null"},
		{"plain string", ".vars.plain", "hello"},
		{"tilde string", ".vars.tilde", "~"},
		{"bool", ".vars.flag", true},
		{"nested string", ".vars.nested.acct", "000123456789"},
		{"nested int", ".vars.nested.count", 42},
		{"nested float", ".vars.nested.ratio", 0.25},
		{
			"nested map",
			".vars.nested",
			map[string]any{"acct": "000123456789", "count": 42, "ratio": 0.25},
		},
		{"list", ".vars.list", []any{"007", 7, 7.5, "x"}},
		{
			"list of maps",
			".vars.maps",
			[]any{map[string]any{"id": "012345678901"}, map[string]any{"id": 12345678901}},
		},
		{"list element string", ".vars.list[0]", "007"},
		{"list element int", ".vars.list[1]", 7},
		{"string function result", ".vars.acct_int | tostring", "123456789012"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EvaluateYqExpression(&schema.AtmosConfiguration{}, data, tt.query)
			require.NoError(t, err)
			// The YAML decoder returns int on 64-bit platforms for values that fit; compare by value.
			if want, ok := tt.expected.(int64); ok {
				require.IsType(t, 0, got)
				assert.Equal(t, want, int64(got.(int)))
				return
			}
			assert.Equal(t, tt.expected, got)
			assert.IsType(t, tt.expected, got)
		})
	}
}

// TestEvaluateYqExpression_PreservesScalarTypesFromYAMLInput exercises the same behavior with data that was
// decoded from YAML (the way stack manifests reach describe commands), including unquoted values.
func TestEvaluateYqExpression_PreservesScalarTypesFromYAMLInput(t *testing.T) {
	const manifest = `
vars:
  quoted: "068007702576"
  unquoted_leading_zero: 068007702576
  twelve: 123456789012
  big: 9223372036854775807
  ratio: 1.5
`
	data, err := UnmarshalYAML[map[string]any](manifest)
	require.NoError(t, err)

	tests := []struct {
		key      string
		expected any
	}{
		{"quoted", "068007702576"},
		{"unquoted_leading_zero", "068007702576"},
		{"twelve", 123456789012},
		{"big", 9223372036854775807},
		{"ratio", 1.5},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			// The decoded manifest itself must already hold the exact value.
			assert.Equal(t, tt.expected, data["vars"].(map[string]any)[tt.key])

			got, err := EvaluateYqExpression(&schema.AtmosConfiguration{}, data, ".vars."+tt.key)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
			assert.IsType(t, tt.expected, got)
		})
	}
}

// TestEvaluateYqExpression_EmptyStringResultIsNil documents that an empty-string scalar result is reported as nil
// (the established behavior for default expressions such as `.key // ""`), while a non-empty string is not.
func TestEvaluateYqExpression_EmptyStringResultIsNil(t *testing.T) {
	data := map[string]any{"empty": "", "set": "x"}

	got, err := EvaluateYqExpression(&schema.AtmosConfiguration{}, data, `.missing // ""`)
	require.NoError(t, err)
	assert.Nil(t, got)

	got, err = EvaluateYqExpression(&schema.AtmosConfiguration{}, data, ".empty")
	require.NoError(t, err)
	assert.Nil(t, got)

	got, err = EvaluateYqExpression(&schema.AtmosConfiguration{}, data, ".set")
	require.NoError(t, err)
	assert.Equal(t, "x", got)
}

// TestYAMLOutput_TopLevelStringIsRaw verifies the exact YAML text for describe-style output: a top-level string is
// printed verbatim (as yq does), everything else keeps its normal YAML encoding. ConvertToYAML itself stays strict,
// because it also feeds the yq evaluator, which needs type-preserving input.
func TestYAMLOutput_TopLevelStringIsRaw(t *testing.T) {
	tests := []struct {
		name     string
		data     any
		expected string
		strict   string
	}{
		{"string with leading zero", "068007702576", "068007702576\n", "\"068007702576\"\n"},
		{"numeric-looking string", "123", "123\n", "\"123\"\n"},
		{"bool-looking string", "true", "true\n", "\"true\"\n"},
		{"integer", 42, "42\n", "42\n"},
		{"float", 1.5, "1.5\n", "1.5\n"},
		{"map keeps quoting", map[string]any{"a": "007"}, "a: \"007\"\n", "a: \"007\"\n"},
		{"list keeps quoting", []any{"007", 7}, "- \"007\"\n- 7\n", "- \"007\"\n- 7\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := convertToYAMLOutput(tt.data, YAMLOptions{Indent: 2})
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)

			highlighted, err := GetHighlightedYAML(&schema.AtmosConfiguration{}, tt.data)
			require.NoError(t, err)
			if _, isString := tt.data.(string); isString {
				assert.Equal(t, tt.expected, highlighted)
			}

			strict, err := ConvertToYAML(tt.data)
			require.NoError(t, err)
			assert.Equal(t, tt.strict, strict)
		})
	}
}
