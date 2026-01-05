package generate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Test renderTemplate function.
func TestRenderTemplate_Success(t *testing.T) {
	tests := []struct {
		name            string
		templateName    string
		templateStr     string
		context         map[string]any
		expectedContent string
	}{
		{
			name:            "simple variable substitution",
			templateName:    "test",
			templateStr:     "Hello {{ .name }}!",
			context:         map[string]any{"name": "World"},
			expectedContent: "Hello World!",
		},
		{
			name:            "multiple variables",
			templateName:    "multi",
			templateStr:     "{{ .greeting }} {{ .name }}!",
			context:         map[string]any{"greeting": "Hi", "name": "User"},
			expectedContent: "Hi User!",
		},
		{
			name:            "nested context",
			templateName:    "nested",
			templateStr:     "Region: {{ .vars.region }}",
			context:         map[string]any{"vars": map[string]any{"region": "us-west-2"}},
			expectedContent: "Region: us-west-2",
		},
		{
			name:            "empty template",
			templateName:    "empty",
			templateStr:     "",
			context:         map[string]any{},
			expectedContent: "",
		},
		{
			name:            "no variables in template",
			templateName:    "static",
			templateStr:     "static content",
			context:         map[string]any{},
			expectedContent: "static content",
		},
		{
			name:            "nil context",
			templateName:    "nilctx",
			templateStr:     "static",
			context:         nil,
			expectedContent: "static",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := renderTemplate(tt.templateName, tt.templateStr, tt.context)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedContent, string(result))
		})
	}
}

func TestRenderTemplate_ParseError(t *testing.T) {
	// Invalid template syntax.
	_, err := renderTemplate("bad", "{{ .name", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "template parse error")
}

func TestRenderTemplate_ExecutionError(t *testing.T) {
	// Template references undefined function.
	_, err := renderTemplate("bad", "{{ undefinedFunc }}", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "template")
}

// Test toCtyValue function.
func TestToCtyValue(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected cty.Value
		wantErr  bool
	}{
		{
			name:     "string value",
			input:    "hello",
			expected: cty.StringVal("hello"),
			wantErr:  false,
		},
		{
			name:     "empty string",
			input:    "",
			expected: cty.StringVal(""),
			wantErr:  false,
		},
		{
			name:     "bool true",
			input:    true,
			expected: cty.BoolVal(true),
			wantErr:  false,
		},
		{
			name:     "bool false",
			input:    false,
			expected: cty.BoolVal(false),
			wantErr:  false,
		},
		{
			name:     "int value",
			input:    42,
			expected: cty.NumberIntVal(42),
			wantErr:  false,
		},
		{
			name:     "int zero",
			input:    0,
			expected: cty.NumberIntVal(0),
			wantErr:  false,
		},
		{
			name:     "negative int",
			input:    -10,
			expected: cty.NumberIntVal(-10),
			wantErr:  false,
		},
		{
			name:     "int64 value",
			input:    int64(9223372036854775807),
			expected: cty.NumberIntVal(9223372036854775807),
			wantErr:  false,
		},
		{
			name:     "float64 value",
			input:    3.14,
			expected: cty.NumberFloatVal(3.14),
			wantErr:  false,
		},
		{
			name:     "float64 zero",
			input:    0.0,
			expected: cty.NumberFloatVal(0.0),
			wantErr:  false,
		},
		{
			name:     "nil value",
			input:    nil,
			expected: cty.NullVal(cty.DynamicPseudoType),
			wantErr:  false,
		},
		{
			name:    "unsupported type (complex)",
			input:   complex(1, 2),
			wantErr: true,
		},
		{
			name:    "unsupported type (chan)",
			input:   make(chan int),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toCtyValue(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrUnsupportedInputType)
			} else {
				require.NoError(t, err)
				assert.True(t, tt.expected.RawEquals(result), "expected %v, got %v", tt.expected, result)
			}
		})
	}
}

// Test sliceToCtyTuple function.
func TestSliceToCtyTuple(t *testing.T) {
	tests := []struct {
		name     string
		input    []any
		validate func(t *testing.T, result cty.Value)
		wantErr  bool
	}{
		{
			name:  "empty slice",
			input: []any{},
			validate: func(t *testing.T, result cty.Value) {
				assert.True(t, result.RawEquals(cty.EmptyTupleVal))
			},
			wantErr: false,
		},
		{
			name:  "single string",
			input: []any{"hello"},
			validate: func(t *testing.T, result cty.Value) {
				assert.True(t, result.Type().IsTupleType())
				assert.Equal(t, 1, result.LengthInt())
			},
			wantErr: false,
		},
		{
			name:  "mixed types",
			input: []any{"hello", 42, true, 3.14},
			validate: func(t *testing.T, result cty.Value) {
				assert.True(t, result.Type().IsTupleType())
				assert.Equal(t, 4, result.LengthInt())
			},
			wantErr: false,
		},
		{
			name:  "nested slice",
			input: []any{[]any{"a", "b"}, []any{1, 2}},
			validate: func(t *testing.T, result cty.Value) {
				assert.True(t, result.Type().IsTupleType())
				assert.Equal(t, 2, result.LengthInt())
			},
			wantErr: false,
		},
		{
			name:    "slice with unsupported type",
			input:   []any{complex(1, 2)},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := sliceToCtyTuple(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				tt.validate(t, result)
			}
		})
	}
}

// Test mapToCtyObject function.
func TestMapToCtyObject(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		validate func(t *testing.T, result cty.Value)
		wantErr  bool
	}{
		{
			name:  "empty map",
			input: map[string]any{},
			validate: func(t *testing.T, result cty.Value) {
				assert.True(t, result.RawEquals(cty.EmptyObjectVal))
			},
			wantErr: false,
		},
		{
			name:  "single string value",
			input: map[string]any{"key": "value"},
			validate: func(t *testing.T, result cty.Value) {
				assert.True(t, result.Type().IsObjectType())
				val := result.GetAttr("key")
				assert.Equal(t, "value", val.AsString())
			},
			wantErr: false,
		},
		{
			name:  "mixed value types",
			input: map[string]any{"str": "hello", "num": 42, "bool": true},
			validate: func(t *testing.T, result cty.Value) {
				assert.True(t, result.Type().IsObjectType())
				assert.Equal(t, "hello", result.GetAttr("str").AsString())
			},
			wantErr: false,
		},
		{
			name:  "nested map",
			input: map[string]any{"outer": map[string]any{"inner": "value"}},
			validate: func(t *testing.T, result cty.Value) {
				assert.True(t, result.Type().IsObjectType())
				outer := result.GetAttr("outer")
				assert.True(t, outer.Type().IsObjectType())
			},
			wantErr: false,
		},
		{
			name:    "map with unsupported value type",
			input:   map[string]any{"bad": complex(1, 2)},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := mapToCtyObject(tt.input)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				tt.validate(t, result)
			}
		})
	}
}

// Test serializeByExtension function.
func TestSerializeByExtension(t *testing.T) {
	tests := []struct {
		name        string
		filename    string
		content     map[string]any
		context     map[string]any
		validateFn  func(t *testing.T, result []byte)
		wantErr     bool
		errContains string
	}{
		{
			name:     "json extension",
			filename: "config.json",
			content:  map[string]any{"key": "value"},
			context:  map[string]any{},
			validateFn: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), `"key": "value"`)
			},
			wantErr: false,
		},
		{
			name:     "JSON uppercase extension",
			filename: "config.JSON",
			content:  map[string]any{"key": "value"},
			context:  map[string]any{},
			validateFn: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), `"key"`)
			},
			wantErr: false,
		},
		{
			name:     "yaml extension",
			filename: "config.yaml",
			content:  map[string]any{"key": "value"},
			context:  map[string]any{},
			validateFn: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), "key: value")
			},
			wantErr: false,
		},
		{
			name:     "yml extension",
			filename: "config.yml",
			content:  map[string]any{"key": "value"},
			context:  map[string]any{},
			validateFn: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), "key: value")
			},
			wantErr: false,
		},
		{
			name:     "hcl extension",
			filename: "config.hcl",
			content:  map[string]any{"key": "value"},
			context:  map[string]any{},
			validateFn: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), "key")
				assert.Contains(t, string(result), "value")
			},
			wantErr: false,
		},
		{
			name:     "tf extension",
			filename: "locals.tf",
			content:  map[string]any{"locals": map[string]any{"env": "prod"}},
			context:  map[string]any{},
			validateFn: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), "locals")
			},
			wantErr: false,
		},
		{
			name:     "unknown extension defaults to JSON",
			filename: "config.unknown",
			content:  map[string]any{"key": "value"},
			context:  map[string]any{},
			validateFn: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), `"key"`)
			},
			wantErr: false,
		},
		{
			name:     "no extension defaults to JSON",
			filename: "config",
			content:  map[string]any{"key": "value"},
			context:  map[string]any{},
			validateFn: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), `"key"`)
			},
			wantErr: false,
		},
		{
			name:     "template in content value",
			filename: "config.json",
			content:  map[string]any{"env": "{{ .environment }}"},
			context:  map[string]any{"environment": "prod"},
			validateFn: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), `"env": "prod"`)
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := serializeByExtension(tt.filename, tt.content, tt.context)
			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				tt.validateFn(t, result)
			}
		})
	}
}

// Test serializeToYAML function.
func TestSerializeToYAML(t *testing.T) {
	tests := []struct {
		name       string
		content    map[string]any
		validateFn func(t *testing.T, result []byte)
		wantErr    bool
	}{
		{
			name:    "simple map",
			content: map[string]any{"key": "value"},
			validateFn: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), "key: value")
			},
			wantErr: false,
		},
		{
			name:    "empty map",
			content: map[string]any{},
			validateFn: func(t *testing.T, result []byte) {
				assert.Equal(t, "{}\n", string(result))
			},
			wantErr: false,
		},
		{
			name: "nested structure with proper indentation",
			content: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"level3": "deep",
					},
				},
			},
			validateFn: func(t *testing.T, result []byte) {
				// YAML should have 2-space indentation.
				assert.Contains(t, string(result), "level1:")
				assert.Contains(t, string(result), "  level2:")
				assert.Contains(t, string(result), "    level3: deep")
			},
			wantErr: false,
		},
		{
			name:    "special characters in values",
			content: map[string]any{"key": "value: with: colons"},
			validateFn: func(t *testing.T, result []byte) {
				// YAML should properly quote or handle special chars.
				assert.Contains(t, string(result), "value: with: colons")
			},
			wantErr: false,
		},
		{
			name:    "array values",
			content: map[string]any{"items": []any{"a", "b", "c"}},
			validateFn: func(t *testing.T, result []byte) {
				str := string(result)
				assert.Contains(t, str, "items:")
				assert.Contains(t, str, "- a")
				assert.Contains(t, str, "- b")
				assert.Contains(t, str, "- c")
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := serializeToYAML(tt.content)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				tt.validateFn(t, result)
			}
		})
	}
}

// Test serializeToHCL function.
func TestSerializeToHCL(t *testing.T) {
	tests := []struct {
		name       string
		content    map[string]any
		validateFn func(t *testing.T, result []byte)
		wantErr    bool
	}{
		{
			name:    "simple attribute",
			content: map[string]any{"name": "test"},
			validateFn: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), "name")
				assert.Contains(t, string(result), "test")
			},
			wantErr: false,
		},
		{
			name:    "empty map",
			content: map[string]any{},
			validateFn: func(t *testing.T, result []byte) {
				// Empty HCL file.
				assert.Equal(t, "", string(result))
			},
			wantErr: false,
		},
		{
			name: "nested block",
			content: map[string]any{
				"locals": map[string]any{
					"environment": "prod",
				},
			},
			validateFn: func(t *testing.T, result []byte) {
				str := string(result)
				assert.Contains(t, str, "locals")
				assert.Contains(t, str, "environment")
				assert.Contains(t, str, "prod")
			},
			wantErr: false,
		},
		{
			name: "mixed types",
			content: map[string]any{
				"str_val":  "hello",
				"int_val":  42,
				"bool_val": true,
			},
			validateFn: func(t *testing.T, result []byte) {
				str := string(result)
				assert.Contains(t, str, "str_val")
				assert.Contains(t, str, "int_val")
				assert.Contains(t, str, "bool_val")
			},
			wantErr: false,
		},
		{
			name: "array attribute",
			content: map[string]any{
				"tags": []any{"web", "prod"},
			},
			validateFn: func(t *testing.T, result []byte) {
				str := string(result)
				assert.Contains(t, str, "tags")
				assert.Contains(t, str, "web")
				assert.Contains(t, str, "prod")
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := serializeToHCL(tt.content)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				tt.validateFn(t, result)
			}
		})
	}
}

// Test renderMapTemplates function.
func TestRenderMapTemplates(t *testing.T) {
	tests := []struct {
		name     string
		content  map[string]any
		context  map[string]any
		validate func(t *testing.T, result map[string]any)
		wantErr  bool
	}{
		{
			name:    "simple string template",
			content: map[string]any{"env": "{{ .environment }}"},
			context: map[string]any{"environment": "prod"},
			validate: func(t *testing.T, result map[string]any) {
				assert.Equal(t, "prod", result["env"])
			},
			wantErr: false,
		},
		{
			name:    "non-string values pass through",
			content: map[string]any{"count": 42, "enabled": true},
			context: map[string]any{},
			validate: func(t *testing.T, result map[string]any) {
				assert.Equal(t, 42, result["count"])
				assert.Equal(t, true, result["enabled"])
			},
			wantErr: false,
		},
		{
			name: "nested map templates",
			content: map[string]any{
				"outer": map[string]any{
					"inner": "{{ .value }}",
				},
			},
			context: map[string]any{"value": "nested"},
			validate: func(t *testing.T, result map[string]any) {
				outer := result["outer"].(map[string]any)
				assert.Equal(t, "nested", outer["inner"])
			},
			wantErr: false,
		},
		{
			name: "array with templates",
			content: map[string]any{
				"items": []any{"{{ .first }}", "{{ .second }}"},
			},
			context: map[string]any{"first": "a", "second": "b"},
			validate: func(t *testing.T, result map[string]any) {
				items := result["items"].([]any)
				assert.Equal(t, "a", items[0])
				assert.Equal(t, "b", items[1])
			},
			wantErr: false,
		},
		{
			name:    "empty map",
			content: map[string]any{},
			context: map[string]any{},
			validate: func(t *testing.T, result map[string]any) {
				assert.Empty(t, result)
			},
			wantErr: false,
		},
		{
			name:    "template parse error propagates",
			content: map[string]any{"bad": "{{ .name"},
			context: map[string]any{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := renderMapTemplates(tt.content, tt.context)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				tt.validate(t, result)
			}
		})
	}
}

// Test renderArrayTemplates function.
func TestRenderArrayTemplates(t *testing.T) {
	tests := []struct {
		name     string
		content  []any
		context  map[string]any
		validate func(t *testing.T, result []any)
		wantErr  bool
	}{
		{
			name:    "string templates in array",
			content: []any{"{{ .a }}", "{{ .b }}"},
			context: map[string]any{"a": "first", "b": "second"},
			validate: func(t *testing.T, result []any) {
				assert.Equal(t, "first", result[0])
				assert.Equal(t, "second", result[1])
			},
			wantErr: false,
		},
		{
			name:    "mixed types in array",
			content: []any{"{{ .str }}", 42, true},
			context: map[string]any{"str": "hello"},
			validate: func(t *testing.T, result []any) {
				assert.Equal(t, "hello", result[0])
				assert.Equal(t, 42, result[1])
				assert.Equal(t, true, result[2])
			},
			wantErr: false,
		},
		{
			name: "nested map in array",
			content: []any{
				map[string]any{"key": "{{ .val }}"},
			},
			context: map[string]any{"val": "nested"},
			validate: func(t *testing.T, result []any) {
				item := result[0].(map[string]any)
				assert.Equal(t, "nested", item["key"])
			},
			wantErr: false,
		},
		{
			name: "nested array in array",
			content: []any{
				[]any{"{{ .x }}", "{{ .y }}"},
			},
			context: map[string]any{"x": "1", "y": "2"},
			validate: func(t *testing.T, result []any) {
				inner := result[0].([]any)
				assert.Equal(t, "1", inner[0])
				assert.Equal(t, "2", inner[1])
			},
			wantErr: false,
		},
		{
			name:    "empty array",
			content: []any{},
			context: map[string]any{},
			validate: func(t *testing.T, result []any) {
				assert.Empty(t, result)
			},
			wantErr: false,
		},
		{
			name:    "template error in array propagates",
			content: []any{"{{ .bad"},
			context: map[string]any{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := renderArrayTemplates(tt.content, tt.context)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				tt.validate(t, result)
			}
		})
	}
}

// Test renderContent function.
func TestRenderContent(t *testing.T) {
	tests := []struct {
		name       string
		filename   string
		content    any
		context    map[string]any
		validateFn func(t *testing.T, result []byte)
		wantErr    bool
	}{
		{
			name:     "string content as template",
			filename: "test.txt",
			content:  "Hello {{ .name }}!",
			context:  map[string]any{"name": "World"},
			validateFn: func(t *testing.T, result []byte) {
				assert.Equal(t, "Hello World!", string(result))
			},
			wantErr: false,
		},
		{
			name:     "map content serialized by extension",
			filename: "config.json",
			content:  map[string]any{"key": "value"},
			context:  map[string]any{},
			validateFn: func(t *testing.T, result []byte) {
				assert.Contains(t, string(result), `"key"`)
			},
			wantErr: false,
		},
		{
			name:     "unsupported content type",
			filename: "test.txt",
			content:  42, // int is not supported.
			context:  map[string]any{},
			wantErr:  true,
		},
		{
			name:     "unsupported content type slice",
			filename: "test.txt",
			content:  []string{"a", "b"}, // typed slice not supported.
			context:  map[string]any{},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := renderContent(tt.filename, tt.content, tt.context)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrInvalidConfig)
			} else {
				require.NoError(t, err)
				tt.validateFn(t, result)
			}
		})
	}
}

// Test writeHCLBlock function.
func TestWriteHCLBlock(t *testing.T) {
	tests := []struct {
		name       string
		content    map[string]any
		validateFn func(t *testing.T, result []byte)
		wantErr    bool
	}{
		{
			name:    "simple attributes",
			content: map[string]any{"name": "test", "count": 5},
			validateFn: func(t *testing.T, result []byte) {
				str := string(result)
				assert.Contains(t, str, "name")
				assert.Contains(t, str, "count")
			},
			wantErr: false,
		},
		{
			name: "nested block creates HCL block",
			content: map[string]any{
				"resource": map[string]any{
					"type": "aws_instance",
				},
			},
			validateFn: func(t *testing.T, result []byte) {
				str := string(result)
				assert.Contains(t, str, "resource")
				assert.Contains(t, str, "type")
			},
			wantErr: false,
		},
		{
			name: "deeply nested blocks",
			content: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"level3": "value",
					},
				},
			},
			validateFn: func(t *testing.T, result []byte) {
				str := string(result)
				assert.Contains(t, str, "level1")
				assert.Contains(t, str, "level2")
				assert.Contains(t, str, "level3")
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := serializeToHCL(tt.content)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				tt.validateFn(t, result)
			}
		})
	}
}

// Test edge cases for processCleanFile and processGenerateFile.
func TestProcessCleanFile_NonExistent(t *testing.T) {
	result := GenerateResult{Filename: "nonexistent.txt"}
	processCleanFile(&result, "/nonexistent/path/file.txt", false)

	// File doesn't exist, should be skipped.
	assert.True(t, result.Skipped)
	assert.False(t, result.Deleted)
}

func TestProcessCleanFile_DryRun(t *testing.T) {
	result := GenerateResult{Filename: "test.txt"}
	processCleanFile(&result, "/some/path/test.txt", true)

	assert.True(t, result.Skipped)
	assert.False(t, result.Deleted)
}

func TestProcessGenerateFile_DryRun(t *testing.T) {
	result := GenerateResult{Filename: "test.txt"}
	ctx := fileContext{
		filename:        "test.txt",
		filePath:        "/some/path/test.txt",
		content:         "content",
		templateContext: nil,
		dryRun:          true,
	}

	processGenerateFile(&result, ctx)

	assert.True(t, result.Skipped)
	assert.False(t, result.Created)
}

func TestProcessGenerateFile_InvalidTemplate(t *testing.T) {
	result := GenerateResult{Filename: "test.txt"}
	ctx := fileContext{
		filename:        "test.txt",
		filePath:        "/some/path/test.txt",
		content:         "{{ .bad", // Invalid template.
		templateContext: nil,
		dryRun:          false,
	}

	processGenerateFile(&result, ctx)

	assert.Error(t, result.Error)
	assert.Contains(t, result.Error.Error(), "failed to render")
}
