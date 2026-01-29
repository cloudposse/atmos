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

	// shellescape.Quote only adds quotes when needed.
	assert.Contains(t, result, "url=https://example.com\n")
	assert.Contains(t, result, "port=8080\n")
	assert.Contains(t, result, "enabled=true\n")
}

func TestFormatOutputs_Bash(t *testing.T) {
	outputs := map[string]any{
		"url":     "https://example.com",
		"port":    float64(8080),
		"enabled": true,
	}

	result, err := FormatOutputs(outputs, FormatBash)
	require.NoError(t, err)

	// shellescape.Quote only adds quotes when needed.
	assert.Contains(t, result, "export url=https://example.com\n")
	assert.Contains(t, result, "export port=8080\n")
	assert.Contains(t, result, "export enabled=true\n")
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
	assert.Contains(t, result, "url=https://example.com\n")
	assert.NotContains(t, result, "nullable")

	// Bash format should skip null values.
	result, err = FormatOutputs(outputs, FormatBash)
	require.NoError(t, err)
	assert.Contains(t, result, "export url=https://example.com\n")
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

	// Dotenv format should escape single quotes using shellescape pattern.
	result, err := FormatOutputs(outputs, FormatDotenv)
	require.NoError(t, err)
	assert.Contains(t, result, `with_quote='it'"'"'s a test'`)

	// Bash format should escape single quotes using shellescape pattern.
	result, err = FormatOutputs(outputs, FormatBash)
	require.NoError(t, err)
	assert.Contains(t, result, `export with_quote='it'"'"'s a test'`)

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
		{"dotenv string", "url", "https://example.com", FormatDotenv, "url=https://example.com\n"},
		{"bash string", "url", "https://example.com", FormatBash, "export url=https://example.com\n"},
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
	assert.Contains(t, result, "export VPC_ID=vpc-123\n")
	assert.Contains(t, result, "export SUBNET_ID=subnet-456\n")

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
	assert.Equal(t, "export CLUSTER_NAME=my-cluster\n", result)

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
	assert.Contains(t, result, "export config_host=localhost\n")
	assert.Contains(t, result, "export config_port=3000\n")

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
	assert.Contains(t, result, "export CONFIG_DB_HOST=localhost\n")
	assert.Contains(t, result, "export CONFIG_DB_PORT=5432\n")
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

func TestFormatOutputs_GitHub(t *testing.T) {
	outputs := map[string]any{
		"url":     "https://example.com",
		"port":    float64(8080),
		"enabled": true,
	}

	result, err := FormatOutputs(outputs, FormatGitHub)
	require.NoError(t, err)

	assert.Contains(t, result, "url=https://example.com\n")
	assert.Contains(t, result, "port=8080\n")
	assert.Contains(t, result, "enabled=true\n")
}

func TestFormatOutputs_GitHub_MultilineValue(t *testing.T) {
	outputs := map[string]any{
		"multiline": "line1\nline2\nline3",
		"simple":    "single line",
	}

	result, err := FormatOutputs(outputs, FormatGitHub)
	require.NoError(t, err)

	// Multiline values should use heredoc syntax.
	assert.Contains(t, result, "multiline<<ATMOS_EOF_multiline\n")
	assert.Contains(t, result, "line1\nline2\nline3\n")
	assert.Contains(t, result, "ATMOS_EOF_multiline\n")

	// Simple values should be key=value format.
	assert.Contains(t, result, "simple=single line\n")
}

func TestFormatOutputs_GitHub_ComplexTypes(t *testing.T) {
	outputs := map[string]any{
		"list": []any{"a", "b", "c"},
		"map":  map[string]any{"key": "value"},
	}

	result, err := FormatOutputs(outputs, FormatGitHub)
	require.NoError(t, err)

	// Complex types should be JSON-encoded.
	assert.Contains(t, result, `list=["a","b","c"]`)
	assert.Contains(t, result, `map={"key":"value"}`)
}

func TestFormatOutputs_GitHub_Uppercase(t *testing.T) {
	outputs := map[string]any{
		"vpc_id":    "vpc-12345",
		"subnet_id": "subnet-67890",
	}

	opts := FormatOptions{Uppercase: true}
	result, err := FormatOutputsWithOptions(outputs, FormatGitHub, opts)
	require.NoError(t, err)

	assert.Contains(t, result, "VPC_ID=vpc-12345\n")
	assert.Contains(t, result, "SUBNET_ID=subnet-67890\n")
}

func TestFormatOutputs_GitHub_NullValues(t *testing.T) {
	outputs := map[string]any{
		"valid":    "value",
		"null_val": nil,
	}

	result, err := FormatOutputs(outputs, FormatGitHub)
	require.NoError(t, err)

	// Null values should be skipped.
	assert.Contains(t, result, "valid=value\n")
	assert.NotContains(t, result, "null_val")
}

func TestFormatSingleValue_GitHub(t *testing.T) {
	// Test single scalar value.
	result, err := FormatSingleValue("vpc_id", "vpc-12345", FormatGitHub)
	require.NoError(t, err)
	assert.Equal(t, "vpc_id=vpc-12345\n", result)

	// Test single multiline value.
	result, err = FormatSingleValue("cert", "line1\nline2", FormatGitHub)
	require.NoError(t, err)
	assert.Contains(t, result, "cert<<ATMOS_EOF_cert\n")
	assert.Contains(t, result, "line1\nline2\n")
	assert.Contains(t, result, "ATMOS_EOF_cert\n")

	// Test single complex value (JSON-encoded).
	result, err = FormatSingleValue("config", map[string]any{"host": "localhost"}, FormatGitHub)
	require.NoError(t, err)
	assert.Equal(t, `config={"host":"localhost"}`+"\n", result)
}

// TestFormatOutputs_KeysSorted verifies that all output formats produce
// alphabetically sorted keys for consistent, deterministic output.
func TestFormatOutputs_KeysSorted(t *testing.T) {
	// Use keys that are intentionally out of order to verify sorting.
	outputs := map[string]any{
		"zebra":  "last",
		"alpha":  "first",
		"middle": "center",
		"beta":   "second",
	}

	tests := []struct {
		name     string
		format   Format
		contains []string // Ordered list of substrings that should appear in order.
	}{
		{
			name:     "json sorted",
			format:   FormatJSON,
			contains: []string{`"alpha"`, `"beta"`, `"middle"`, `"zebra"`},
		},
		{
			name:     "yaml sorted",
			format:   FormatYAML,
			contains: []string{"alpha:", "beta:", "middle:", "zebra:"},
		},
		{
			name:     "hcl sorted",
			format:   FormatHCL,
			contains: []string{"alpha =", "beta =", "middle =", "zebra ="},
		},
		{
			name:     "env sorted",
			format:   FormatEnv,
			contains: []string{"alpha=", "beta=", "middle=", "zebra="},
		},
		{
			name:     "dotenv sorted",
			format:   FormatDotenv,
			contains: []string{"alpha=", "beta=", "middle=", "zebra="},
		},
		{
			name:     "bash sorted",
			format:   FormatBash,
			contains: []string{"export alpha=", "export beta=", "export middle=", "export zebra="},
		},
		{
			name:     "csv sorted",
			format:   FormatCSV,
			contains: []string{"alpha,", "beta,", "middle,", "zebra,"},
		},
		{
			name:     "tsv sorted",
			format:   FormatTSV,
			contains: []string{"alpha\t", "beta\t", "middle\t", "zebra\t"},
		},
		{
			name:     "github sorted",
			format:   FormatGitHub,
			contains: []string{"alpha=", "beta=", "middle=", "zebra="},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := FormatOutputs(outputs, tt.format)
			require.NoError(t, err)

			// Verify that keys appear in alphabetical order.
			lastIndex := -1
			for _, substr := range tt.contains {
				idx := findSubstringIndex(result, substr, lastIndex+1)
				require.True(t, idx > lastIndex, "Expected %q to appear after previous key in format %s, result:\n%s", substr, tt.format, result)
				lastIndex = idx
			}
		})
	}
}

// TestFormatOutputs_NestedKeysSorted verifies that nested maps also have sorted keys.
func TestFormatOutputs_NestedKeysSorted(t *testing.T) {
	outputs := map[string]any{
		"outer": map[string]any{
			"zebra": "last",
			"alpha": "first",
			"beta":  "second",
		},
	}

	// Test JSON with nested sorted keys.
	result, err := FormatOutputs(outputs, FormatJSON)
	require.NoError(t, err)

	// Verify nested keys are sorted: alpha, beta, zebra.
	alphaIdx := findSubstringIndex(result, `"alpha"`, 0)
	betaIdx := findSubstringIndex(result, `"beta"`, 0)
	zebraIdx := findSubstringIndex(result, `"zebra"`, 0)

	assert.True(t, alphaIdx < betaIdx, "alpha should appear before beta in JSON")
	assert.True(t, betaIdx < zebraIdx, "beta should appear before zebra in JSON")

	// Test YAML with nested sorted keys.
	result, err = FormatOutputs(outputs, FormatYAML)
	require.NoError(t, err)

	alphaIdx = findSubstringIndex(result, "alpha:", 0)
	betaIdx = findSubstringIndex(result, "beta:", 0)
	zebraIdx = findSubstringIndex(result, "zebra:", 0)

	assert.True(t, alphaIdx < betaIdx, "alpha should appear before beta in YAML")
	assert.True(t, betaIdx < zebraIdx, "beta should appear before zebra in YAML")
}

// findSubstringIndex returns the index of substr in s starting from startIdx, or -1 if not found.
func findSubstringIndex(s, substr string, startIdx int) int {
	if startIdx >= len(s) {
		return -1
	}
	idx := len(s[:startIdx]) + len(substr)
	for i := startIdx; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	_ = idx // Avoid unused variable warning.
	return -1
}

// TestFormatOutputs_Table tests the table format output.
func TestFormatOutputs_Table(t *testing.T) {
	outputs := map[string]any{
		"url":     "https://example.com",
		"port":    float64(8080),
		"enabled": true,
	}

	result, err := FormatOutputs(outputs, FormatTable)
	require.NoError(t, err)

	// Table should contain headers and values.
	assert.Contains(t, result, "Key")
	assert.Contains(t, result, "Value")
	assert.Contains(t, result, "url")
	assert.Contains(t, result, "https://example.com")
	assert.Contains(t, result, "8080")
	assert.Contains(t, result, "true")
}

// TestFormatOutputs_Table_EmptyOutputs tests table format with empty outputs.
func TestFormatOutputs_Table_EmptyOutputs(t *testing.T) {
	outputs := map[string]any{}

	result, err := FormatOutputs(outputs, FormatTable)
	require.NoError(t, err)

	// Should still contain headers.
	assert.Contains(t, result, "Key")
	assert.Contains(t, result, "Value")
}

// TestFormatOutputs_Table_NullValues tests table format skips null values.
func TestFormatOutputs_Table_NullValues(t *testing.T) {
	outputs := map[string]any{
		"valid":    "value",
		"null_val": nil,
	}

	result, err := FormatOutputs(outputs, FormatTable)
	require.NoError(t, err)

	assert.Contains(t, result, "valid")
	assert.Contains(t, result, "value")
	assert.NotContains(t, result, "null_val")
}

// TestFormatOutputs_Table_ComplexTypes tests table format with complex types.
func TestFormatOutputs_Table_ComplexTypes(t *testing.T) {
	outputs := map[string]any{
		"simple": "value",
		"list":   []any{"a", "b", "c"},
		"map":    map[string]any{"key": "val"},
	}

	result, err := FormatOutputs(outputs, FormatTable)
	require.NoError(t, err)

	// Should contain all keys.
	assert.Contains(t, result, "simple")
	assert.Contains(t, result, "list")
	assert.Contains(t, result, "map")
	// Complex types should be JSON-encoded.
	assert.Contains(t, result, `["a","b","c"]`)
	assert.Contains(t, result, `{"key":"val"}`)
}

// TestFormatOutputs_Table_IntegerAndFloat tests table format with numeric types.
func TestFormatOutputs_Table_IntegerAndFloat(t *testing.T) {
	outputs := map[string]any{
		"integer": float64(42),
		"float":   float64(3.14159),
	}

	result, err := FormatOutputs(outputs, FormatTable)
	require.NoError(t, err)

	// Integer should be formatted without decimal.
	assert.Contains(t, result, "42")
	// Float should retain decimals.
	assert.Contains(t, result, "3.14")
}

// TestFormatValueForTable tests the formatValueForTable helper function.
func TestFormatValueForTable(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		contains string
	}{
		{"string", "hello", "hello"},
		{"integer", float64(42), "42"},
		{"float", float64(3.14), "3.14"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"nil", nil, ""},
		{"slice", []any{"a", "b"}, `["a","b"]`},
		{"map", map[string]any{"key": "value"}, `{"key":"value"}`},
		{"nested map", map[string]any{"outer": map[string]any{"inner": "val"}}, `inner`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Pass nil config to skip highlighting.
			result := formatValueForTable(tt.value, nil)
			if tt.contains == "" {
				assert.Equal(t, "", result)
			} else {
				assert.Contains(t, result, tt.contains)
			}
		})
	}
}

// TestHighlightValue tests the highlightValue helper function.
func TestHighlightValue(t *testing.T) {
	// With nil config, should return the value unchanged.
	result := highlightValue("test", nil)
	assert.Equal(t, "test", result)

	// With empty string, should return empty string.
	result = highlightValue("", nil)
	assert.Equal(t, "", result)
}

// TestSortMapRecursive tests the sortMapRecursive function.
func TestSortMapRecursive(t *testing.T) {
	m := map[string]any{
		"zebra": "last",
		"alpha": "first",
		"middle": map[string]any{
			"zulu": "end",
			"able": "start",
		},
	}

	sorted := sortMapRecursive(m)

	// Keys should be sorted.
	keys := sortedKeys(sorted)
	assert.Equal(t, []string{"alpha", "middle", "zebra"}, keys)

	// Nested map keys should also be sorted.
	nestedMap := sorted["middle"].(map[string]any)
	nestedKeys := sortedKeys(nestedMap)
	assert.Equal(t, []string{"able", "zulu"}, nestedKeys)
}

// TestSortValueRecursive tests the sortValueRecursive function.
func TestSortValueRecursive(t *testing.T) {
	// Test with a map.
	m := map[string]any{
		"z": 1,
		"a": 2,
	}
	result := sortValueRecursive(m)
	resultMap, ok := result.(map[string]any)
	require.True(t, ok)
	keys := sortedKeys(resultMap)
	assert.Equal(t, []string{"a", "z"}, keys)

	// Test with a slice containing maps.
	s := []any{
		map[string]any{"z": 1, "a": 2},
	}
	result = sortValueRecursive(s)
	resultSlice, ok := result.([]any)
	require.True(t, ok)
	require.Len(t, resultSlice, 1)
	innerMap, ok := resultSlice[0].(map[string]any)
	require.True(t, ok)
	keys = sortedKeys(innerMap)
	assert.Equal(t, []string{"a", "z"}, keys)

	// Test with a scalar (should return unchanged).
	scalar := "test"
	result = sortValueRecursive(scalar)
	assert.Equal(t, scalar, result)
}

// TestSortSliceRecursive tests the sortSliceRecursive function.
func TestSortSliceRecursive(t *testing.T) {
	// Slice with maps should have maps recursively sorted.
	s := []any{
		map[string]any{"z": 1, "a": 2},
		"scalar",
		[]any{"nested", "slice"},
	}

	result := sortSliceRecursive(s)

	// First element should be a sorted map.
	firstElem, ok := result[0].(map[string]any)
	require.True(t, ok)
	keys := sortedKeys(firstElem)
	assert.Equal(t, []string{"a", "z"}, keys)

	// Second element should be the scalar unchanged.
	assert.Equal(t, "scalar", result[1])

	// Third element should be the nested slice.
	thirdElem, ok := result[2].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"nested", "slice"}, thirdElem)
}

// TestTransformKeys tests the transformKeys function.
func TestTransformKeys(t *testing.T) {
	outputs := map[string]any{
		"vpc_id":    "vpc-123",
		"subnet_id": "subnet-456",
	}

	// Without uppercase option.
	result := transformKeys(outputs, FormatOptions{})
	assert.Equal(t, outputs, result)

	// With uppercase option.
	result = transformKeys(outputs, FormatOptions{Uppercase: true})
	assert.Equal(t, "vpc-123", result["VPC_ID"])
	assert.Equal(t, "subnet-456", result["SUBNET_ID"])
}

// TestFlattenMap tests the flattenMap function.
func TestFlattenMap(t *testing.T) {
	m := map[string]any{
		"simple": "value",
		"nested": map[string]any{
			"deep": "deepvalue",
		},
	}

	result := flattenMap(m, "", "_")

	assert.Equal(t, "value", result["simple"])
	assert.Equal(t, "deepvalue", result["nested_deep"])
}

// TestFlattenMap_WithPrefix tests flattenMap with a prefix.
func TestFlattenMap_WithPrefix(t *testing.T) {
	m := map[string]any{
		"key": "value",
	}

	result := flattenMap(m, "prefix", "_")

	assert.Equal(t, "value", result["prefix_key"])
}

// TestFlattenMap_WithCustomSeparator tests flattenMap with different separators.
func TestFlattenMap_WithCustomSeparator(t *testing.T) {
	m := map[string]any{
		"outer": map[string]any{
			"inner": "value",
		},
	}

	// Test with double underscore separator.
	result := flattenMap(m, "", "__")
	assert.Equal(t, "value", result["outer__inner"])

	// Test with dot separator.
	result = flattenMap(m, "", ".")
	assert.Equal(t, "value", result["outer.inner"])
}

// TestFlattenSlice tests the flattenSlice function.
func TestFlattenSlice(t *testing.T) {
	result := make(map[string]any)
	s := []any{"a", "b", "c"}

	flattenSlice("items", s, "_", result)

	assert.Equal(t, "a", result["items_0"])
	assert.Equal(t, "b", result["items_1"])
	assert.Equal(t, "c", result["items_2"])
}

// TestFlattenValue tests the flattenValue function.
func TestFlattenValue(t *testing.T) {
	result := make(map[string]any)

	// Test scalar value.
	flattenValue("key", "value", "_", result)
	assert.Equal(t, "value", result["key"])

	// Test map value.
	result = make(map[string]any)
	flattenValue("config", map[string]any{"port": float64(8080)}, "_", result)
	assert.Equal(t, float64(8080), result["config_port"])

	// Test slice value.
	result = make(map[string]any)
	flattenValue("items", []any{"x", "y"}, "_", result)
	assert.Equal(t, "x", result["items_0"])
	assert.Equal(t, "y", result["items_1"])
}
