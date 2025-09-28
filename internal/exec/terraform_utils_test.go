package exec

import (
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests"
)

// Helper function to create a bool pointer for testing.
func boolPtr(b bool) *bool {
	return &b
}

func TestIsWorkspacesEnabled(t *testing.T) {
	// Test cases for isWorkspacesEnabled function.
	tests := []struct {
		name              string
		backendType       string
		workspacesEnabled *bool
		expectedEnabled   bool
		expectWarning     bool
	}{
		{
			name:              "Default behavior (no explicit setting, non-HTTP backend)",
			backendType:       "s3",
			workspacesEnabled: nil,
			expectedEnabled:   true,
			expectWarning:     false,
		},
		{
			name:              "HTTP backend automatically disables workspaces",
			backendType:       "http",
			workspacesEnabled: nil,
			expectedEnabled:   false,
			expectWarning:     false,
		},
		{
			name:              "Explicitly disabled workspaces",
			backendType:       "s3",
			workspacesEnabled: boolPtr(false),
			expectedEnabled:   false,
			expectWarning:     false,
		},
		{
			name:              "Explicitly enabled workspaces",
			backendType:       "s3",
			workspacesEnabled: boolPtr(true),
			expectedEnabled:   true,
			expectWarning:     false,
		},
		{
			name:              "HTTP backend ignores explicitly enabled workspaces with warning",
			backendType:       "http",
			workspacesEnabled: boolPtr(true),
			expectedEnabled:   false,
			expectWarning:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup test config.
			atmosConfig := &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						WorkspacesEnabled: tc.workspacesEnabled,
					},
				},
			}

			info := &schema.ConfigAndStacksInfo{
				ComponentBackendType: tc.backendType,
				Component:            "test-component",
			}

			// Test function.
			result := isWorkspacesEnabled(atmosConfig, info)

			// Assert results.
			assert.Equal(t, tc.expectedEnabled, result, "Expected workspace enabled status to match")
		})
	}
}

func TestExecuteTerraformAffectedWithDependents(t *testing.T) {
	// Check for valid Git remote URL before running test
	tests.RequireGitRemoteWithValidURL(t)

	os.Unsetenv("ATMOS_BASE_PATH")
	os.Unsetenv("ATMOS_CLI_CONFIG_PATH")

	// Capture the starting working directory
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}

	defer func() {
		// Change back to the original working directory after the test
		if err := os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to the starting directory: %v", err)
		}
	}()

	// Define the work directory and change to it
	workDir := "../../tests/fixtures/scenarios/terraform-apply-affected"
	if err = os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	oldStd := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w

	stack := "prod"

	info := schema.ConfigAndStacksInfo{
		Stack:         stack,
		ComponentType: "terraform",
		SubCommand:    "plan",
		Affected:      true,
		DryRun:        true,
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		t.Fatalf("Failed to execute 'InitCliConfig': %v", err)
	}

	a := DescribeAffectedCmdArgs{
		CLIConfig:         &atmosConfig,
		Stack:             stack,
		IncludeDependents: true,
		CloneTargetRef:    true,
	}

	err = ExecuteTerraformAffected(&a, &info)
	if err != nil {
		t.Fatalf("Failed to execute 'ExecuteTerraformAffected': %v", err)
	}

	w.Close()
	os.Stderr = oldStd
}

func TestExecuteTerraformQuery(t *testing.T) {
	os.Unsetenv("ATMOS_BASE_PATH")
	os.Unsetenv("ATMOS_CLI_CONFIG_PATH")

	// Capture the starting working directory
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}

	defer func() {
		// Change back to the original working directory after the test
		if err := os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to the starting directory: %v", err)
		}
	}()

	// Define the work directory and change to it
	workDir := "../../tests/fixtures/scenarios/terraform-apply-affected"
	if err = os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

	oldStd := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w

	stack := "prod"

	info := schema.ConfigAndStacksInfo{
		Stack:         stack,
		ComponentType: "terraform",
		SubCommand:    "plan",
		DryRun:        true,
		Query:         ".vars.tags.team == \"eks\"",
	}

	err = ExecuteTerraformQuery(&info)
	if err != nil {
		t.Fatalf("Failed to execute 'ExecuteTerraformQuery': %v", err)
	}

	w.Close()
	os.Stderr = oldStd
}

// TestWalkTerraformComponents verifies that walkTerraformComponents iterates over all components.
func TestWalkTerraformComponents(t *testing.T) {
	stacks := map[string]any{
		"stack1": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"comp1": map[string]any{},
					"comp2": map[string]any{},
				},
			},
		},
	}

	var visited []string
	err := walkTerraformComponents(stacks, func(stack, comp string, section map[string]any) error {
		visited = append(visited, stack+"-"+comp)
		return nil
	})
	assert.NoError(t, err)
	assert.ElementsMatch(t, []string{"stack1-comp1", "stack1-comp2"}, visited)
}

// TestProcessTerraformComponent exercises the filtering logic of processTerraformComponent.
func TestProcessTerraformComponent(t *testing.T) {
	atmosConfig := schema.AtmosConfiguration{}
	logFunc := func(msg interface{}, keyvals ...interface{}) {}
	stack := "s1"
	component := "comp1"

	newSection := func(meta map[string]any) map[string]any {
		return map[string]any{
			cfg.MetadataSectionName: meta,
			"vars": map[string]any{
				"tags": map[string]any{"team": "eks"},
			},
		}
	}

	t.Run("abstract", func(t *testing.T) {
		section := newSection(map[string]any{"type": "abstract"})
		called := false
		patch := gomonkey.ApplyFunc(ExecuteTerraform, func(i schema.ConfigAndStacksInfo) error {
			called = true
			return nil
		})
		defer patch.Reset()

		info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
		err := processTerraformComponent(&atmosConfig, &info, stack, component, section, logFunc)
		assert.NoError(t, err)
		assert.False(t, called)
	})

	t.Run("disabled", func(t *testing.T) {
		section := newSection(map[string]any{"enabled": false})
		called := false
		patch := gomonkey.ApplyFunc(ExecuteTerraform, func(i schema.ConfigAndStacksInfo) error {
			called = true
			return nil
		})
		defer patch.Reset()

		info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
		err := processTerraformComponent(&atmosConfig, &info, stack, component, section, logFunc)
		assert.NoError(t, err)
		assert.False(t, called)
	})

	t.Run("query not satisfied", func(t *testing.T) {
		section := newSection(map[string]any{"enabled": true})
		called := false
		patch := gomonkey.ApplyFunc(ExecuteTerraform, func(i schema.ConfigAndStacksInfo) error {
			called = true
			return nil
		})
		defer patch.Reset()

		info := schema.ConfigAndStacksInfo{SubCommand: "plan", Query: ".vars.tags.team == \"foo\""}
		err := processTerraformComponent(&atmosConfig, &info, stack, component, section, logFunc)
		assert.NoError(t, err)
		assert.False(t, called)
	})

	t.Run("execute", func(t *testing.T) {
		section := newSection(map[string]any{"enabled": true})
		called := false
		patch := gomonkey.ApplyFunc(ExecuteTerraform, func(i schema.ConfigAndStacksInfo) error {
			called = true
			// check fields set
			assert.Equal(t, component, i.Component)
			assert.Equal(t, stack, i.Stack)
			return nil
		})
		defer patch.Reset()

		info := schema.ConfigAndStacksInfo{SubCommand: "plan"}
		err := processTerraformComponent(&atmosConfig, &info, stack, component, section, logFunc)
		assert.NoError(t, err)
		assert.True(t, called)
	})
}

func TestParseTFCliArgsVars(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		expected map[string]string
	}{
		{
			name:     "empty environment variable",
			envValue: "",
			expected: map[string]string{},
		},
		{
			name:     "single -var argument",
			envValue: "-var environment=prod",
			expected: map[string]string{
				"environment": "prod",
			},
		},
		{
			name:     "multiple -var arguments",
			envValue: "-var environment=prod -var region=us-east-1 -var instance_count=3",
			expected: map[string]string{
				"environment":    "prod",
				"region":         "us-east-1",
				"instance_count": "3",
			},
		},
		{
			name:     "mixed with other arguments",
			envValue: "-auto-approve -var environment=staging -input=false -var tag=latest",
			expected: map[string]string{
				"environment": "staging",
				"tag":         "latest",
			},
		},
		{
			name:     "quoted values",
			envValue: `-var "environment=production with spaces" -var 'region=us-west-2'`,
			expected: map[string]string{
				"environment": "production with spaces",
				"region":      "us-west-2",
			},
		},
		{
			name:     "var with equals format",
			envValue: "-var=environment=dev -var=region=eu-west-1",
			expected: map[string]string{
				"environment": "dev",
				"region":      "eu-west-1",
			},
		},
		{
			name:     "complex values with equals signs",
			envValue: `-var database_url="postgres://user:pass@host:5432/db" -var connection_string="server=host;database=db"`,
			expected: map[string]string{
				"database_url":      "postgres://user:pass@host:5432/db",
				"connection_string": "server=host;database=db",
			},
		},
		{
			name:     "JSON-like values",
			envValue: `-var 'tags={"Environment":"prod","Team":"devops"}' -var list='["item1","item2"]'`,
			expected: map[string]string{
				"tags": `{"Environment":"prod","Team":"devops"}`,
				"list": `["item1","item2"]`,
			},
		},
		{
			name:     "empty value",
			envValue: "-var empty_var= -var normal_var=value",
			expected: map[string]string{
				"empty_var":  "",
				"normal_var": "value",
			},
		},
		{
			name:     "special characters in values",
			envValue: `-var path="/tmp/test file" -var command="echo 'hello world'"`,
			expected: map[string]string{
				"path":    "/tmp/test file",
				"command": "echo 'hello world'",
			},
		},
		{
			name:     "malformed var arguments are ignored",
			envValue: "-var -var malformed -var good=value",
			expected: map[string]string{
				"good": "value",
			},
		},
		{
			name:     "var without value is ignored",
			envValue: "-var key_without_equals -var good=value",
			expected: map[string]string{
				"good": "value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Store original value to restore later.
			originalValue := os.Getenv("TF_CLI_ARGS")
			defer func() {
				if originalValue != "" {
					os.Setenv("TF_CLI_ARGS", originalValue)
				} else {
					os.Unsetenv("TF_CLI_ARGS")
				}
			}()

			// Set test environment variable
			os.Setenv("TF_CLI_ARGS", tt.envValue)

			// Test the function
			result := ParseTFCliArgsVars()

			// Assert results
			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("ParseTFCliArgsVars() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestParseTFCliArgsVars_NoEnvironmentVariable(t *testing.T) {
	// Ensure TF_CLI_ARGS is not set.
	originalValue := os.Getenv("TF_CLI_ARGS")
	os.Unsetenv("TF_CLI_ARGS")
	defer func() {
		if originalValue != "" {
			os.Setenv("TF_CLI_ARGS", originalValue)
		}
	}()

	result := ParseTFCliArgsVars()

	// Should return empty map when environment variable is not set.
	expected := map[string]string{}
	if !reflect.DeepEqual(result, expected) {
		t.Errorf("ParseTFCliArgsVars() with no env var = %v, expected %v", result, expected)
	}
}

func TestParseCommandArgs(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
		{
			name:     "single argument",
			input:    "plan",
			expected: []string{"plan"},
		},
		{
			name:     "multiple arguments",
			input:    "terraform plan -auto-approve",
			expected: []string{"terraform", "plan", "-auto-approve"},
		},
		{
			name:     "quoted arguments",
			input:    `terraform plan -var "environment=production"`,
			expected: []string{"terraform", "plan", "-var", "environment=production"},
		},
		{
			name:     "single quoted arguments",
			input:    `terraform plan -var 'region=us-west-2'`,
			expected: []string{"terraform", "plan", "-var", "region=us-west-2"},
		},
		{
			name:     "mixed quotes",
			input:    `terraform plan -var "env=prod" -var 'region=us-east-1'`,
			expected: []string{"terraform", "plan", "-var", "env=prod", "-var", "region=us-east-1"},
		},
		{
			name:     "quotes within quotes",
			input:    `terraform plan -var 'command=echo "hello world"'`,
			expected: []string{"terraform", "plan", "-var", `command=echo "hello world"`},
		},
		{
			name:     "extra spaces",
			input:    "  terraform   plan   -auto-approve  ",
			expected: []string{"terraform", "plan", "-auto-approve"},
		},
		{
			name:     "complex example",
			input:    `-auto-approve -var "database_url=postgres://user:pass@host:5432/db" -var environment=prod`,
			expected: []string{"-auto-approve", "-var", "database_url=postgres://user:pass@host:5432/db", "-var", "environment=prod"},
		},
		{
			name:     "empty quotes",
			input:    `terraform plan -var "empty=" -var normal=value`,
			expected: []string{"terraform", "plan", "-var", "empty=", "-var", "normal=value"},
		},
		{
			name:     "unclosed quotes handled gracefully",
			input:    `terraform plan -var "unclosed`,
			expected: []string{"terraform", "plan", "-var", "unclosed"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseCommandArgs(tt.input)

			if !reflect.DeepEqual(result, tt.expected) {
				t.Errorf("parseCommandArgs(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

// Benchmark test to ensure the function performs well with large inputs.
func BenchmarkParseTFCliArgsVars(b *testing.B) {
	// Create a large TF_CLI_ARGS string for benchmarking
	largeTFCliArgs := ""
	for i := 0; i < 100; i++ {
		largeTFCliArgs += fmt.Sprintf(" -var key%d=value%d", i, i)
	}

	originalValue := os.Getenv("TF_CLI_ARGS")
	os.Setenv("TF_CLI_ARGS", largeTFCliArgs)
	defer func() {
		if originalValue != "" {
			os.Setenv("TF_CLI_ARGS", originalValue)
		} else {
			os.Unsetenv("TF_CLI_ARGS")
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseTFCliArgsVars()
	}
}
