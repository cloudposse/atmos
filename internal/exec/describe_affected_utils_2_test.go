package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestAddAffectedSpaceliftAdminStack(t *testing.T) {
	// Prepare test data
	atmosConfig := &schema.AtmosConfiguration{}
	stackName := "test-stack"
	componentName := "test-component"
	affectedList := []schema.Affected{
		{
			Component:     componentName,
			Stack:         stackName,
			ComponentType: "spacelift",
			Affected:      "foo",
		},
	}
	spaceliftAdminStack := "spacelift-admin-stack"
	spaceliftAdminStacks := map[string]any{
		spaceliftAdminStack: map[string]any{
			"spacelift": map[string]any{
				"admin": true,
			},
		},
	}
	settingsSection := map[string]any{}
	configAndStacksInfo := &schema.ConfigAndStacksInfo{}

	// Call the function under test
	affectedListResult, err := addAffectedSpaceliftAdminStack(
		atmosConfig,
		&affectedList,
		&settingsSection,
		&spaceliftAdminStacks,
		stackName,
		componentName,
		configAndStacksInfo,
		false,
	)

	assert.NoError(t, err)

	// Check that the spacelift admin stack was added to the affected list
	found := false
	for _, affected := range *affectedListResult {
		if affected.Component == componentName && affected.ComponentType == "spacelift" {
			found = true
			break
		}
	}
	assert.True(t, found, "Spacelift admin stack should be added to affected list")
}

func TestAddAffectedSpaceliftAdminStack_NoAdminStack(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	stackName := "test-stack"
	componentName := "test-component"
	affectedList := []schema.Affected{}
	spaceliftAdminStacks := map[string]any{}
	settingsSection := map[string]any{}
	configAndStacksInfo := &schema.ConfigAndStacksInfo{}

	// Should not panic or add anything
	_, err := addAffectedSpaceliftAdminStack(
		atmosConfig,
		&affectedList,
		&settingsSection,
		&spaceliftAdminStacks,
		stackName,
		componentName,
		configAndStacksInfo,
		false,
	)

	assert.NoError(t, err)
	assert.Equal(t, 0, len(affectedList), "Affected list should remain empty if no admin stack")
}

func TestAddAffectedSpaceliftAdminStack_DuplicateNotAdded(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	stackName := "test-stack"
	componentName := "test-component"
	spaceliftAdminStack := "spacelift-admin-stack"
	affectedList := []schema.Affected{
		{
			Component:     spaceliftAdminStack,
			Stack:         stackName,
			ComponentType: "spacelift",
			Affected:      "foo",
		},
	}
	spaceliftAdminStacks := map[string]any{
		spaceliftAdminStack: map[string]any{
			"spacelift": map[string]any{
				"admin": true,
			},
		},
	}
	settingsSection := map[string]any{}
	configAndStacksInfo := &schema.ConfigAndStacksInfo{}

	// Call the function under test
	_, err := addAffectedSpaceliftAdminStack(
		atmosConfig,
		&affectedList,
		&settingsSection,
		&spaceliftAdminStacks,
		stackName,
		componentName,
		configAndStacksInfo,
		false,
	)

	assert.NoError(t, err)

	// Should not add a duplicate
	count := 0
	for _, affected := range affectedList {
		if affected.Component == spaceliftAdminStack && affected.ComponentType == "spacelift" {
			count++
		}
	}
	assert.Equal(t, 1, count, "Spacelift admin stack should not be added twice")
}

func TestAddAffectedSpaceliftAdminStack_WithValidConfig(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Stacks: schema.Stacks{
			NamePattern: "{environment}-{stage}",
		},
	}

	stackName := "tenant1-ue2-dev"
	componentName := "test-component"
	adminStackName := "tenant1-ue2-spacelift-admin"
	adminComponentName := "spacelift-admin"

	// Existing affected list with one item
	affectedList := []schema.Affected{{
		Component:     componentName,
		Stack:         stackName,
		ComponentType: "terraform",
		Affected:      "test",
	}}

	// Settings section with Spacelift admin stack configuration
	settingsSection := map[string]any{
		"spacelift": map[string]any{
			"admin_stack_selector": map[string]string{
				"component":   adminComponentName,
				"environment": "ue2",
				"stage":       "dev",
			},
		},
	}

	// Mock stacks data with the admin stack
	stacks := map[string]any{
		adminStackName: map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					adminComponentName: map[string]any{
						"vars": map[string]any{
							"environment": "ue2",
							"stage":       "dev",
							"component":   adminComponentName,
						},
						"settings": map[string]any{
							"spacelift": map[string]any{
								"workspace_enabled": true,
							},
						},
					},
				},
			},
		},
	}

	configAndStacksInfo := &schema.ConfigAndStacksInfo{
		ComponentSection: map[string]any{
			"environment": "ue2",
			"stage":       "dev",
			"component":   componentName,
		},
	}

	// Call the function under test
	result, err := addAffectedSpaceliftAdminStack(
		atmosConfig,
		&affectedList,
		&settingsSection,
		&stacks,
		stackName,
		componentName,
		configAndStacksInfo,
		true, // includeSettings
	)

	// Verify results
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should have 2 items in the result (original + admin stack)
	assert.Equal(t, 2, len(*result))

	// Verify the admin stack was added
	found := false
	for _, affected := range *result {
		if affected.Component == adminComponentName &&
			affected.Stack == adminStackName &&
			affected.ComponentType == "terraform" {
			found = true
			// Verify the affected reason is set correctly
			assert.Equal(t, "stack.settings.spacelift.admin_stack_selector", affected.Affected)
		}
	}
	assert.True(t, found, "Spacelift admin stack should be added to affected list")
}

func TestIsComponentFolderChanged(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "atmos-test-*")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tempDir) })

	// Create test component directories
	terraformComponentPath := filepath.Join(tempDir, "terraform", "test-component")
	helmfileComponentPath := filepath.Join(tempDir, "helmfile", "test-component")
	packerComponentPath := filepath.Join(tempDir, "packer", "test-component")

	err = os.MkdirAll(terraformComponentPath, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(helmfileComponentPath, 0o755)
	require.NoError(t, err)
	err = os.MkdirAll(packerComponentPath, 0o755)
	require.NoError(t, err)

	// Create some test files in the component directories
	createTestFile := func(path string) {
		err = os.WriteFile(path, []byte("test"), 0o644)
		require.NoError(t, err)
	}

	createTestFile(filepath.Join(terraformComponentPath, "main.tf"))
	createTestFile(filepath.Join(helmfileComponentPath, "helmfile.yaml"))
	createTestFile(filepath.Join(packerComponentPath, "packer.json"))

	// Create a subdirectory with a file
	subDir := filepath.Join(terraformComponentPath, "modules")
	err = os.MkdirAll(subDir, 0o755)
	require.NoError(t, err)
	createTestFile(filepath.Join(subDir, "module.tf"))

	// Setup test config
	atmosConfig := &schema.AtmosConfiguration{
		BasePath:   tempDir,
		Components: schema.Components{},
	}

	tests := []struct {
		name           string
		component      string
		componentType  string
		changedFiles   []string
		expectedResult bool
		expectedError  bool
		errorMessage   string
	}{
		{
			name:           "no changes in component folder",
			component:      "test-component",
			componentType:  "terraform",
			changedFiles:   []string{"/some/other/path/file.txt"},
			expectedResult: false,
			expectedError:  false,
		},
		{
			name:           "empty changed files",
			component:      "test-component",
			componentType:  "terraform",
			changedFiles:   []string{},
			expectedResult: false,
			expectedError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := isComponentFolderChanged(tt.component, tt.componentType, atmosConfig, tt.changedFiles)

			if tt.expectedError {
				assert.Error(t, err)
				if tt.errorMessage != "" {
					assert.Contains(t, err.Error(), tt.errorMessage)
				}
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, result)
			}
		})
	}
}

func TestIsComponentDependentFolderOrFileChanged_HandlesEmptyFileAndFolder(t *testing.T) {
	// Test case where depends_on has multiple entries:
	// 1st entry has a file field
	// 2nd entry has only component field (no file/folder) - this causes the bug
	deps := schema.DependsOn{
		"1": schema.Context{
			File: "components/terraform/mixins/test.tf",
		},
		"2": schema.Context{
			Component: "some-component",
			// File and Folder are empty strings - this is the problematic case
		},
	}

	changedFiles := []string{"components/terraform/mixins/test.tf"}

	// This should not panic or return an error about "lstat : no such file or directory"
	isChanged, changeType, changedItem, err := isComponentDependentFolderOrFileChanged(changedFiles, deps)

	assert.NoError(t, err, "Should handle entries without file/folder gracefully")
	assert.True(t, isChanged, "Should detect the change in first entry")
	assert.Equal(t, "file", changeType)
	assert.Equal(t, "components/terraform/mixins/test.tf", changedItem)
}

func TestIsComponentDependentFolderOrFileChanged_MixedDependencies(t *testing.T) {
	// Test with folder in first entry, empty in second, file in third
	deps := schema.DependsOn{
		"1": schema.Context{
			Folder: "components/terraform/vpc",
		},
		"2": schema.Context{
			Component: "only-component",
			// No file or folder
		},
		"3": schema.Context{
			File: "components/terraform/mixins/common.tf",
		},
	}

	testCases := []struct {
		name               string
		changedFiles       []string
		expectedIsChanged  bool
		expectedChangeType string
		expectedItem       string
	}{
		{
			name:               "folder changed",
			changedFiles:       []string{"components/terraform/vpc/main.tf"},
			expectedIsChanged:  true,
			expectedChangeType: "folder",
			expectedItem:       "components/terraform/vpc",
		},
		{
			name:               "file changed",
			changedFiles:       []string{"components/terraform/mixins/common.tf"},
			expectedIsChanged:  true,
			expectedChangeType: "file",
			expectedItem:       "components/terraform/mixins/common.tf",
		},
		{
			name:               "no relevant changes",
			changedFiles:       []string{"unrelated/file.tf"},
			expectedIsChanged:  false,
			expectedChangeType: "",
			expectedItem:       "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			isChanged, changeType, changedItem, err := isComponentDependentFolderOrFileChanged(tc.changedFiles, deps)

			assert.NoError(t, err, "Should not return error for mixed dependencies")
			assert.Equal(t, tc.expectedIsChanged, isChanged)
			assert.Equal(t, tc.expectedChangeType, changeType)
			assert.Equal(t, tc.expectedItem, changedItem)
		})
	}
}

func TestIsComponentDependentFolderOrFileChanged_OnlyComponentDependencies(t *testing.T) {
	// Test where all entries only have component field (no file/folder)
	deps := schema.DependsOn{
		"1": schema.Context{
			Component: "vpc",
		},
		"2": schema.Context{
			Component: "vpc-flow-logs-bucket",
		},
	}

	changedFiles := []string{"some/random/file.tf"}

	// Should handle gracefully and return false (no file/folder dependencies to check)
	isChanged, changeType, changedItem, err := isComponentDependentFolderOrFileChanged(changedFiles, deps)

	assert.NoError(t, err, "Should not error when no file/folder dependencies exist")
	assert.False(t, isChanged, "Should return false when no file/folder dependencies")
	assert.Equal(t, "", changeType)
	assert.Equal(t, "", changedItem)
}

func TestIsComponentDependentFolderOrFileChanged_EmptyDependsOn(t *testing.T) {
	// Test with empty DependsOn map
	deps := schema.DependsOn{}

	changedFiles := []string{"some/file.tf"}

	isChanged, changeType, changedItem, err := isComponentDependentFolderOrFileChanged(changedFiles, deps)

	assert.NoError(t, err, "Should handle empty DependsOn")
	assert.False(t, isChanged)
	assert.Equal(t, "", changeType)
	assert.Equal(t, "", changedItem)
}

func TestAppendToAffected(t *testing.T) {
	t.Run("should add new affected component", func(t *testing.T) {
		// Setup
		atmosConfig := &schema.AtmosConfiguration{}
		componentName := "test-component"
		stackName := "test-stack"
		affectedList := []schema.Affected{}
		affected := &schema.Affected{
			Component:     componentName,
			Stack:         stackName,
			ComponentType: "terraform",
			Affected:      "test-change",
		}

		componentSection := map[string]any{
			"settings": map[string]any{},
		}

		// Execute
		err := appendToAffected(
			atmosConfig,
			componentName,
			stackName,
			&componentSection,
			&affectedList,
			affected,
			false,
			&map[string]any{},
			true,
		)

		// Verify
		require.NoError(t, err)
		assert.Len(t, affectedList, 1)
		assert.Equal(t, componentName, affectedList[0].Component)
		assert.Equal(t, stackName, affectedList[0].Stack)
		assert.Equal(t, "test-change", affectedList[0].Affected)
		assert.Len(t, affectedList[0].AffectedAll, 1)
	})

	t.Run("should update existing component with new affected reason", func(t *testing.T) {
		// Setup
		atmosConfig := &schema.AtmosConfiguration{}
		componentName := "test-component"
		stackName := "test-stack"
		affectedList := []schema.Affected{
			{
				Component:     componentName,
				Stack:         stackName,
				ComponentType: "terraform",
				Affected:      "initial-change",
				AffectedAll:   []string{"initial-change"},
			},
		}

		affected := &schema.Affected{
			Component:     componentName,
			Stack:         stackName,
			ComponentType: "terraform",
			Affected:      "another-change",
		}

		componentSection := map[string]any{
			"settings": map[string]any{},
		}

		// Execute
		err := appendToAffected(
			atmosConfig,
			componentName,
			stackName,
			&componentSection,
			&affectedList,
			affected,
			false,
			&map[string]any{},
			true,
		)

		// Verify
		require.NoError(t, err)
		assert.Len(t, affectedList, 1)
		assert.Equal(t, componentName, affectedList[0].Component)
		assert.Len(t, affectedList[0].AffectedAll, 2)
		assert.Contains(t, affectedList[0].AffectedAll, "initial-change")
		assert.Contains(t, affectedList[0].AffectedAll, "another-change")
	})

	t.Run("should include settings when requested", func(t *testing.T) {
		// Setup
		atmosConfig := &schema.AtmosConfiguration{}
		componentName := "test-component"
		stackName := "test-stack"
		affectedList := []schema.Affected{}
		affected := &schema.Affected{
			Component:     componentName,
			Stack:         stackName,
			ComponentType: "terraform",
			Affected:      "test-change",
		}

		settings := map[string]any{
			"setting1": "value1",
			"setting2": 42,
		}

		componentSection := map[string]any{
			"settings": settings,
		}

		// Execute with includeSettings = true
		err := appendToAffected(
			atmosConfig,
			componentName,
			stackName,
			&componentSection,
			&affectedList,
			affected,
			false,
			&map[string]any{},
			true,
		)

		// Verify
		require.NoError(t, err)
		assert.Len(t, affectedList, 1)
		assert.NotNil(t, affectedList[0].Settings)
		assert.Equal(t, "value1", affectedList[0].Settings["setting1"])
	})

	t.Run("should not include settings when not requested", func(t *testing.T) {
		// Setup
		atmosConfig := &schema.AtmosConfiguration{}
		componentName := "test-component"
		stackName := "test-stack"
		affectedList := []schema.Affected{}
		affected := &schema.Affected{
			Component:     componentName,
			Stack:         stackName,
			ComponentType: "terraform",
			Affected:      "test-change",
		}

		componentSection := map[string]any{
			"settings": map[string]any{
				"setting1": "value1",
			},
		}

		// Execute with includeSettings = false
		err := appendToAffected(
			atmosConfig,
			componentName,
			stackName,
			&componentSection,
			&affectedList,
			affected,
			false,
			&map[string]any{},
			false,
		)

		// Verify
		require.NoError(t, err)
		assert.Len(t, affectedList, 1)
		assert.Nil(t, affectedList[0].Settings)
	})
}
