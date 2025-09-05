package exec

import (
	"os"
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
