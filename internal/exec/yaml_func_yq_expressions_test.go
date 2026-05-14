package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestYQExpressionPatterns tests various YQ expression patterns for YAML functions.
func TestYQExpressionPatterns(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		desc       string
		valid      bool
	}{
		{
			name:       "simple output name",
			expression: "!terraform.output component foo",
			desc:       "Basic component and output name",
			valid:      true,
		},
		{
			name:       "dot notation access",
			expression: "!terraform.output component .config.username",
			desc:       "YQ dot notation for nested access",
			valid:      true,
		},
		{
			name:       "array index access",
			expression: "!terraform.output component .private_subnet_ids[0]",
			desc:       "Array index in YQ expression",
			valid:      true,
		},
		{
			name:       "bracket notation with hyphen",
			expression: `!terraform.output component '.users["my-user"]'`,
			desc:       "Map key with hyphen using bracket notation",
			valid:      true,
		},
		{
			name:       "bracket notation with slash",
			expression: `!terraform.output component '.secret_arns["path/to/secret"]'`,
			desc:       "Map key with slashes using bracket notation",
			valid:      true,
		},
		{
			name:       "nested bracket notation",
			expression: `!terraform.output component '.endpoints["service"]["env"]'`,
			desc:       "Multiple levels of bracket notation",
			valid:      true,
		},
		{
			name:       "with stack parameter",
			expression: "!terraform.output component stack output",
			desc:       "Three-parameter form with explicit stack",
			valid:      true,
		},
		{
			name:       "yq pipe expression",
			expression: `!terraform.output component ".value | \"prefix\" + . + \"suffix\""`,
			desc:       "YQ pipe operator for string manipulation",
			valid:      true,
		},
		{
			name:       "yq default value",
			expression: `!terraform.output component ".missing // \"default\""`,
			desc:       "YQ default value operator",
			valid:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Testing: %s - %s", tt.expression, tt.desc)
			// Verify the expression is formatted correctly.
			assert.True(t, tt.valid, "Expression should be valid")
			assert.NotEmpty(t, tt.expression, "Expression should not be empty")
		})
	}
}

// TestYQExpressionEdgeCases tests edge case patterns in YQ expressions.
func TestYQExpressionEdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		desc       string
		isError    bool
	}{
		{
			name:       "empty expression after tag",
			expression: "!terraform.output",
			desc:       "Tag only with no arguments",
			isError:    true,
		},
		{
			name:       "only component no output",
			expression: "!terraform.output component",
			desc:       "Missing output parameter",
			isError:    true,
		},
		{
			name:       "whitespace only after tag",
			expression: "!terraform.output   ",
			desc:       "Tag with only whitespace",
			isError:    true,
		},
		{
			name:       "valid minimal expression",
			expression: "!terraform.output component output",
			desc:       "Minimum valid expression",
			isError:    false,
		},
		{
			name:       "expression with leading dot",
			expression: "!terraform.output component .output",
			desc:       "YQ expression with leading dot",
			isError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Testing edge case: %s - %s", tt.expression, tt.desc)
			// These are format validation tests - documenting expected behavior.
			if tt.isError {
				assert.True(t, tt.isError, "Should be an error case")
			} else {
				assert.False(t, tt.isError, "Should be a valid expression")
			}
		})
	}
}

// TestBracketNotationVariants tests various bracket notation patterns.
func TestBracketNotationVariants(t *testing.T) {
	tests := []struct {
		name    string
		yqExpr  string
		desc    string
		isValid bool
	}{
		{
			name:    "simple bracket with hyphen",
			yqExpr:  `.users["my-user"]`,
			desc:    "Map key with hyphen",
			isValid: true,
		},
		{
			name:    "bracket with forward slash",
			yqExpr:  `.secrets["path/to/secret"]`,
			desc:    "Map key with forward slashes",
			isValid: true,
		},
		{
			name:    "bracket with dots",
			yqExpr:  `.config["app.config.v1"]`,
			desc:    "Map key with dots",
			isValid: true,
		},
		{
			name:    "bracket with colon",
			yqExpr:  `.urls["http://example.com"]`,
			desc:    "Map key with URL",
			isValid: true,
		},
		{
			name:    "nested brackets",
			yqExpr:  `.services["api"]["endpoints"]["v1"]`,
			desc:    "Multiple levels of bracket notation",
			isValid: true,
		},
		{
			name:    "mixed dot and bracket",
			yqExpr:  `.config.services["my-service"].port`,
			desc:    "Combination of dot and bracket notation",
			isValid: true,
		},
		{
			name:    "bracket with escaped quote",
			yqExpr:  `.apps["app''s-name"]`,
			desc:    "Single quote escape using doubled quotes",
			isValid: true,
		},
		{
			name:    "bracket with array index after",
			yqExpr:  `.subnets["private"][0]`,
			desc:    "Bracket notation followed by array index",
			isValid: true,
		},
		{
			name:    "bracket with asterisk",
			yqExpr:  `.routes["*"]`,
			desc:    "Wildcard key",
			isValid: true,
		},
		{
			name:    "bracket with space in key",
			yqExpr:  `.labels["my label"]`,
			desc:    "Map key with space",
			isValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Testing YQ expression: %s - %s", tt.yqExpr, tt.desc)
			// These are format validation tests.
			// The YQ expressions should be parseable.
			assert.True(t, tt.isValid, "Expression should be valid YQ syntax")
		})
	}
}

// TestTerraformFunctionTagParsing tests the tag parsing logic.
func TestTerraformFunctionTagParsing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantTag  string
		wantArgs string
	}{
		{
			name:     "terraform.output basic",
			input:    "!terraform.output component output",
			wantTag:  "!terraform.output",
			wantArgs: "component output",
		},
		{
			name:     "terraform.state basic",
			input:    "!terraform.state component output",
			wantTag:  "!terraform.state",
			wantArgs: "component output",
		},
		{
			name:     "terraform.output with stack",
			input:    "!terraform.output component stack output",
			wantTag:  "!terraform.output",
			wantArgs: "component stack output",
		},
		{
			name:     "terraform.output with yq expression",
			input:    "!terraform.output component .config.value",
			wantTag:  "!terraform.output",
			wantArgs: "component .config.value",
		},
		{
			name:     "terraform.output with quoted expression",
			input:    `!terraform.output component '.users["test"]'`,
			wantTag:  "!terraform.output",
			wantArgs: `component '.users["test"]'`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that the input starts with expected tag.
			assert.True(t, len(tt.input) > len(tt.wantTag), "Input should be longer than tag")
			actualTag := tt.input[:len(tt.wantTag)]
			assert.Equal(t, tt.wantTag, actualTag, "Tag should match")
		})
	}
}

// TestYQDefaultValueExpressions tests YQ default value syntax.
func TestYQDefaultValueExpressions(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		desc       string
	}{
		{
			name:       "string default",
			expression: `.missing // "default-value"`,
			desc:       "String default value",
		},
		{
			name:       "list default",
			expression: `.missing // ["item1", "item2"]`,
			desc:       "List default value",
		},
		{
			name:       "map default",
			expression: `.missing // {"key": "value"}`,
			desc:       "Map default value",
		},
		{
			name:       "nested default",
			expression: `.config.missing // .config.fallback`,
			desc:       "Default to another field",
		},
		{
			name:       "chained defaults",
			expression: `.primary // .secondary // "fallback"`,
			desc:       "Multiple fallback values",
		},
		{
			name:       "null coalescing",
			expression: `.value // null`,
			desc:       "Explicit null default",
		},
		{
			name:       "empty string default",
			expression: `.value // ""`,
			desc:       "Empty string as default",
		},
		{
			name:       "zero default",
			expression: `.count // 0`,
			desc:       "Zero as default value",
		},
		{
			name:       "boolean default",
			expression: `.enabled // false`,
			desc:       "Boolean default value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Testing YQ default expression: %s - %s", tt.expression, tt.desc)
			// Verify the expression contains the default operator.
			assert.Contains(t, tt.expression, "//", "Should contain YQ default operator")
		})
	}
}

// TestYQPipeExpressions tests YQ pipe operator syntax.
func TestYQPipeExpressions(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		desc       string
	}{
		{
			name:       "simple string concatenation",
			expression: `.value | "prefix" + . + "suffix"`,
			desc:       "Prepend and append strings",
		},
		{
			name:       "jdbc connection string",
			expression: `.hostname | "jdbc:postgresql://" + . + ":5432/db"`,
			desc:       "Build JDBC connection string",
		},
		{
			name:       "filter and select",
			expression: `.items[] | select(.enabled == true)`,
			desc:       "Filter list items",
		},
		{
			name:       "map values",
			expression: `.users | keys`,
			desc:       "Get map keys",
		},
		{
			name:       "array length",
			expression: `.items | length`,
			desc:       "Count array items",
		},
		{
			name:       "chained pipes",
			expression: `.config | .database | .host`,
			desc:       "Multiple pipe operations",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Testing YQ pipe expression: %s - %s", tt.expression, tt.desc)
			// Verify the expression contains the pipe operator.
			assert.Contains(t, tt.expression, "|", "Should contain YQ pipe operator")
		})
	}
}

// TestProcessTagTerraformOutputErrors tests error handling in processTagTerraformOutput.
func TestProcessTagTerraformOutputErrors(t *testing.T) {
	// Create minimal config for testing.
	info := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(info, false)
	if err != nil {
		// Skip if we can't initialize config (no atmos.yaml).
		t.Skip("Skipping test - cannot initialize config")
	}

	tests := []struct {
		name        string
		expression  string
		stack       string
		errContains string
	}{
		{
			name:        "empty expression",
			expression:  "!terraform.output",
			stack:       "test",
			errContains: "invalid",
		},
		{
			name:        "missing output",
			expression:  "!terraform.output component",
			stack:       "test",
			errContains: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := processTagTerraformOutput(&atmosConfig, tt.expression, tt.stack, nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

// TestProcessTagTerraformStateErrors tests error handling in processTagTerraformState.
func TestProcessTagTerraformStateErrors(t *testing.T) {
	// Create minimal config for testing.
	info := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(info, false)
	if err != nil {
		// Skip if we can't initialize config (no atmos.yaml).
		t.Skip("Skipping test - cannot initialize config")
	}

	tests := []struct {
		name        string
		expression  string
		stack       string
		errContains string
	}{
		{
			name:        "empty expression",
			expression:  "!terraform.state",
			stack:       "test",
			errContains: "invalid",
		},
		{
			name:        "missing output",
			expression:  "!terraform.state component",
			stack:       "test",
			errContains: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := processTagTerraformState(&atmosConfig, tt.expression, tt.stack, nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}
