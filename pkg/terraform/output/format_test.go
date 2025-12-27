package output

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestFormatOutputs_JSON(t *testing.T) {
	outputs := map[string]any{
		"url":     "https://example.com",
		"port":    float64(8080),
		"enabled": true,
	}

	result, err := FormatOutputs(outputs, FormatJSON)
	require.NoError(t, err)

	expected := `{
  "enabled": true,
  "port": 8080,
  "url": "https://example.com"
}
`
	assert.Equal(t, expected, result)
}

func TestFormatOutputs_YAML(t *testing.T) {
	outputs := map[string]any{
		"url":     "https://example.com",
		"port":    float64(8080),
		"enabled": true,
	}

	result, err := FormatOutputs(outputs, FormatYAML)
	require.NoError(t, err)

	// YAML output is sorted alphabetically.
	assert.Contains(t, result, "enabled: true")
	assert.Contains(t, result, "port: 8080")
	assert.Contains(t, result, "url: https://example.com")
}

func TestFormatOutputs_HCL(t *testing.T) {
	outputs := map[string]any{
		"url":     "https://example.com",
		"port":    float64(8080),
		"enabled": true,
	}

	result, err := FormatOutputs(outputs, FormatHCL)
	require.NoError(t, err)

	assert.Contains(t, result, "enabled = true\n")
	assert.Contains(t, result, "port = 8080\n")
	assert.Contains(t, result, `url = "https://example.com"`)
}

func TestFormatOutputs_Env(t *testing.T) {
	outputs := map[string]any{
		"url":     "https://example.com",
		"port":    float64(8080),
		"enabled": true,
	}

	result, err := FormatOutputs(outputs, FormatEnv)
	require.NoError(t, err)

	assert.Contains(t, result, "url=https://example.com\n")
	assert.Contains(t, result, "port=8080\n")
	assert.Contains(t, result, "enabled=true\n")
}

func TestFormatOutputs_Dotenv(t *testing.T) {
	outputs := map[string]any{
		"url":     "https://example.com",
		"port":    float64(8080),
		"enabled": true,
	}

	result, err := FormatOutputs(outputs, FormatDotenv)
	require.NoError(t, err)

	assert.Contains(t, result, "url='https://example.com'\n")
	assert.Contains(t, result, "port='8080'\n")
	assert.Contains(t, result, "enabled='true'\n")
}

func TestFormatOutputs_Bash(t *testing.T) {
	outputs := map[string]any{
		"url":     "https://example.com",
		"port":    float64(8080),
		"enabled": true,
	}

	result, err := FormatOutputs(outputs, FormatBash)
	require.NoError(t, err)

	assert.Contains(t, result, "export url='https://example.com'\n")
	assert.Contains(t, result, "export port='8080'\n")
	assert.Contains(t, result, "export enabled='true'\n")
}

func TestFormatOutputs_CSV(t *testing.T) {
	outputs := map[string]any{
		"url":     "https://example.com",
		"port":    float64(8080),
		"enabled": true,
	}

	result, err := FormatOutputs(outputs, FormatCSV)
	require.NoError(t, err)

	assert.Contains(t, result, "key,value\n")
	assert.Contains(t, result, "url,https://example.com\n")
	assert.Contains(t, result, "port,8080\n")
	assert.Contains(t, result, "enabled,true\n")
}

func TestFormatOutputs_TSV(t *testing.T) {
	outputs := map[string]any{
		"url":     "https://example.com",
		"port":    float64(8080),
		"enabled": true,
	}

	result, err := FormatOutputs(outputs, FormatTSV)
	require.NoError(t, err)

	assert.Contains(t, result, "key\tvalue\n")
	assert.Contains(t, result, "url\thttps://example.com\n")
	assert.Contains(t, result, "port\t8080\n")
	assert.Contains(t, result, "enabled\ttrue\n")
}

func TestFormatOutputs_CSV_SpecialCharacters(t *testing.T) {
	outputs := map[string]any{
		"with_comma":   "hello, world",
		"with_quote":   `say "hello"`,
		"with_newline": "line1\nline2",
	}

	result, err := FormatOutputs(outputs, FormatCSV)
	require.NoError(t, err)

	// Values with commas, quotes, or newlines should be quoted.
	assert.Contains(t, result, `with_comma,"hello, world"`)
	assert.Contains(t, result, `with_quote,"say ""hello"""`)
	assert.Contains(t, result, "with_newline,\"line1\nline2\"")
}

func TestFormatOutputs_UnsupportedFormat(t *testing.T) {
	outputs := map[string]any{"key": "value"}

	_, err := FormatOutputs(outputs, Format("invalid"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidArgumentError), "error should be ErrInvalidArgumentError")
}

func TestFormatOutputs_NullValues(t *testing.T) {
	outputs := map[string]any{
		"url":      "https://example.com",
		"nullable": nil,
	}

	// Env format should skip null values.
	result, err := FormatOutputs(outputs, FormatEnv)
	require.NoError(t, err)
	assert.Contains(t, result, "url=https://example.com\n")
	assert.NotContains(t, result, "nullable")

	// Dotenv format should skip null values.
	result, err = FormatOutputs(outputs, FormatDotenv)
	require.NoError(t, err)
	assert.Contains(t, result, "url='https://example.com'\n")
	assert.NotContains(t, result, "nullable")

	// Bash format should skip null values.
	result, err = FormatOutputs(outputs, FormatBash)
	require.NoError(t, err)
	assert.Contains(t, result, "export url='https://example.com'\n")
	assert.NotContains(t, result, "nullable")

	// HCL format should skip null values.
	result, err = FormatOutputs(outputs, FormatHCL)
	require.NoError(t, err)
	assert.Contains(t, result, `url = "https://example.com"`)
	assert.NotContains(t, result, "nullable")
}

func TestFormatOutputs_ComplexTypes(t *testing.T) {
	outputs := map[string]any{
		"simple": "value",
		"list":   []any{"a", "b", "c"},
		"map":    map[string]any{"key": "value"},
	}

	// Env format should JSON-encode complex types.
	result, err := FormatOutputs(outputs, FormatEnv)
	require.NoError(t, err)
	assert.Contains(t, result, "simple=value\n")
	assert.Contains(t, result, `list=["a","b","c"]`)
	assert.Contains(t, result, `map={"key":"value"}`)

	// HCL format should properly format complex types.
	result, err = FormatOutputs(outputs, FormatHCL)
	require.NoError(t, err)
	assert.Contains(t, result, `list = ["a", "b", "c"]`)
	assert.Contains(t, result, `map = {`)
}

func TestFormatOutputs_EmptyOutputs(t *testing.T) {
	outputs := map[string]any{}

	result, err := FormatOutputs(outputs, FormatEnv)
	require.NoError(t, err)
	assert.Equal(t, "", result)

	result, err = FormatOutputs(outputs, FormatJSON)
	require.NoError(t, err)
	assert.Equal(t, "{}\n", result)
}

func TestFormatOutputs_SpecialCharacters(t *testing.T) {
	outputs := map[string]any{
		"with_quote":     "it's a test",
		"with_backslash": `path\to\file`,
	}

	// Dotenv format should escape single quotes.
	result, err := FormatOutputs(outputs, FormatDotenv)
	require.NoError(t, err)
	assert.Contains(t, result, `with_quote='it'\''s a test'`)

	// Bash format should escape single quotes.
	result, err = FormatOutputs(outputs, FormatBash)
	require.NoError(t, err)
	assert.Contains(t, result, `export with_quote='it'\''s a test'`)

	// HCL format should escape backslashes and quotes.
	result, err = FormatOutputs(outputs, FormatHCL)
	require.NoError(t, err)
	assert.Contains(t, result, `with_backslash = "path\\to\\file"`)
}

func TestFormatOutputs_IntegerValues(t *testing.T) {
	outputs := map[string]any{
		"integer": float64(42),
		"float":   float64(3.14),
	}

	result, err := FormatOutputs(outputs, FormatEnv)
	require.NoError(t, err)
	assert.Contains(t, result, "integer=42\n")
	assert.Contains(t, result, "float=3.14\n")

	result, err = FormatOutputs(outputs, FormatHCL)
	require.NoError(t, err)
	assert.Contains(t, result, "integer = 42\n")
	assert.Contains(t, result, "float = 3.14\n")
}

func TestValueToString(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"string", "hello", "hello"},
		{"integer", float64(42), "42"},
		{"float", float64(3.14), "3.14"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"nil", nil, ""},
		{"slice", []any{"a", "b"}, `["a","b"]`},
		{"map", map[string]any{"key": "value"}, `{"key":"value"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := valueToString(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestValueToHCL(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"string", "hello", `"hello"`},
		{"integer", float64(42), "42"},
		{"float", float64(3.14), "3.14"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"nil", nil, "null"},
		{"slice", []any{"a", "b"}, `["a", "b"]`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := valueToHCL(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSortedKeys(t *testing.T) {
	m := map[string]any{
		"zebra":    1,
		"apple":    2,
		"mango":    3,
		"banana":   4,
		"coconut":  5,
		"date":     6,
		"eggplant": 7,
	}

	keys := sortedKeys(m)
	expected := []string{"apple", "banana", "coconut", "date", "eggplant", "mango", "zebra"}
	assert.Equal(t, expected, keys)
}

func TestFormatOutputs_HCL_NestedMap(t *testing.T) {
	outputs := map[string]any{
		"config": map[string]any{
			"host": "localhost",
			"port": float64(3000),
		},
	}

	result, err := FormatOutputs(outputs, FormatHCL)
	require.NoError(t, err)
	assert.Contains(t, result, "config = {\n")
	assert.Contains(t, result, `host = "localhost"`)
	assert.Contains(t, result, "port = 3000")
}

func TestFormatOutputs_HCL_NestedList(t *testing.T) {
	outputs := map[string]any{
		"items": []any{
			map[string]any{"name": "item1"},
			map[string]any{"name": "item2"},
		},
	}

	result, err := FormatOutputs(outputs, FormatHCL)
	require.NoError(t, err)
	assert.Contains(t, result, "items = [{")
}

// Tests for FormatSingleValue.

func TestFormatSingleValue_Scalar(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    any
		format   Format
		expected string
	}{
		{"json string", "url", "https://example.com", FormatJSON, "\"https://example.com\"\n"},
		{"json number", "port", float64(8080), FormatJSON, "8080\n"},
		{"json bool", "enabled", true, FormatJSON, "true\n"},
		{"yaml string", "url", "https://example.com", FormatYAML, "https://example.com\n"},
		{"yaml number", "port", float64(8080), FormatYAML, "8080\n"},
		{"hcl string", "url", "https://example.com", FormatHCL, "url = \"https://example.com\"\n"},
		{"hcl number", "port", float64(8080), FormatHCL, "port = 8080\n"},
		{"env string", "url", "https://example.com", FormatEnv, "url=https://example.com\n"},
		{"env number", "port", float64(8080), FormatEnv, "port=8080\n"},
		{"dotenv string", "url", "https://example.com", FormatDotenv, "url='https://example.com'\n"},
		{"bash string", "url", "https://example.com", FormatBash, "export url='https://example.com'\n"},
		{"csv string", "url", "https://example.com", FormatCSV, "url,https://example.com\n"},
		{"tsv string", "url", "https://example.com", FormatTSV, "url\thttps://example.com\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FormatSingleValue(tt.key, tt.value, tt.format)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatSingleValue_ComplexJSON(t *testing.T) {
	// Complex types work with JSON format.
	value := map[string]any{
		"host": "localhost",
		"port": float64(3000),
	}

	result, err := FormatSingleValue("config", value, FormatJSON)
	require.NoError(t, err)
	assert.Contains(t, result, `"host": "localhost"`)
	assert.Contains(t, result, `"port": 3000`)
}

func TestFormatSingleValue_ComplexYAML(t *testing.T) {
	// Complex types work with YAML format.
	value := map[string]any{
		"host": "localhost",
		"port": float64(3000),
	}

	result, err := FormatSingleValue("config", value, FormatYAML)
	require.NoError(t, err)
	assert.Contains(t, result, "host: localhost")
	assert.Contains(t, result, "port: 3000")
}

func TestFormatSingleValue_ComplexHCL(t *testing.T) {
	// Complex types work with HCL format.
	value := map[string]any{
		"host": "localhost",
		"port": float64(3000),
	}

	result, err := FormatSingleValue("config", value, FormatHCL)
	require.NoError(t, err)
	assert.Contains(t, result, "config = {")
	assert.Contains(t, result, `host = "localhost"`)
	assert.Contains(t, result, "port = 3000")
}

func TestFormatSingleValue_ComplexCSV_Error(t *testing.T) {
	// Complex types should error with CSV format.
	value := map[string]any{
		"host": "localhost",
		"port": float64(3000),
	}

	_, err := FormatSingleValue("config", value, FormatCSV)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidArgumentError), "error should be ErrInvalidArgumentError")
}

func TestFormatSingleValue_ComplexTSV_Error(t *testing.T) {
	// Complex types should error with TSV format.
	value := []any{"a", "b", "c"}

	_, err := FormatSingleValue("items", value, FormatTSV)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidArgumentError), "error should be ErrInvalidArgumentError")
}

func TestFormatSingleValue_ComplexEnv_Error(t *testing.T) {
	// Complex types should error with env format.
	value := map[string]any{"key": "value"}

	_, err := FormatSingleValue("config", value, FormatEnv)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidArgumentError))
}

func TestFormatSingleValue_ComplexDotenv_Error(t *testing.T) {
	// Complex types should error with dotenv format.
	value := []any{1, 2, 3}

	_, err := FormatSingleValue("numbers", value, FormatDotenv)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidArgumentError))
}

func TestFormatSingleValue_ComplexBash_Error(t *testing.T) {
	// Complex types should error with bash format.
	value := map[string]any{"nested": map[string]any{"deep": "value"}}

	_, err := FormatSingleValue("config", value, FormatBash)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidArgumentError))
}

func TestFormatOutputs_ComplexTypes_AllFormats(t *testing.T) {
	// Test that complex types in outputs work with all formats (FormatOutputs handles them).
	outputs := map[string]any{
		"simple":  "value",
		"config":  map[string]any{"host": "localhost", "port": float64(3000)},
		"items":   []any{"a", "b", "c"},
		"enabled": true,
		"count":   float64(42),
	}

	// JSON should work.
	result, err := FormatOutputs(outputs, FormatJSON)
	require.NoError(t, err)
	assert.Contains(t, result, `"simple": "value"`)
	assert.Contains(t, result, `"config"`)
	assert.Contains(t, result, `"items"`)

	// YAML should work.
	result, err = FormatOutputs(outputs, FormatYAML)
	require.NoError(t, err)
	assert.Contains(t, result, "simple: value")

	// HCL should work.
	result, err = FormatOutputs(outputs, FormatHCL)
	require.NoError(t, err)
	assert.Contains(t, result, `simple = "value"`)
	assert.Contains(t, result, "config = {")
	assert.Contains(t, result, `items = ["a", "b", "c"]`)

	// Env should work (complex types are JSON-encoded).
	result, err = FormatOutputs(outputs, FormatEnv)
	require.NoError(t, err)
	assert.Contains(t, result, "simple=value\n")
	assert.Contains(t, result, `config={"host":"localhost","port":3000}`)
	assert.Contains(t, result, `items=["a","b","c"]`)

	// CSV should work (complex types are JSON-encoded).
	result, err = FormatOutputs(outputs, FormatCSV)
	require.NoError(t, err)
	assert.Contains(t, result, "key,value\n")
	assert.Contains(t, result, "simple,value\n")
}

func TestFormatOutputsWithOptions_Uppercase(t *testing.T) {
	outputs := map[string]any{
		"vpc_id":       "vpc-123",
		"subnet_id":    "subnet-456",
		"cluster_name": "my-cluster",
	}

	opts := FormatOptions{Uppercase: true}

	// Test env format with uppercase.
	result, err := FormatOutputsWithOptions(outputs, FormatEnv, opts)
	require.NoError(t, err)
	assert.Contains(t, result, "VPC_ID=vpc-123\n")
	assert.Contains(t, result, "SUBNET_ID=subnet-456\n")
	assert.Contains(t, result, "CLUSTER_NAME=my-cluster\n")

	// Test bash format with uppercase.
	result, err = FormatOutputsWithOptions(outputs, FormatBash, opts)
	require.NoError(t, err)
	assert.Contains(t, result, "export VPC_ID='vpc-123'\n")
	assert.Contains(t, result, "export SUBNET_ID='subnet-456'\n")

	// Test JSON format with uppercase (keys should be uppercase).
	result, err = FormatOutputsWithOptions(outputs, FormatJSON, opts)
	require.NoError(t, err)
	assert.Contains(t, result, `"VPC_ID": "vpc-123"`)
	assert.Contains(t, result, `"SUBNET_ID": "subnet-456"`)

	// Test without uppercase option (keys stay lowercase).
	result, err = FormatOutputsWithOptions(outputs, FormatEnv, FormatOptions{})
	require.NoError(t, err)
	assert.Contains(t, result, "vpc_id=vpc-123\n")
	assert.Contains(t, result, "subnet_id=subnet-456\n")
}

func TestFormatSingleValueWithOptions_Uppercase(t *testing.T) {
	opts := FormatOptions{Uppercase: true}

	// Test env format with uppercase.
	result, err := FormatSingleValueWithOptions("vpc_id", "vpc-123", FormatEnv, opts)
	require.NoError(t, err)
	assert.Equal(t, "VPC_ID=vpc-123\n", result)

	// Test bash format with uppercase.
	result, err = FormatSingleValueWithOptions("cluster_name", "my-cluster", FormatBash, opts)
	require.NoError(t, err)
	assert.Equal(t, "export CLUSTER_NAME='my-cluster'\n", result)

	// Test HCL format with uppercase.
	result, err = FormatSingleValueWithOptions("port", float64(8080), FormatHCL, opts)
	require.NoError(t, err)
	assert.Equal(t, "PORT = 8080\n", result)

	// Test without uppercase option (key stays as-is).
	result, err = FormatSingleValueWithOptions("vpc_id", "vpc-123", FormatEnv, FormatOptions{})
	require.NoError(t, err)
	assert.Equal(t, "vpc_id=vpc-123\n", result)
}

func TestFormatOutputsWithOptions_Flatten(t *testing.T) {
	outputs := map[string]any{
		"simple": "value",
		"config": map[string]any{
			"host": "localhost",
			"port": float64(3000),
		},
		"nested": map[string]any{
			"level1": map[string]any{
				"level2": "deep_value",
			},
		},
	}

	opts := FormatOptions{Flatten: true}

	// Test env format with flatten.
	result, err := FormatOutputsWithOptions(outputs, FormatEnv, opts)
	require.NoError(t, err)
	assert.Contains(t, result, "simple=value\n")
	assert.Contains(t, result, "config_host=localhost\n")
	assert.Contains(t, result, "config_port=3000\n")
	assert.Contains(t, result, "nested_level1_level2=deep_value\n")
	// Should NOT contain the original nested key.
	assert.NotContains(t, result, "config={")

	// Test bash format with flatten.
	result, err = FormatOutputsWithOptions(outputs, FormatBash, opts)
	require.NoError(t, err)
	assert.Contains(t, result, "export config_host='localhost'\n")
	assert.Contains(t, result, "export config_port='3000'\n")

	// Test JSON format with flatten.
	result, err = FormatOutputsWithOptions(outputs, FormatJSON, opts)
	require.NoError(t, err)
	assert.Contains(t, result, `"config_host": "localhost"`)
	assert.Contains(t, result, `"config_port": 3000`)
	assert.Contains(t, result, `"nested_level1_level2": "deep_value"`)
}

func TestFormatOutputsWithOptions_FlattenAndUppercase(t *testing.T) {
	outputs := map[string]any{
		"config": map[string]any{
			"db_host": "localhost",
			"db_port": float64(5432),
		},
	}

	opts := FormatOptions{Flatten: true, Uppercase: true}

	// Test env format with both flatten and uppercase.
	result, err := FormatOutputsWithOptions(outputs, FormatEnv, opts)
	require.NoError(t, err)
	assert.Contains(t, result, "CONFIG_DB_HOST=localhost\n")
	assert.Contains(t, result, "CONFIG_DB_PORT=5432\n")

	// Test bash format with both options.
	result, err = FormatOutputsWithOptions(outputs, FormatBash, opts)
	require.NoError(t, err)
	assert.Contains(t, result, "export CONFIG_DB_HOST='localhost'\n")
	assert.Contains(t, result, "export CONFIG_DB_PORT='5432'\n")
}

func TestFormatOutputsWithOptions_FlattenArrays(t *testing.T) {
	outputs := map[string]any{
		"config": map[string]any{
			"hosts": []any{"host1", "host2", "host3"},
			"port":  float64(8080),
		},
	}

	opts := FormatOptions{Flatten: true}

	// Test env format - arrays should be flattened with numeric indices.
	result, err := FormatOutputsWithOptions(outputs, FormatEnv, opts)
	require.NoError(t, err)
	assert.Contains(t, result, "config_port=8080\n")
	assert.Contains(t, result, "config_hosts_0=host1\n")
	assert.Contains(t, result, "config_hosts_1=host2\n")
	assert.Contains(t, result, "config_hosts_2=host3\n")
	// Should NOT contain the original array.
	assert.NotContains(t, result, "config_hosts=[")

	// Test JSON format.
	result, err = FormatOutputsWithOptions(outputs, FormatJSON, opts)
	require.NoError(t, err)
	assert.Contains(t, result, `"config_port": 8080`)
	assert.Contains(t, result, `"config_hosts_0": "host1"`)
	assert.Contains(t, result, `"config_hosts_1": "host2"`)
	assert.Contains(t, result, `"config_hosts_2": "host3"`)
}

func TestFormatOutputsWithOptions_FlattenEmptyMaps(t *testing.T) {
	outputs := map[string]any{
		"simple":    "value",
		"emptymap":  map[string]any{},
		"populated": map[string]any{"key": "val"},
	}

	opts := FormatOptions{Flatten: true}

	result, err := FormatOutputsWithOptions(outputs, FormatEnv, opts)
	require.NoError(t, err)
	assert.Contains(t, result, "simple=value\n")
	assert.Contains(t, result, "populated_key=val\n")
	// Empty maps result in no keys.
	assert.NotContains(t, result, "emptymap")
}

func TestFormatOutputsWithOptions_WithoutFlatten(t *testing.T) {
	outputs := map[string]any{
		"config": map[string]any{
			"host": "localhost",
		},
	}

	// Without flatten option, nested maps should be JSON-encoded.
	opts := FormatOptions{Flatten: false}
	result, err := FormatOutputsWithOptions(outputs, FormatEnv, opts)
	require.NoError(t, err)
	assert.Contains(t, result, `config={"host":"localhost"}`)
	assert.NotContains(t, result, "config_host")
}

func TestFormatOutputsWithOptions_FlattenNestedArrays(t *testing.T) {
	// Test arrays containing maps (common in terraform outputs like aws_subnet).
	outputs := map[string]any{
		"subnets": []any{
			map[string]any{"id": "subnet-1", "cidr": "10.0.1.0/24"},
			map[string]any{"id": "subnet-2", "cidr": "10.0.2.0/24"},
		},
	}

	opts := FormatOptions{Flatten: true}

	result, err := FormatOutputsWithOptions(outputs, FormatEnv, opts)
	require.NoError(t, err)
	assert.Contains(t, result, "subnets_0_id=subnet-1\n")
	assert.Contains(t, result, "subnets_0_cidr=10.0.1.0/24\n")
	assert.Contains(t, result, "subnets_1_id=subnet-2\n")
	assert.Contains(t, result, "subnets_1_cidr=10.0.2.0/24\n")
}

func TestFormatOutputsWithOptions_FlattenNestedArraysOfArrays(t *testing.T) {
	// Test nested arrays.
	outputs := map[string]any{
		"matrix": []any{
			[]any{"a", "b"},
			[]any{"c", "d"},
		},
	}

	opts := FormatOptions{Flatten: true}

	result, err := FormatOutputsWithOptions(outputs, FormatEnv, opts)
	require.NoError(t, err)
	assert.Contains(t, result, "matrix_0_0=a\n")
	assert.Contains(t, result, "matrix_0_1=b\n")
	assert.Contains(t, result, "matrix_1_0=c\n")
	assert.Contains(t, result, "matrix_1_1=d\n")
}
