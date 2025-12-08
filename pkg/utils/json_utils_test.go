package utils

import (
	"encoding/json"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
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
