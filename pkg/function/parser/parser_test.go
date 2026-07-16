package parser

import (
	"errors"
	"testing"

	"github.com/alecthomas/participle/v2/lexer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	yaml "gopkg.in/yaml.v3"
)

func TestParseTerraform(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  TerraformArgs
	}{
		{"simple output", "vpc vpc_id", TerraformArgs{Component: "vpc", Expression: "vpc_id"}},
		{"explicit stack", "vpc tenant-ue2-dev vpc_id", TerraformArgs{Component: "vpc", Stack: "tenant-ue2-dev", Expression: "vpc_id"}},
		{"compact JSON default", `component-2 .output // {"key1":"fallback1"}`, TerraformArgs{Component: "component-2", Expression: `.output // {"key1":"fallback1"}`}},
		{"clean default", `vpc .vpc_id // {"key": "value"}`, TerraformArgs{Component: "vpc", Expression: `.vpc_id // {"key": "value"}`}},
		{"explicit stack and pipe", `vpc dev .hostname | "jdbc://" + .`, TerraformArgs{Component: "vpc", Stack: "dev", Expression: `.hostname | "jdbc://" + .`}},
		{"single-quoted bracket expression", `vpc '.secret_arns_map["service/key"]'`, TerraformArgs{Component: "vpc", Expression: `.secret_arns_map["service/key"]`}},
		{"folded scalar value", "vpc\n  .vpc_id // {\"key\": \"value\"}", TerraformArgs{Component: "vpc", Expression: `.vpc_id // {"key": "value"}`}},
		{"legacy csv", `vpc ".vpc_id // ""fallback"""`, TerraformArgs{Component: "vpc", Expression: `.vpc_id // "fallback"`}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := ParseTerraform(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, actual)
		})
	}
}

func TestParseTerraformErrors(t *testing.T) {
	for _, input := range []string{"", "vpc", "vpc dev output extra"} {
		_, err := ParseTerraform(input)
		require.Error(t, err)
		var parseErr *Error
		require.True(t, errors.As(err, &parseErr))
		assert.Positive(t, parseErr.Position.Line)
		assert.Positive(t, parseErr.Position.Column)
	}

	_, err := ParseTerraform("vpc dev output extra")
	var parseErr *Error
	require.True(t, errors.As(err, &parseErr))
	assert.Equal(t, Position{Offset: 15, Line: 1, Column: 16}, parseErr.Position)
}

func TestParseTerraformRejectsUnterminatedQuotedExpression(t *testing.T) {
	_, err := ParseTerraform(`vpc ".output // unterminated`)
	require.Error(t, err)

	var parseErr *Error
	require.ErrorAs(t, err, &parseErr)
	assert.Equal(t, "unterminated quoted value", parseErr.Message)
	assert.Equal(t, Position{Offset: 4, Line: 1, Column: 5}, parseErr.Position)
}

func TestParseTerraformTaggedBlockScalars(t *testing.T) {
	for _, test := range []struct {
		name string
		yaml string
		tag  string
		want TerraformArgs
	}{
		{
			name: "folded state expression",
			yaml: "value: !terraform.state >-\n  component-2 .output // {\"key1\": \"fallback1\"}\n",
			tag:  "!terraform.state",
			want: TerraformArgs{Component: "component-2", Expression: `.output // {"key1": "fallback1"}`},
		},
		{
			name: "literal output expression with pipe",
			yaml: "value: !terraform.output |\n  component-2 .output | .nested\n",
			tag:  "!terraform.output",
			want: TerraformArgs{Component: "component-2", Expression: ".output | .nested"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var document yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte(test.yaml), &document))
			value := document.Content[0].Content[1]
			assert.Equal(t, test.tag, value.Tag)

			actual, err := ParseTerraform(value.Value)
			require.NoError(t, err)
			assert.Equal(t, test.want, actual)
		})
	}
}

func TestParseEnv(t *testing.T) {
	actual, err := ParseEnv(`NAME "default value"`)
	require.NoError(t, err)
	assert.Equal(t, EnvArgs{Name: "NAME", Default: "default value"}, actual)

	actual, err = ParseEnv("NAME")
	require.NoError(t, err)
	assert.Equal(t, EnvArgs{Name: "NAME"}, actual)

	_, err = ParseEnv("NAME default extra")
	require.Error(t, err)
	_, err = ParseEnv("")
	require.Error(t, err)
}

func TestParseRandom(t *testing.T) {
	for _, test := range []struct {
		input string
		want  []string
	}{
		{"", []string{}},
		{"100", []string{"100"}},
		{"1024 65535", []string{"1024", "65535"}},
	} {
		actual, err := ParseRandom(test.input)
		require.NoError(t, err)
		assert.Equal(t, test.want, actual.Values)
	}
	_, err := ParseRandom("1 2 3")
	require.Error(t, err)
	_, err = ParseRandom(`"`)
	require.Error(t, err)
}

func TestParseInclude(t *testing.T) {
	actual, err := ParseInclude(`config.yaml .vars.items[] | select(.enabled)`)
	require.NoError(t, err)
	assert.Equal(t, IncludeArgs{Path: "config.yaml", Query: ".vars.items[] | select(.enabled)"}, actual)

	actual, err = ParseInclude(`"config with spaces.yaml"`)
	require.NoError(t, err)
	assert.Equal(t, IncludeArgs{Path: "config with spaces.yaml"}, actual)

	_, err = ParseInclude("")
	require.Error(t, err)
	_, err = ParseInclude(`"`)
	require.Error(t, err)
}

func TestParseStore(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  StoreArgs
	}{
		{"current stack", "ssm vpc id", StoreArgs{Store: "ssm", Component: "vpc", Key: "id"}},
		{"explicit stack", "ssm dev vpc id", StoreArgs{Store: "ssm", Stack: "dev", Component: "vpc", Key: "id"}},
		{"default and query", `ssm dev vpc id | default "not set" | query .config | .value`, StoreArgs{Store: "ssm", Stack: "dev", Component: "vpc", Key: "id", Default: stringPtr("not set"), Query: ".config | .value"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := ParseStore(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.want, actual)
		})
	}

	for _, input := range []string{"", "ssm vpc", "ssm vpc id | default", "ssm vpc id | default value extra", "ssm vpc id | unknown value"} {
		_, err := ParseStore(input)
		require.Error(t, err)
	}
}

func TestParseStoreGet(t *testing.T) {
	actual, err := ParseStoreGet(`ssm app-key | query .value | .nested`)
	require.NoError(t, err)
	assert.Equal(t, StoreGetArgs{Store: "ssm", Key: "app-key", Query: ".value | .nested"}, actual)

	actual, err = ParseStoreGet(`ssm app-key | default "disabled"`)
	require.NoError(t, err)
	assert.Equal(t, "disabled", *actual.Default)

	actual, err = ParseStoreGet(`ssm app-key | default ""`)
	require.NoError(t, err)
	require.NotNil(t, actual.Default)
	assert.Equal(t, "", *actual.Default)

	_, err = ParseStoreGet("ssm")
	require.Error(t, err)
	_, err = ParseStoreGet("")
	require.Error(t, err)
	_, err = ParseStoreGet("ssm key | query")
	require.Error(t, err)
	_, err = ParseStoreGet("ssm key | default | query .value")
	require.Error(t, err)
	_, err = ParseStoreGet("ssm key | unknown value")
	require.Error(t, err)
}

func TestParseStoreRejectsMissingOptionValue(t *testing.T) {
	_, err := ParseStore("ssm vpc id | default")
	require.Error(t, err)

	var parseErr *Error
	require.ErrorAs(t, err, &parseErr)
	assert.Equal(t, "expected option value", parseErr.Message)
}

func TestParserHelpers(t *testing.T) {
	parts, err := readLegacyCSV(`component ".value // ""fallback"""`)
	require.NoError(t, err)
	assert.Equal(t, []string{"component", `.value // "fallback"`}, parts)

	assert.Equal(t, "plain", unquote("plain"))
	assert.Equal(t, "double", unquote(`"double"`))
	assert.Equal(t, "single", unquote(`'single'`))
	assert.Equal(t, `"unterminated`, unquote(`"unterminated`))
	for _, value := range []string{".value", "[0]", "{\"key\":\"value\"}", "| .value", `"value"`, "'value'"} {
		assert.True(t, isExpressionStart(value))
	}
	assert.False(t, isExpressionStart("stack"))
	assert.False(t, isExpressionStart(""))
	assert.Equal(t, "Unknown", symbolName(map[string]lexer.TokenType{}, lexer.TokenType(999)))

	_, err = terraformFromLegacy([]string{"component", "stack", "output"})
	require.NoError(t, err)
	_, err = terraformFromLegacy([]string{"component"})
	require.Error(t, err)
	_, err = validateTerraform(TerraformArgs{}, token{})
	require.Error(t, err)
	_, err = validateTerraform(TerraformArgs{Component: "component"}, token{})
	require.Error(t, err)
	_, err = words("store | query .value")
	require.Error(t, err)
	_, _, err = parseOptions("| default", []token{{typeName: tokenPipe}, {typeName: tokenText, value: "default"}}, 0)
	require.Error(t, err)

	_, err = ParseEnv(`"unterminated`)
	require.Error(t, err)
	assert.NotEmpty(t, err.Error())
}

func stringPtr(value string) *string {
	return &value
}
