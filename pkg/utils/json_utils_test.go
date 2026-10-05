package utils

import (
	"encoding/json"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrintAsJson(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		data        interface{}
		wantErr     bool
	}{
		{
			name: "simple map data",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						SyntaxHighlighting: schema.SyntaxHighlighting{
							Enabled:     true,
							Formatter:   "terminal",
							Theme:       "dracula",
							LineNumbers: true,
							Wrap:        false,
						},
					},
				},
			},
			data: map[string]any{
				"key": "value",
			},
			wantErr: false,
		},
		{
			name: "nested data structure",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						SyntaxHighlighting: schema.SyntaxHighlighting{
							Enabled: false,
							Theme:   "default",
							Wrap:    true,
						},
					},
				},
			},
			data: map[string]interface{}{
				"string": "value",
				"number": 42,
				"nested": map[string]interface{}{
					"array": []string{"one", "two", "three"},
					"bool":  true,
				},
			},
			wantErr: false,
		},
		{
			name: "nil data",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{},
			},
			data:    nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := PrintAsJSON(tt.atmosConfig, tt.data)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConvertToJson(t *testing.T) {
	tests := []struct {
		name    string
		input   interface{}
		want    string
		wantErr bool
	}{
		{
			name: "simple map",
			input: map[string]string{
				"key": "value",
			},
			want:    `{"key":"value"}`,
			wantErr: false,
		},
		{
			name: "complex structure",
			input: map[string]interface{}{
				"string": "text",
				"number": 123,
				"array":  []int{1, 2, 3},
				"nested": map[string]bool{
					"enabled": true,
				},
			},
			want:    `{"array":[1,2,3],"nested":{"enabled":true},"number":123,"string":"text"}`,
			wantErr: false,
		},
		{
			name:    "nil input",
			input:   nil,
			want:    "null",
			wantErr: false,
		},
		{
			name: "slice",
			input: []string{
				"one",
				"two",
				"three",
			},
			want:    `["one","two","three"]`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ConvertToJSON(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)

			// Compare JSON by parsing both strings and comparing the results
			var expected, actual interface{}
			err = json.Unmarshal([]byte(tt.want), &expected)
			assert.NoError(t, err)

			err = json.Unmarshal([]byte(result), &actual)
			assert.NoError(t, err)

			assert.Equal(t, expected, actual)
		})
	}
}

// TestConvertToJson_NestedArrayIndentation is a regression test for a jsoniter MarshalIndent
// bug: nesting a []interface{} more than one level deep (array -> object -> array) resets the
// inner array's element indentation instead of continuing to nest it, while the surrounding
// object's own indentation stays correct. Verified against a minimal standalone repro of
// jsoniter's Froze().MarshalIndent - stdlib encoding/json.MarshalIndent does not have this bug.
// This shape mirrors a real Terraform plan attribute (e.g. CloudFront's
// ordered_cache_behavior, which nests allowed_methods/cached_methods arrays inside its own
// array of objects) that rendered with visibly broken indentation in the plan tree.
func TestConvertToJson_NestedArrayIndentation(t *testing.T) {
	input := map[string]interface{}{
		"ordered_cache_behavior": []interface{}{
			map[string]interface{}{
				"allowed_methods": []interface{}{"GET", "HEAD"},
				"compress":        true,
			},
		},
	}

	result, err := ConvertToJSON(input)
	require.NoError(t, err)

	// Compare line-by-line against stdlib's MarshalIndent, which is known-correct for this
	// shape: every line must be indented exactly as the equivalent stdlib output.
	want, err := json.MarshalIndent(input, "", "   ")
	require.NoError(t, err)

	assert.Equal(t, string(want), result)
}

// TestJSONToMapOfInterfaces covers decoding JSON documents into a map, including the
// error paths for malformed JSON and non-object top-level values.
func TestJSONToMapOfInterfaces(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      schema.AtmosSectionMapType
		wantErr   bool
		wantErrIs error
	}{
		{
			name:  "simple object",
			input: `{"hello": "world"}`,
			want:  schema.AtmosSectionMapType{"hello": "world"},
		},
		{
			name:  "nested object",
			input: `{"a": {"b": [1, 2, 3]}, "c": true}`,
			want: schema.AtmosSectionMapType{
				"a": map[string]any{"b": []any{float64(1), float64(2), float64(3)}},
				"c": true,
			},
		},
		{
			name:  "empty object",
			input: `{}`,
			want:  schema.AtmosSectionMapType{},
		},
		{
			name:    "invalid json",
			input:   "Not JSON",
			wantErr: true,
		},
		{
			name:    "top-level array is not an object",
			input:   `["a", "b"]`,
			wantErr: true,
		},
		{
			name:      "top-level null is not an object",
			input:     "null",
			wantErr:   true,
			wantErrIs: ErrJSONTopLevelNotObject,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := JSONToMapOfInterfaces(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, result)
				if tt.wantErrIs != nil {
					assert.ErrorIs(t, err, tt.wantErrIs)
				}
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, tt.want, result)
		})
	}
}

// TestPrintAsJSONSimple tests the fast-path JSON printing without syntax highlighting.
func TestPrintAsJSONSimple(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		data        interface{}
		wantErr     bool
	}{
		{
			name: "simple map data",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{},
			},
			data: map[string]any{
				"key": "value",
			},
			wantErr: false,
		},
		{
			name: "nested data structure",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{},
			},
			data: map[string]interface{}{
				"string": "value",
				"number": 42,
				"nested": map[string]interface{}{
					"array": []string{"one", "two", "three"},
					"bool":  true,
				},
			},
			wantErr: false,
		},
		{
			name: "nil data",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{},
			},
			data:    nil,
			wantErr: false,
		},
		{
			name: "complex array",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{},
			},
			data: []map[string]any{
				{"id": 1, "name": "first"},
				{"id": 2, "name": "second"},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := PrintAsJSONSimple(tt.atmosConfig, tt.data)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
