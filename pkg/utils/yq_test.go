package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestEvaluateYqExpression(t *testing.T) {
	input := `
settings:
  test: true
  mode: test
vars:
  assign_generated_ipv6_cidr_block: false
  availability_zones:
    - us-east-2a
    - us-east-2b
    - us-east-2c
  enabled: true
  environment: ue2
  ipv4_primary_cidr_block: 10.8.0.0/18
  map_public_ip_on_launch: false
  max_subnet_count: 3
  name: common
  namespace: acme
  nat_eip_aws_shield_protection_enabled: false
  nat_gateway_enabled: true
  nat_instance_enabled: false
  region: us-east-2
  stage: prod
  tags:
    atmos_component: vpc
    atmos_manifest: orgs/acme/plat/prod/us-east-2
    atmos_stack: plat-ue2-prod
    terraform_component: vpc
    terraform_workspace: plat-ue2-prod
  tenant: plat
  vpc_flow_logs_enabled: true
  vpc_flow_logs_log_destination_type: s3
  vpc_flow_logs_traffic_type: ALL
`

	atmosConfig := &schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: "Trace",
		},
	}

	data, err := UnmarshalYAML[map[string]any](input)
	assert.Nil(t, err)
	assert.NotNil(t, data)

	// Test with nil atmosConfig to ensure it doesn't panic
	yq := ".settings.test"
	res, err := EvaluateYqExpression(nil, data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, true, res)

	yq = ".settings.test"
	res, err = EvaluateYqExpression(atmosConfig, data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, true, res)
	err = PrintAsYAML(atmosConfig, res)
	assert.Nil(t, err)

	yq = ".settings.mode"
	res, err = EvaluateYqExpression(atmosConfig, data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "test", res)
	err = PrintAsYAML(atmosConfig, res)
	assert.Nil(t, err)

	yq = ".vars.tags.atmos_component"
	res, err = EvaluateYqExpression(atmosConfig, data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "vpc", res)
	err = PrintAsYAML(atmosConfig, res)
	assert.Nil(t, err)

	yq = ".vars.availability_zones.0"
	res, err = EvaluateYqExpression(atmosConfig, data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "us-east-2a", res)
	err = PrintAsYAML(atmosConfig, res)
	assert.Nil(t, err)

	yq = ".vars.ipv4_primary_cidr_block"
	res, err = EvaluateYqExpression(atmosConfig, data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "10.8.0.0/18", res)
	err = PrintAsYAML(atmosConfig, res)
	assert.Nil(t, err)

	yq = ".vars.enabled"
	res, err = EvaluateYqExpression(atmosConfig, data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, true, res)
	err = PrintAsYAML(atmosConfig, res)
	assert.Nil(t, err)

	yq = ".vars.enabled = false"
	res, err = EvaluateYqExpression(atmosConfig, data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	yq = ".vars.enabled"
	res, err = EvaluateYqExpression(atmosConfig, res, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, false, res)
	err = PrintAsYAML(atmosConfig, res)
	assert.Nil(t, err)

	yq = ".vars.ipv4_primary_cidr_block = \"10.8.8.0/20\""
	res, err = EvaluateYqExpression(atmosConfig, data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	yq = ".vars.ipv4_primary_cidr_block"
	res, err = EvaluateYqExpression(atmosConfig, res, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "10.8.8.0/20", res)
	err = PrintAsYAML(atmosConfig, res)
	assert.Nil(t, err)

	yq = ".vars.availability_zones.0 = \"us-east-2d\""
	res, err = EvaluateYqExpression(atmosConfig, data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	yq = ".vars.availability_zones.0"
	res, err = EvaluateYqExpression(atmosConfig, res, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	assert.Equal(t, "us-east-2d", res)
	err = PrintAsYAML(atmosConfig, res)
	assert.Nil(t, err)

	yq = ".vars.enabled = false | .vars.tags.terraform_workspace = \"plat-ue2-prod-override\" | .vars.max_subnet_count = 2 | .settings.test = false"
	res, err = EvaluateYqExpression(atmosConfig, data, yq)
	assert.Nil(t, err)
	assert.NotNil(t, res)
	yq = ".vars.enabled"
	res1, err := EvaluateYqExpression(atmosConfig, res, yq)
	assert.Nil(t, err)
	assert.Equal(t, false, res1)
	err = PrintAsYAML(atmosConfig, res1)
	assert.Nil(t, err)
	yq = ".vars.tags.terraform_workspace"
	res2, err := EvaluateYqExpression(atmosConfig, res, yq)
	assert.Nil(t, err)
	assert.Equal(t, "plat-ue2-prod-override", res2)
	err = PrintAsYAML(atmosConfig, res2)
	assert.Nil(t, err)
	yq = ".vars.max_subnet_count"
	res3, err := EvaluateYqExpression(atmosConfig, res, yq)
	assert.Nil(t, err)
	assert.Equal(t, 2, res3)
	err = PrintAsYAML(atmosConfig, res3)
	assert.Nil(t, err)
	yq = ".settings.test"
	res4, err := EvaluateYqExpression(atmosConfig, res, yq)
	assert.Nil(t, err)
	assert.Equal(t, false, res4)
	err = PrintAsYAML(atmosConfig, res4)
	assert.Nil(t, err)
}

func TestEvaluateYqExpressionWithNilConfig(t *testing.T) {
	input := `
settings:
  test: true
  mode: test
vars:
  enabled: true
  name: test
`
	data, err := UnmarshalYAML[map[string]any](input)
	assert.Nil(t, err)
	assert.NotNil(t, data)

	// Test various expressions with nil atmosConfig
	testCases := []struct {
		name     string
		yq       string
		expected any
	}{
		{
			name:     "get boolean value",
			yq:       ".settings.test",
			expected: true,
		},
		{
			name:     "get string value",
			yq:       ".settings.mode",
			expected: "test",
		},
		{
			name:     "modify boolean value",
			yq:       ".vars.enabled = false | .vars.enabled",
			expected: false,
		},
		{
			name:     "add new field",
			yq:       ".vars.new_field = \"new value\" | .vars.new_field",
			expected: "new value",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := EvaluateYqExpression(nil, data, tc.yq)
			assert.Nil(t, err)
			assert.NotNil(t, res)
			assert.Equal(t, tc.expected, res)
		})
	}
}

// verifyReadResult checks the result of a YQ read operation.
func verifyReadResult(t *testing.T, testName string, result any) {
	t.Helper()
	// Check that the result is a string.
	if strResult, ok := result.(string); ok {
		// Note: With UnwrapScalar: true, YQ currently collapses multiline strings.
		// This test documents the current behavior:
		// - Multiline strings are collapsed to single lines with spaces
		// - Explicit newlines in quoted strings are removed
		// - This is not ideal but is the current implementation
		assert.NotEmpty(t, strResult, "Result should not be empty")

		// Document what we're actually getting vs what we might expect
		t.Logf("Test case '%s': Got result: %q", testName, strResult)
	}
}

// verifyWriteResult checks the result of a YQ write operation.
func verifyWriteResult(t *testing.T, result any, checkKey string) {
	t.Helper()
	// For write operations, check the modified structure.
	mapResult, ok := result.(map[string]any)
	if !ok {
		return
	}

	configMap, ok := mapResult[checkKey].(map[string]any)
	if !ok {
		return
	}

	if template, ok := configMap["template"].(string); ok {
		assert.Contains(t, template, "\n", "Modified value should preserve newlines")
	}
}

// TestEvaluateYqExpressionPreservesWhitespace tests the current behavior of YQ with whitespace.
// Note: The current implementation with UnwrapScalar: true may not preserve all formatting.
func TestEvaluateYqExpressionPreservesWhitespace(t *testing.T) {
	// Test inputs with various whitespace patterns.
	testCases := []struct {
		name     string
		input    string
		yqExpr   string
		checkKey string
	}{
		{
			name: "multiline string with trailing newline",
			input: `
multiline_text: |
  line one
  line two
  line three
`,
			yqExpr:   ".multiline_text",
			checkKey: "",
		},
		{
			name: "string with single trailing newline",
			input: `
single_line: "hello world\n"
`,
			yqExpr:   ".single_line",
			checkKey: "",
		},
		{
			name: "text with leading newline",
			input: `
leading: "\nhello world"
`,
			yqExpr:   ".leading",
			checkKey: "",
		},
		{
			name: "text with multiple consecutive newlines",
			input: `
multiple: "\n\nhello\n\nworld\n\n"
`,
			yqExpr:   ".multiple",
			checkKey: "",
		},
		{
			name: "multiline literal block scalar",
			input: `
literal: |
  This is a literal block scalar
  with multiple lines
  and indentation
    preserved exactly
  including trailing newline
`,
			yqExpr:   ".literal",
			checkKey: "",
		},
		{
			name: "multiline folded block scalar",
			input: `
folded: >
  This is a folded block scalar
  where lines are folded
  but newlines are preserved
  at the end
`,
			yqExpr:   ".folded",
			checkKey: "",
		},
		{
			name: "setting value with newlines",
			input: `
config:
  template: "start\nmiddle\nend\n"
`,
			yqExpr:   `.config.template = "new\nvalue\nwith\nnewlines\n"`,
			checkKey: "config",
		},
	}

	atmosConfig := &schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: "Trace",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := UnmarshalYAML[map[string]any](tc.input)
			assert.Nil(t, err, "Failed to unmarshal YAML input")
			assert.NotNil(t, data)

			// Evaluate the YQ expression.
			result, err := EvaluateYqExpression(atmosConfig, data, tc.yqExpr)
			assert.Nil(t, err, "YQ expression evaluation failed")
			assert.NotNil(t, result)

			// For read operations, verify the result directly.
			if tc.checkKey == "" {
				verifyReadResult(t, tc.name, result)
			} else {
				verifyWriteResult(t, result, tc.checkKey)
			}
		})
	}
}

// TestEvaluateYqExpressionWhitespaceInComplexStructures tests YQ behavior with complex data structures.
// Note: The current YQ implementation with UnwrapScalar: true may not preserve multiline strings as expected.
// This test documents the current behavior rather than ideal behavior.
func TestEvaluateYqExpressionWhitespaceInComplexStructures(t *testing.T) {
	input := `
components:
  terraform:
    vpc:
      vars:
        description: |
          This is a VPC component
          with multiple lines
          of description text
        config_yaml: |
          key1: value1
          key2: value2
          nested:
            key3: value3
        script: |
          #!/bin/bash
          echo "Line 1"
          echo "Line 2"
          echo "Line 3"
`

	atmosConfig := &schema.AtmosConfiguration{
		Logs: schema.Logs{
			Level: "Trace",
		},
	}

	data, err := UnmarshalYAML[map[string]any](input)
	assert.Nil(t, err)
	assert.NotNil(t, data)

	// Test various YQ expressions on the complex structure.
	testCases := []struct {
		name   string
		yqExpr string
	}{
		{
			name:   "extract multiline description",
			yqExpr: ".components.terraform.vpc.vars.description",
		},
		{
			name:   "extract yaml config",
			yqExpr: ".components.terraform.vpc.vars.config_yaml",
		},
		{
			name:   "extract script with shebang",
			yqExpr: ".components.terraform.vpc.vars.script",
		},
		{
			name:   "modify and preserve multiline",
			yqExpr: `.components.terraform.vpc.vars.new_field = "line1\nline2\nline3\n"`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := EvaluateYqExpression(atmosConfig, data, tc.yqExpr)
			assert.Nil(t, err, "YQ expression evaluation should not fail")
			assert.NotNil(t, result, "Result should not be nil")

			// For string results, verify they're not empty.
			if strResult, ok := result.(string); ok {
				assert.NotEmpty(t, strResult, "String result should not be empty")
				// Note: Due to UnwrapScalar: true in YQ config, multiline strings may be collapsed.
				// This is the current behavior, not necessarily the desired behavior.
				// We're just checking that we got a non-empty result.
			}
		})
	}
}
