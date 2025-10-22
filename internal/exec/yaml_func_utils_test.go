package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestGetStringAfterTag(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		tag           string
		expected      string
		expectedError string
	}{
		{
			name:     "basic tag",
			input:    "!template path/to/file.yaml",
			tag:      "!template",
			expected: "path/to/file.yaml",
		},
		{
			name:     "tag with spaces",
			input:    "!template    path/with/spaces.yaml   ",
			tag:      "!template",
			expected: "path/with/spaces.yaml",
		},
		{
			name:     "tag with special characters",
			input:    "!template@# path/with/special@#.yaml",
			tag:      "!template@#",
			expected: "path/with/special@#.yaml",
		},
		{
			name:          "empty input",
			input:         "",
			tag:           "!template",
			expectedError: "invalid Atmos YAML function: ",
		},
		{
			name:     "tag not at start",
			input:    "some prefix !template path/to/file.yaml",
			tag:      "!template",
			expected: "some prefix !template path/to/file.yaml",
		},
		{
			name:     "multiple spaces after tag",
			input:    "!template    multiple   spaces.yaml",
			tag:      "!template",
			expected: "multiple   spaces.yaml",
		},
		{
			name:     "tag with newline",
			input:    "!template\npath/with/newline.yaml",
			tag:      "!template",
			expected: "path/with/newline.yaml",
		},
		{
			name:     "tag with tab",
			input:    "!template\tpath/with/tab.yaml",
			tag:      "!template",
			expected: "path/with/tab.yaml",
		},
		{
			name:     "empty tag",
			input:    "!template path/to/file.yaml",
			tag:      "",
			expected: "!template path/to/file.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getStringAfterTag(tt.input, tt.tag)

			// Check error cases
			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("expected error to contain %q, got %q", tt.expectedError, err.Error())
				}
				return
			}

			// Check normal cases
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestGetStringAfterTag_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		tag      string
		expected string
	}{
		{
			name:     "tag not found",
			input:    "some string",
			tag:      "!template",
			expected: "some string",
		},
		{
			name:     "unicode characters",
			input:    "!templaté pâth/with/ünïcødé.yml",
			tag:      "!templaté",
			expected: "pâth/with/ünïcødé.yml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := getStringAfterTag(tt.input, tt.tag)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestSkipFunc(t *testing.T) {
	tests := []struct {
		name     string
		skip     []string
		function string
		expected bool
	}{
		{
			name:     "empty skip list",
			skip:     []string{},
			function: "!template",
			expected: false,
		},
		{
			name:     "function not in skip list",
			skip:     []string{"!exec", "!store"},
			function: "!template",
			expected: false,
		},
		{
			name:     "function with negation in skip list",
			skip:     []string{"!exec", "!!template", "!store"},
			function: "!template",
			expected: false,
		},
		{
			name:     "empty function",
			skip:     []string{"!exec", "!template", "!store"},
			function: "",
			expected: false,
		},
		{
			name:     "case sensitive match",
			skip:     []string{"!Template", "!Exec"},
			function: "!template",
			expected: false,
		},
		{
			name:     "exact match required",
			skip:     []string{"!template"},
			function: "!template:extra",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := skipFunc(tt.skip, tt.function)
			if result != tt.expected {
				t.Errorf("expected %v, got %v for function %q and skip list %v",
					tt.expected, result, tt.function, tt.skip)
			}
		})
	}
}

func TestSkipFunc_EdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		skip     []string
		function string
		expected bool
	}{
		{
			name:     "nil skip list",
			skip:     nil,
			function: "!template",
			expected: false,
		},
		{
			name:     "function with leading/trailing spaces",
			skip:     []string{"  !template  "},
			function: "!template",
			expected: false, // Because "  !template  " != "!template"
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := skipFunc(tt.skip, tt.function)
			if result != tt.expected {
				t.Errorf("expected %v, got %v for function %q and skip list %v",
					tt.expected, result, tt.function, tt.skip)
			}
		})
	}
}

func TestProcessCustomYamlTags(t *testing.T) {
	err := os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_CLI_CONFIG_PATH': %v", err)
	}

	err = os.Unsetenv("ATMOS_BASE_PATH")
	if err != nil {
		t.Fatalf("Failed to unset 'ATMOS_BASE_PATH': %v", err)
	}

	log.SetLevel(log.InfoLevel)
	log.SetOutput(os.Stdout)

	stack := "nonprod"

	defer func() {
		// Delete the generated files and folders after the test
		err := os.RemoveAll(filepath.Join("..", "..", "components", "terraform", "mock", ".terraform"))
		assert.NoError(t, err)

		err = os.RemoveAll(filepath.Join("..", "..", "components", "terraform", "mock", "terraform.tfstate.d"))
		assert.NoError(t, err)
	}()

	// Define the working directory
	workDir := "../../tests/fixtures/scenarios/atmos-terraform-state-yaml-function"
	t.Chdir(workDir)

	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            stack,
		StackFile:        "",
		ComponentType:    "terraform",
		ComponentFromArg: "component-1",
		SubCommand:       "deploy",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	err = ExecuteTerraform(info)
	if err != nil {
		t.Fatalf("Failed to execute 'ExecuteTerraform': %v", err)
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	assert.NoError(t, err)

	d := processTagTerraformState(&atmosConfig, "!terraform.state component-1 foo", stack, nil)
	assert.Equal(t, "component-1-a", d)

	d = processTagTerraformState(&atmosConfig, "!terraform.state component-1 bar", stack, nil)
	assert.Equal(t, "component-1-b", d)

	d = processTagTerraformState(&atmosConfig, "!terraform.state component-1 nonprod baz", "", nil)
	assert.Equal(t, "component-1-c", d)

	res, err := ExecuteDescribeComponent(
		"component-2",
		stack,
		true,
		true,
		nil,
	)
	assert.NoError(t, err)

	y, err := u.ConvertToYAML(res)
	assert.Nil(t, err)
	assert.Contains(t, y, "foo: component-1-a")
	assert.Contains(t, y, "bar: component-1-b")
	assert.Contains(t, y, "baz: component-1-c")

	info = schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            stack,
		StackFile:        "",
		ComponentType:    "terraform",
		ComponentFromArg: "component-2",
		SubCommand:       "deploy",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	err = ExecuteTerraform(info)
	if err != nil {
		t.Fatalf("Failed to execute 'ExecuteTerraform': %v", err)
	}

	d = processTagTerraformState(&atmosConfig, "!terraform.state component-2 foo", stack, nil)
	assert.Equal(t, "component-1-a", d)

	d = processTagTerraformState(&atmosConfig, "!terraform.state component-2 nonprod bar", stack, nil)
	assert.Equal(t, "component-1-b", d)

	d = processTagTerraformState(&atmosConfig, "!terraform.state component-2 nonprod baz", "", nil)
	assert.Equal(t, "component-1-c", d)

	res, err = ExecuteDescribeComponent(
		"component-3",
		stack,
		true,
		false,
		nil,
	)
	assert.NoError(t, err)

	processed, err := ProcessCustomYamlTags(&atmosConfig, res, stack, []string{}, nil)
	assert.NoError(t, err)

	val, err := u.EvaluateYqExpression(&atmosConfig, processed, ".vars.foo")
	assert.NoError(t, err)
	assert.Equal(t, "component-1-a", val)

	val, err = u.EvaluateYqExpression(&atmosConfig, processed, ".vars.bar")
	assert.NoError(t, err)
	assert.Equal(t, "component-1-b", val)

	val, err = u.EvaluateYqExpression(&atmosConfig, processed, ".vars.baz")
	assert.NoError(t, err)
	assert.Equal(t, "default-value", val)

	val, err = u.EvaluateYqExpression(&atmosConfig, processed, ".vars.test_map.key1")
	assert.NoError(t, err)
	assert.Equal(t, "fallback1", val)

	val, err = u.EvaluateYqExpression(&atmosConfig, processed, ".vars.test_list[1]")
	assert.NoError(t, err)
	assert.Equal(t, "fallback2", val)

	val, err = u.EvaluateYqExpression(&atmosConfig, processed, ".vars.test_val")
	assert.NoError(t, err)
	assert.Equal(t, "jdbc:postgresql://component-1-a:5432/events", val)
}
