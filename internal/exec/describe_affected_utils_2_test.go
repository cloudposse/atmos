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
	tempDir := t.TempDir()

	// Create test component directories
	terraformComponentPath := filepath.Join(tempDir, "terraform", "test-component")
	helmfileComponentPath := filepath.Join(tempDir, "helmfile", "test-component")
	packerComponentPath := filepath.Join(tempDir, "packer", "test-component")

	err := os.MkdirAll(terraformComponentPath, 0o755)
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

// Tests for getFileFolderDependencies helper function.

func TestGetFileFolderDependencies(t *testing.T) {
	t.Run("extracts file dependencies from dependencies.components", func(t *testing.T) {
		componentSection := map[string]any{
			"dependencies": map[string]any{
				"components": []any{
					map[string]any{"component": "vpc"},
					map[string]any{"kind": "file", "path": "configs/app.json"},
					map[string]any{"kind": "folder", "path": "src/lambda"},
				},
			},
		}
		settingsSection := map[string]any{}

		deps := getFileFolderDependencies(componentSection, settingsSection)

		// Should only have file and folder entries, not component entries.
		assert.Len(t, deps, 2)
		// Check that the file and folder are present.
		hasFile := false
		hasFolder := false
		for _, dep := range deps {
			if dep.Kind == "file" && dep.Path == "configs/app.json" {
				hasFile = true
			}
			if dep.Kind == "folder" && dep.Path == "src/lambda" {
				hasFolder = true
			}
		}
		assert.True(t, hasFile, "should contain file dependency")
		assert.True(t, hasFolder, "should contain folder dependency")
	})

	t.Run("falls back to settings.depends_on for file/folder deps", func(t *testing.T) {
		componentSection := map[string]any{
			"vars": map[string]any{
				"name": "test",
			},
		}
		settingsSection := map[string]any{
			"depends_on": map[any]any{
				1: map[string]any{"component": "vpc"},
				2: map[string]any{"file": "external/config.yaml"},
				3: map[string]any{"folder": "shared/modules"},
			},
		}

		deps := getFileFolderDependencies(componentSection, settingsSection)

		// Should only have file and folder entries from settings.depends_on.
		// These are converted to kind/path format.
		assert.Len(t, deps, 2)
		hasFile := false
		hasFolder := false
		for _, dep := range deps {
			if dep.Kind == "file" && dep.Path == "external/config.yaml" {
				hasFile = true
			}
			if dep.Kind == "folder" && dep.Path == "shared/modules" {
				hasFolder = true
			}
		}
		assert.True(t, hasFile, "should contain file dependency")
		assert.True(t, hasFolder, "should contain folder dependency")
	})

	t.Run("returns nil when no file/folder dependencies", func(t *testing.T) {
		componentSection := map[string]any{
			"dependencies": map[string]any{
				"components": []any{
					map[string]any{"component": "vpc"},
					map[string]any{"component": "rds"},
				},
			},
		}
		settingsSection := map[string]any{}

		deps := getFileFolderDependencies(componentSection, settingsSection)

		assert.Nil(t, deps)
	})

	t.Run("returns nil when no dependencies defined", func(t *testing.T) {
		componentSection := map[string]any{}
		settingsSection := map[string]any{}

		deps := getFileFolderDependencies(componentSection, settingsSection)

		assert.Nil(t, deps)
	})

	t.Run("prefers dependencies.components over settings.depends_on", func(t *testing.T) {
		componentSection := map[string]any{
			"dependencies": map[string]any{
				"components": []any{
					map[string]any{"kind": "file", "path": "new-config.json"},
				},
			},
		}
		settingsSection := map[string]any{
			"depends_on": map[any]any{
				1: map[string]any{"file": "old-config.json"},
			},
		}

		deps := getFileFolderDependencies(componentSection, settingsSection)

		assert.Len(t, deps, 1)
		// Should use the file from dependencies.components, not settings.depends_on.
		assert.Equal(t, "file", deps[0].Kind)
		assert.Equal(t, "new-config.json", deps[0].Path)
	})

	t.Run("handles mixed component and file/folder dependencies", func(t *testing.T) {
		componentSection := map[string]any{
			"dependencies": map[string]any{
				"components": []any{
					map[string]any{"component": "vpc"},
					map[string]any{"component": "rds", "stack": "prod"},
					map[string]any{"kind": "file", "path": "config.json"},
					map[string]any{"kind": "folder", "path": "modules/"},
				},
			},
		}
		settingsSection := map[string]any{}

		deps := getFileFolderDependencies(componentSection, settingsSection)

		// Should only return file/folder deps, filtering out component deps.
		assert.Len(t, deps, 2)
		for _, dep := range deps {
			// Should only contain file or folder deps.
			assert.True(t, dep.IsFileDependency() || dep.IsFolderDependency(), "should only contain file or folder deps")
		}
	})

	// v2 surface coverage: dependencies.files / dependencies.folders sibling keys.

	t.Run("v2: extracts file deps from dependencies.files sibling key", func(t *testing.T) {
		componentSection := map[string]any{
			"dependencies": map[string]any{
				"files": []any{"configs/app.json", "configs/db.json"},
			},
		}
		deps := getFileFolderDependencies(componentSection, nil)

		assert.Len(t, deps, 2)
		paths := make(map[string]bool, len(deps))
		for _, dep := range deps {
			assert.True(t, dep.IsFileDependency(), "every entry must be a file dep")
			paths[dep.Path] = true
		}
		assert.True(t, paths["configs/app.json"])
		assert.True(t, paths["configs/db.json"])
	})

	t.Run("v2: extracts folder deps from dependencies.folders sibling key", func(t *testing.T) {
		componentSection := map[string]any{
			"dependencies": map[string]any{
				"folders": []any{"src/lambda/handler"},
			},
		}
		deps := getFileFolderDependencies(componentSection, nil)

		assert.Len(t, deps, 1)
		assert.True(t, deps[0].IsFolderDependency())
		assert.Equal(t, "src/lambda/handler", deps[0].Path)
	})

	t.Run("v2: name alias on a component entry parses correctly", func(t *testing.T) {
		// Components with `name:` should not be returned by getFileFolderDependencies
		// (they aren't file/folder deps), but the section must parse without error
		// and the file sibling alongside should still be picked up.
		componentSection := map[string]any{
			"dependencies": map[string]any{
				"components": []any{
					map[string]any{"name": "vpc", "stack": "prod"},
				},
				"files": []any{"configs/app.json"},
			},
		}
		deps := getFileFolderDependencies(componentSection, nil)

		assert.Len(t, deps, 1)
		assert.True(t, deps[0].IsFileDependency())
		assert.Equal(t, "configs/app.json", deps[0].Path)
	})

	t.Run("v1 inline and v2 sibling keys produce equivalent file/folder deps", func(t *testing.T) {
		v1 := map[string]any{
			"dependencies": map[string]any{
				"components": []any{
					map[string]any{"component": "vpc"},
					map[string]any{"kind": "file", "path": "configs/app.json"},
					map[string]any{"kind": "folder", "path": "src/handler"},
				},
			},
		}
		v2 := map[string]any{
			"dependencies": map[string]any{
				"components": []any{map[string]any{"name": "vpc"}},
				"files":      []any{"configs/app.json"},
				"folders":    []any{"src/handler"},
			},
		}

		v1Deps := getFileFolderDependencies(v1, nil)
		v2Deps := getFileFolderDependencies(v2, nil)

		// Both surfaces should return the same set of file/folder deps.
		assert.Len(t, v1Deps, 2)
		assert.Len(t, v2Deps, 2)
		assert.ElementsMatch(t, v1Deps, v2Deps,
			"v1 inline and v2 sibling keys must produce identical file/folder deps")
	})

	t.Run("v2: combining inline kind:file and sibling files dedupes by path", func(t *testing.T) {
		componentSection := map[string]any{
			"dependencies": map[string]any{
				"components": []any{
					map[string]any{"kind": "file", "path": "configs/shared.json"},
				},
				"files": []any{"configs/shared.json"},
			},
		}
		deps := getFileFolderDependencies(componentSection, nil)

		// Normalize dedupes the (kind, path) pair, so the same path declared
		// inline and via the sibling key collapses to a single entry.
		require.Len(t, deps, 1, "duplicate (kind,path) pair should be deduped to one entry")
		assert.Equal(t, "file", deps[0].Kind)
		assert.Equal(t, "configs/shared.json", deps[0].Path)
	})
}

func TestIsComponentDependentFolderOrFileChangedIndexed_AdditionalCases(t *testing.T) {
	// Create temp files to act as changed files.
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.json")
	require.NoError(t, os.WriteFile(configFile, []byte("{}"), 0o644))

	lambdaDir := filepath.Join(tmpDir, "src", "lambda")
	require.NoError(t, os.MkdirAll(lambdaDir, 0o755))
	lambdaFile := filepath.Join(lambdaDir, "handler.py")
	require.NoError(t, os.WriteFile(lambdaFile, []byte("def handler(): pass"), 0o644))

	otherFile := filepath.Join(tmpDir, "unrelated.txt")
	require.NoError(t, os.WriteFile(otherFile, []byte("hello"), 0o644))

	tests := []struct {
		name              string
		changedFiles      []string
		deps              []schema.ComponentDependency
		expectChanged     bool
		expectChangedType string
		expectChangedPath string
		expectError       bool
	}{
		{
			name:          "no deps returns unchanged",
			changedFiles:  []string{configFile},
			deps:          []schema.ComponentDependency{},
			expectChanged: false,
		},
		{
			name:         "file dependency matches changed file",
			changedFiles: []string{configFile},
			deps: []schema.ComponentDependency{
				{Kind: "file", Path: configFile},
			},
			expectChanged:     true,
			expectChangedType: "file",
			expectChangedPath: configFile,
		},
		{
			name:         "file dependency does not match",
			changedFiles: []string{otherFile},
			deps: []schema.ComponentDependency{
				{Kind: "file", Path: configFile},
			},
			expectChanged: false,
		},
		{
			name:         "folder dependency matches file in folder",
			changedFiles: []string{lambdaFile},
			deps: []schema.ComponentDependency{
				{Kind: "folder", Path: filepath.Join(tmpDir, "src", "lambda")},
			},
			expectChanged:     true,
			expectChangedType: "folder",
			expectChangedPath: filepath.Join(tmpDir, "src", "lambda"),
		},
		{
			name:         "folder dependency does not match file outside folder",
			changedFiles: []string{otherFile},
			deps: []schema.ComponentDependency{
				{Kind: "folder", Path: filepath.Join(tmpDir, "src", "lambda")},
			},
			expectChanged: false,
		},
		{
			name:         "skips component dependencies",
			changedFiles: []string{configFile},
			deps: []schema.ComponentDependency{
				{Kind: "terraform", Component: "vpc"},
				{Kind: "", Component: "rds"},
			},
			expectChanged: false,
		},
		{
			name:         "mixed deps only checks file and folder",
			changedFiles: []string{configFile},
			deps: []schema.ComponentDependency{
				{Kind: "terraform", Component: "vpc"},
				{Kind: "file", Path: configFile},
				{Kind: "folder", Path: filepath.Join(tmpDir, "other")},
			},
			expectChanged:     true,
			expectChangedType: "file",
			expectChangedPath: configFile,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build a minimal changedFilesIndex.
			idx := &changedFilesIndex{
				allFiles: tt.changedFiles,
			}

			changed, changedType, changedPath, err := isComponentDependentFolderOrFileChangedIndexed(idx, tt.deps)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectChanged, changed)
			if tt.expectChanged {
				assert.Equal(t, tt.expectChangedType, changedType)
				assert.Equal(t, tt.expectChangedPath, changedPath)
			}
			// When no file/folder deps exist, metadata should be empty.
			if len(tt.deps) == 0 {
				assert.Empty(t, changedType, "changedType should be empty when no deps")
				assert.Empty(t, changedPath, "changedPath should be empty when no deps")
			}
		})
	}
}

func TestMatchNewFormatStack(t *testing.T) {
	tests := []struct {
		name      string
		dep       schema.ComponentDependency
		argsStack string
		stackName string
		expected  bool
	}{
		{
			name:      "explicit stack matches argsStack",
			dep:       schema.ComponentDependency{Stack: "prod-ue1"},
			argsStack: "prod-ue1",
			stackName: "dev-ue1",
			expected:  true,
		},
		{
			name:      "explicit stack does not match argsStack",
			dep:       schema.ComponentDependency{Stack: "prod-ue1"},
			argsStack: "dev-ue1",
			stackName: "dev-ue1",
			expected:  false,
		},
		{
			name:      "empty stack defaults to same stack match",
			dep:       schema.ComponentDependency{},
			argsStack: "dev-ue1",
			stackName: "dev-ue1",
			expected:  true,
		},
		{
			name:      "empty stack defaults to same stack no match",
			dep:       schema.ComponentDependency{},
			argsStack: "prod-ue1",
			stackName: "dev-ue1",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchNewFormatStack(&tt.dep, tt.argsStack, tt.stackName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatchLegacyStack(t *testing.T) {
	tests := []struct {
		name      string
		dep       schema.ComponentDependency
		argsStack string
		stackName string
		expected  bool
	}{
		{
			name:      "explicit stack matches",
			dep:       schema.ComponentDependency{Stack: "prod-ue1"},
			argsStack: "prod-ue1",
			stackName: "dev-ue1",
			expected:  true,
		},
		{
			name:      "explicit stack does not match",
			dep:       schema.ComponentDependency{Stack: "prod-ue1"},
			argsStack: "dev-ue1",
			stackName: "dev-ue1",
			expected:  false,
		},
		{
			name:      "no context fields requires same stack match",
			dep:       schema.ComponentDependency{},
			argsStack: "dev-ue1",
			stackName: "dev-ue1",
			expected:  true,
		},
		{
			name:      "no context fields requires same stack no match",
			dep:       schema.ComponentDependency{},
			argsStack: "prod-ue1",
			stackName: "dev-ue1",
			expected:  false,
		},
		{
			name:      "with context fields returns true regardless of stack",
			dep:       schema.ComponentDependency{Environment: "ue1"},
			argsStack: "prod-ue1",
			stackName: "dev-ue1",
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchLegacyStack(&tt.dep, tt.argsStack, tt.stackName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatchLegacyContextFields(t *testing.T) {
	tests := []struct {
		name     string
		dep      schema.ComponentDependency
		provided *schema.Context
		stack    *schema.Context
		expected bool
	}{
		{
			name: "all fields match when specified",
			dep: schema.ComponentDependency{
				Namespace:   "acme",
				Tenant:      "tenant1",
				Environment: "ue1",
				Stage:       "prod",
			},
			provided: &schema.Context{Namespace: "acme", Tenant: "tenant1", Environment: "ue1", Stage: "prod"},
			stack:    &schema.Context{},
			expected: true,
		},
		{
			name: "namespace mismatch",
			dep: schema.ComponentDependency{
				Namespace: "other",
			},
			provided: &schema.Context{Namespace: "acme"},
			stack:    &schema.Context{Namespace: "acme"},
			expected: false,
		},
		{
			name: "no fields specified falls through to stack comparison",
			dep:  schema.ComponentDependency{},
			provided: &schema.Context{
				Namespace: "acme", Tenant: "tenant1", Environment: "ue1", Stage: "prod",
			},
			stack: &schema.Context{
				Namespace: "acme", Tenant: "tenant1", Environment: "ue1", Stage: "prod",
			},
			expected: true,
		},
		{
			name: "unspecified fields fall back to stack context mismatch",
			dep:  schema.ComponentDependency{},
			provided: &schema.Context{
				Namespace: "acme",
			},
			stack: &schema.Context{
				Namespace: "other",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchLegacyContextFields(&tt.dep, tt.provided, tt.stack)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestContextToComponentDependency(t *testing.T) {
	tests := []struct {
		name    string
		context schema.Context
	}{
		{
			name: "converts all fields",
			context: schema.Context{
				Component:   "vpc",
				Stack:       "tenant1-ue1-prod",
				Namespace:   "acme",
				Tenant:      "tenant1",
				Environment: "ue1",
				Stage:       "prod",
			},
		},
		{
			name: "converts with empty fields",
			context: schema.Context{
				Component: "rds",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dep := contextToComponentDependency(&tt.context)

			assert.Equal(t, tt.context.Component, dep.Component)
			assert.Equal(t, tt.context.Stack, dep.Stack)
			assert.Equal(t, tt.context.Namespace, dep.Namespace)
			assert.Equal(t, tt.context.Tenant, dep.Tenant)
			assert.Equal(t, tt.context.Environment, dep.Environment)
			assert.Equal(t, tt.context.Stage, dep.Stage)
		})
	}
}
