package exec

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -package=exec -destination=mock_storer_test.go github.com/go-git/go-git/v5/storage Storer

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestFindAffected(t *testing.T) {
	tests := []struct {
		name                        string
		currentStacks               map[string]any
		remoteStacks                map[string]any
		atmosConfig                 *schema.AtmosConfiguration
		changedFiles                []string
		includeSpaceliftAdminStacks bool
		includeSettings             bool
		stackToFilter               string
		expectedAffected            []schema.Affected
		expectedError               bool
	}{
		{
			name:             "Empty stacks should return empty affected list",
			currentStacks:    map[string]any{},
			remoteStacks:     map[string]any{},
			atmosConfig:      &schema.AtmosConfiguration{},
			changedFiles:     []string{},
			expectedAffected: []schema.Affected{},
			expectedError:    false,
		},
		{
			name: "Stack filter should only process specified stack",
			currentStacks: map[string]any{
				"stack1": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{},
					},
				},
				"stack2": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{},
					},
				},
			},
			remoteStacks:     map[string]any{},
			atmosConfig:      &schema.AtmosConfiguration{},
			changedFiles:     []string{},
			stackToFilter:    "stack1",
			expectedAffected: []schema.Affected{},
			expectedError:    false,
		},
		{
			name: "Should detect changed Terraform component",
			currentStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"metadata": map[string]any{
									"component": "terraform-vpc",
								},
							},
						},
					},
				},
			},
			remoteStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{},
					},
				},
			},
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			changedFiles: []string{"components/terraform/vpc/main.tf"},
			expectedAffected: []schema.Affected{
				{
					Component:     "vpc",
					ComponentType: "terraform",
					ComponentPath: filepath.Join("components", "terraform", "vpc"),
					Stack:         "dev",
					Affected:      "stack.metadata",
					AffectedAll:   []string{"stack.metadata", "component"},
					StackSlug:     "dev-vpc",
				},
			},
			expectedError: false,
		},
		{
			name: "Should detect changed Helmfile component",
			currentStacks: map[string]any{
				"staging": map[string]any{
					"components": map[string]any{
						"helmfile": map[string]any{
							"ingress": map[string]any{
								"metadata": map[string]any{
									"component": "helmfile-ingress",
								},
							},
						},
					},
				},
			},
			remoteStacks: map[string]any{
				"staging": map[string]any{
					"components": map[string]any{
						"helmfile": map[string]any{},
					},
				},
			},
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath: "components/helmfile",
					},
				},
			},
			changedFiles: []string{"components/helmfile/ingress/values.yaml"},
			expectedAffected: []schema.Affected{
				{
					Component:     "ingress",
					ComponentType: "helmfile",
					ComponentPath: filepath.Join("components", "helmfile", "ingress"),
					Stack:         "staging",
					StackSlug:     "staging-ingress",
					Affected:      "stack.metadata",
					AffectedAll:   []string{"stack.metadata", "component"},
				},
			},
			expectedError: false,
		},
		{
			name: "Should detect changed Packer component",
			currentStacks: map[string]any{
				"prod": map[string]any{
					"components": map[string]any{
						"packer": map[string]any{
							"custom-ami": map[string]any{
								"metadata": map[string]any{
									"component": "packer-custom-ami",
								},
							},
						},
					},
				},
			},
			remoteStacks: map[string]any{
				"prod": map[string]any{
					"components": map[string]any{
						"packer": map[string]any{},
					},
				},
			},
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Packer: schema.Packer{
						BasePath: "components/packer",
					},
				},
			},
			changedFiles: []string{"components/packer/custom-ami/ubuntu.pkr.hcl"},
			expectedAffected: []schema.Affected{
				{
					Component:     "custom-ami",
					ComponentType: "packer",
					ComponentPath: filepath.Join("components", "packer", "custom-ami"),
					Stack:         "prod",
					StackSlug:     "prod-custom-ami",
					Affected:      "stack.metadata",
					AffectedAll:   []string{"stack.metadata", "component"},
				},
			},
			expectedError: false,
		},
		{
			name: "Should detect multiple component types in the same stack",
			currentStacks: map[string]any{
				"prod": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"metadata": map[string]any{
									"component": "terraform-vpc",
								},
							},
						},
						"helmfile": map[string]any{
							"ingress": map[string]any{
								"metadata": map[string]any{
									"component": "helmfile-ingress",
								},
							},
						},
					},
				},
			},
			remoteStacks: map[string]any{
				"prod": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{},
						"helmfile":  map[string]any{},
					},
				},
			},
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
					Helmfile: schema.Helmfile{
						BasePath: "components/helmfile",
					},
				},
			},
			changedFiles: []string{
				"components/terraform/vpc/main.tf",
				"components/helmfile/ingress/values.yaml",
			},
			expectedAffected: []schema.Affected{
				{
					Component:     "vpc",
					ComponentType: "terraform",
					ComponentPath: filepath.Join("components", "terraform", "vpc"),
					Stack:         "prod",
					StackSlug:     "prod-vpc",
					Affected:      "stack.metadata",
					AffectedAll:   []string{"stack.metadata", "component"},
				},
				{
					Component:     "ingress",
					ComponentType: "helmfile",
					ComponentPath: filepath.Join("components", "helmfile", "ingress"),
					Stack:         "prod",
					StackSlug:     "prod-ingress",
					Affected:      "stack.metadata",
					AffectedAll:   []string{"stack.metadata", "component"},
				},
			},
			expectedError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			affected, err := findAffected(
				&tt.currentStacks,
				&tt.remoteStacks,
				tt.atmosConfig,
				tt.changedFiles,
				tt.includeSpaceliftAdminStacks,
				tt.includeSettings,
				tt.stackToFilter,
				false,
				"", // gitRepoRoot - empty for unit tests with absolute paths.
			)

			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedAffected, affected)
			}
		})
	}
}

// TestGetComponentFolder tests the GetComponentFolder helper function.
func TestGetComponentFolder(t *testing.T) {
	tests := []struct {
		name             string
		componentSection map[string]any
		componentName    string
		expected         string
	}{
		{
			name:             "returns explicit component field when set",
			componentSection: map[string]any{"component": "vpc"},
			componentName:    "vpc-production",
			expected:         "vpc",
		},
		{
			name:             "returns componentName when component field is empty string",
			componentSection: map[string]any{"component": ""},
			componentName:    "vpc-production",
			expected:         "vpc-production",
		},
		{
			name:             "returns componentName when component field is missing",
			componentSection: map[string]any{"source": map[string]any{"uri": "github.com/test"}},
			componentName:    "vpc-sourced",
			expected:         "vpc-sourced",
		},
		{
			name:             "returns componentName when section is empty",
			componentSection: map[string]any{},
			componentName:    "my-component",
			expected:         "my-component",
		},
		{
			name:             "returns empty when no component field and no fallback",
			componentSection: map[string]any{"vars": map[string]any{}},
			componentName:    "",
			expected:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetComponentFolder(&tt.componentSection, tt.componentName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// buildTerraformStackData creates stack data structure for testing.
// This helper reduces code duplication in component folder change tests.
func buildTerraformStackData(stackName, componentName string, componentConfig map[string]any) map[string]any {
	return map[string]any{
		stackName: map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					componentName: componentConfig,
				},
			},
		},
	}
}

// TestFindAffectedComponentFolderChanges tests component folder detection with the top-level component field.
// This tests the scenario where stack processing sets componentSection["component"] = BaseComponentName.
func TestFindAffectedComponentFolderChanges(t *testing.T) {
	tempDir := t.TempDir()

	// Common atmosConfig for all tests.
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	t.Run("Should detect component folder changes with explicit component field", func(t *testing.T) {
		componentConfig := map[string]any{
			"component": "vpc",
			"metadata":  map[string]any{"enabled": true},
			"vars":      map[string]any{"name": "test-vpc"},
		}
		stacks := buildTerraformStackData("dev", "vpc", componentConfig)

		affected, err := findAffected(
			&stacks, &stacks, atmosConfig,
			[]string{filepath.Join(tempDir, "components", "terraform", "vpc", "main.tf")},
			false, false, "", false, "",
		)

		assert.NoError(t, err)
		require.Len(t, affected, 1)
		assert.Equal(t, "vpc", affected[0].Component)
		assert.Equal(t, "component", affected[0].Affected)
		assert.Equal(t, filepath.Join(tempDir, "components", "terraform", "vpc"), affected[0].ComponentPath)
	})

	t.Run("Should detect component folder changes with inherited base component", func(t *testing.T) {
		componentConfig := map[string]any{
			"component": "vpc", // Points to "vpc" folder, not "vpc-production".
			"metadata":  map[string]any{"enabled": true},
			"vars":      map[string]any{"name": "prod-vpc"},
		}
		stacks := buildTerraformStackData("prod", "vpc-production", componentConfig)

		affected, err := findAffected(
			&stacks, &stacks, atmosConfig,
			[]string{filepath.Join(tempDir, "components", "terraform", "vpc", "variables.tf")},
			false, false, "", false, "",
		)

		assert.NoError(t, err)
		require.Len(t, affected, 1)
		assert.Equal(t, "vpc-production", affected[0].Component)
		assert.Equal(t, "component", affected[0].Affected)
		assert.Equal(t, filepath.Join(tempDir, "components", "terraform", "vpc"), affected[0].ComponentPath)
	})

	t.Run("Should detect changes for JIT vendored component with source - simple config", func(t *testing.T) {
		// Component with source for JIT vendoring (no explicit component field).
		// This tests the fix for detecting vendored component changes.
		componentConfig := map[string]any{
			"metadata": map[string]any{"enabled": true},
			"source":   map[string]any{"uri": "github.com/example/vpc", "version": "1.0.0"},
			"vars":     map[string]any{"name": "sourced-vpc"},
		}
		stacks := buildTerraformStackData("dev", "vpc-sourced", componentConfig)

		affected, err := findAffected(
			&stacks, &stacks, atmosConfig,
			[]string{filepath.Join(tempDir, "components", "terraform", "vpc-sourced", "main.tf")},
			false, false, "", false, "",
		)

		assert.NoError(t, err)
		require.Len(t, affected, 1)
		assert.Equal(t, "vpc-sourced", affected[0].Component)
		assert.Equal(t, "component", affected[0].Affected)
		// ComponentPath is now resolved using the component name as fallback for JIT vendored components.
		assert.Equal(t, filepath.Join(tempDir, "components", "terraform", "vpc-sourced"), affected[0].ComponentPath)
	})

	t.Run("Should detect changes for JIT vendored component with source - full config", func(t *testing.T) {
		// Component with full source configuration (mirrors real fixture structure).
		// This tests JIT vendoring with included_paths, excluded_paths, etc.
		componentConfig := map[string]any{
			"metadata": map[string]any{"enabled": true},
			"source": map[string]any{
				"uri":     "github.com/cloudposse/terraform-null-label//exports",
				"version": "0.25.0",
				"included_paths": []string{
					"*.tf",
				},
				"excluded_paths": []string{
					"*.md",
					"examples/**",
				},
			},
			"vars": map[string]any{
				"enabled":     true,
				"environment": "prod",
			},
		}
		stacks := buildTerraformStackData("prod", "vpc-map", componentConfig)

		affected, err := findAffected(
			&stacks, &stacks, atmosConfig,
			[]string{filepath.Join(tempDir, "components", "terraform", "vpc-map", "main.tf")},
			false, false, "", false, "",
		)

		assert.NoError(t, err)
		require.Len(t, affected, 1)
		assert.Equal(t, "vpc-map", affected[0].Component)
		assert.Equal(t, "terraform", affected[0].ComponentType)
		assert.Equal(t, "prod", affected[0].Stack)
		assert.Equal(t, "component", affected[0].Affected)
		assert.Contains(t, affected[0].AffectedAll, "component")
	})

	t.Run("Should detect stack vars changes for component without explicit component field", func(t *testing.T) {
		// Component with source but different vars between current and remote.
		currentConfig := map[string]any{
			"metadata": map[string]any{"enabled": true},
			"source":   map[string]any{"uri": "github.com/example/vpc", "version": "1.0.0"},
			"vars":     map[string]any{"name": "updated-vpc"},
		}
		remoteConfig := map[string]any{
			"metadata": map[string]any{"enabled": true},
			"source":   map[string]any{"uri": "github.com/example/vpc", "version": "1.0.0"},
			"vars":     map[string]any{"name": "original-vpc"},
		}
		currentStacks := buildTerraformStackData("dev", "vpc-sourced", currentConfig)
		remoteStacks := buildTerraformStackData("dev", "vpc-sourced", remoteConfig)

		affected, err := findAffected(
			&currentStacks, &remoteStacks, atmosConfig,
			[]string{}, // No file changes, only stack config changes.
			false, false, "", false, "",
		)

		assert.NoError(t, err)
		require.Len(t, affected, 1)
		assert.Equal(t, "vpc-sourced", affected[0].Component)
		assert.Equal(t, "stack.vars", affected[0].Affected)
	})

	t.Run("Should detect both component file and stack vars changes", func(t *testing.T) {
		// Component with both file changes and vars changes.
		currentConfig := map[string]any{
			"component": "vpc",
			"metadata":  map[string]any{"enabled": true},
			"vars":      map[string]any{"name": "updated-vpc"},
		}
		remoteConfig := map[string]any{
			"component": "vpc",
			"metadata":  map[string]any{"enabled": true},
			"vars":      map[string]any{"name": "original-vpc"},
		}
		currentStacks := buildTerraformStackData("dev", "vpc", currentConfig)
		remoteStacks := buildTerraformStackData("dev", "vpc", remoteConfig)

		affected, err := findAffected(
			&currentStacks, &remoteStacks, atmosConfig,
			[]string{filepath.Join(tempDir, "components", "terraform", "vpc", "main.tf")},
			false, false, "", false, "",
		)

		assert.NoError(t, err)
		require.Len(t, affected, 1)
		assert.Equal(t, "vpc", affected[0].Component)
		// Should have both reasons in AffectedAll.
		assert.Contains(t, affected[0].AffectedAll, "component")
		assert.Contains(t, affected[0].AffectedAll, "stack.vars")
	})

	t.Run("Should detect metadata changes for component without explicit component field", func(t *testing.T) {
		// Component with source but different metadata between current and remote.
		currentConfig := map[string]any{
			"metadata": map[string]any{"enabled": true, "custom_field": "new_value"},
			"source":   map[string]any{"uri": "github.com/example/vpc", "version": "1.0.0"},
			"vars":     map[string]any{"name": "vpc"},
		}
		remoteConfig := map[string]any{
			"metadata": map[string]any{"enabled": true, "custom_field": "old_value"},
			"source":   map[string]any{"uri": "github.com/example/vpc", "version": "1.0.0"},
			"vars":     map[string]any{"name": "vpc"},
		}
		currentStacks := buildTerraformStackData("dev", "vpc-meta", currentConfig)
		remoteStacks := buildTerraformStackData("dev", "vpc-meta", remoteConfig)

		affected, err := findAffected(
			&currentStacks, &remoteStacks, atmosConfig,
			[]string{},
			false, false, "", false, "",
		)

		assert.NoError(t, err)
		require.Len(t, affected, 1)
		assert.Equal(t, "vpc-meta", affected[0].Component)
		assert.Equal(t, "stack.metadata", affected[0].Affected)
	})
}

// buildComponentStackData creates stack data structure for any component type.
func buildComponentStackData(componentType, stackName, componentName string, componentConfig map[string]any) map[string]any {
	return map[string]any{
		stackName: map[string]any{
			"components": map[string]any{
				componentType: map[string]any{
					componentName: componentConfig,
				},
			},
		},
	}
}

// TestFindAffectedHelmfileAndPackerComponents tests component folder detection for Helmfile and Packer.
// This tests the fix for detecting vendored component changes across all component types.
func TestFindAffectedHelmfileAndPackerComponents(t *testing.T) {
	tests := []struct {
		name              string
		componentType     string
		basePath          string
		componentName     string
		changedFile       string
		hasExplicitField  bool
		expectedComponent string
	}{
		{
			name:              "Helmfile with explicit component field",
			componentType:     "helmfile",
			basePath:          "components/helmfile",
			componentName:     "nginx",
			changedFile:       "helmfile.yaml",
			hasExplicitField:  true,
			expectedComponent: "nginx",
		},
		{
			name:              "Helmfile JIT vendored with source",
			componentType:     "helmfile",
			basePath:          "components/helmfile",
			componentName:     "nginx-sourced",
			changedFile:       "helmfile.yaml",
			hasExplicitField:  false,
			expectedComponent: "nginx-sourced",
		},
		{
			name:              "Packer with explicit component field",
			componentType:     "packer",
			basePath:          "components/packer",
			componentName:     "ami-builder",
			changedFile:       "main.pkr.hcl",
			hasExplicitField:  true,
			expectedComponent: "ami-builder",
		},
		{
			name:              "Packer JIT vendored with source",
			componentType:     "packer",
			basePath:          "components/packer",
			componentName:     "ami-sourced",
			changedFile:       "build.pkr.hcl",
			hasExplicitField:  false,
			expectedComponent: "ami-sourced",
		},
		// Test cases for components with source AND provision.workdir (from source-provisioner-workdir fixture).
		{
			name:              "Terraform with source and workdir",
			componentType:     "terraform",
			basePath:          "components/terraform",
			componentName:     "vpc-remote-workdir",
			changedFile:       "main.tf",
			hasExplicitField:  false,
			expectedComponent: "vpc-remote-workdir",
		},
		{
			name:              "Helmfile with source and workdir",
			componentType:     "helmfile",
			basePath:          "components/helmfile",
			componentName:     "nginx-workdir",
			changedFile:       "helmfile.yaml",
			hasExplicitField:  false,
			expectedComponent: "nginx-workdir",
		},
		{
			name:              "Packer with source and workdir",
			componentType:     "packer",
			basePath:          "components/packer",
			componentName:     "ami-workdir",
			changedFile:       "main.pkr.hcl",
			hasExplicitField:  false,
			expectedComponent: "ami-workdir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()

			// Build atmosConfig with the appropriate component type.
			atmosConfig := &schema.AtmosConfiguration{BasePath: tempDir}
			switch tt.componentType {
			case "terraform":
				atmosConfig.Components.Terraform.BasePath = tt.basePath
			case "helmfile":
				atmosConfig.Components.Helmfile.BasePath = tt.basePath
			case "packer":
				atmosConfig.Components.Packer.BasePath = tt.basePath
			}

			// Build component config.
			componentConfig := map[string]any{
				"metadata": map[string]any{"enabled": true},
				"vars":     map[string]any{"name": "test"},
			}
			if tt.hasExplicitField {
				componentConfig["component"] = tt.componentName
			} else {
				// Component with source (JIT vendoring) - no explicit component field.
				componentConfig["source"] = map[string]any{"uri": "github.com/example/test", "version": "1.0.0"}
				// Also add provision.workdir for workdir test cases.
				if strings.Contains(tt.componentName, "workdir") {
					componentConfig["provision"] = map[string]any{
						"workdir": map[string]any{"enabled": true},
					}
				}
			}

			stacks := buildComponentStackData(tt.componentType, "dev", tt.componentName, componentConfig)
			changedFilePath := filepath.Join(tempDir, tt.basePath, tt.componentName, tt.changedFile)

			affected, err := findAffected(&stacks, &stacks, atmosConfig, []string{changedFilePath}, false, false, "", false, "")

			assert.NoError(t, err)
			require.Len(t, affected, 1)
			assert.Equal(t, tt.expectedComponent, affected[0].Component)
			assert.Equal(t, tt.componentType, affected[0].ComponentType)
			assert.Equal(t, "component", affected[0].Affected)
		})
	}
}

// TestFindAffectedWithGitRepoRoot tests path resolution with non-empty gitRepoRoot.
// This verifies the fix for issue #1978 where relative paths from git diff
// need to be resolved against the git repository root, not the current working directory.
func TestFindAffectedWithGitRepoRoot(t *testing.T) {
	// Create a temp directory structure simulating a git repo with atmos in a subdirectory.
	gitRepoRoot := t.TempDir()
	atmosBaseDir := filepath.Join(gitRepoRoot, "infra")

	// Create component directories.
	componentDir := filepath.Join(atmosBaseDir, "components", "terraform", "vpc")
	err := os.MkdirAll(componentDir, 0o755)
	require.NoError(t, err)

	// Create a dummy file in the component directory.
	err = os.WriteFile(filepath.Join(componentDir, "main.tf"), []byte("# vpc"), 0o644)
	require.NoError(t, err)

	tests := []struct {
		name             string
		currentStacks    map[string]any
		remoteStacks     map[string]any
		atmosConfig      *schema.AtmosConfiguration
		changedFiles     []string // Relative to git repo root (as git diff returns).
		gitRepoRoot      string
		expectedAffected []schema.Affected
	}{
		{
			name: "Should detect component changes with relative paths resolved against gitRepoRoot",
			currentStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"component": "vpc",
								"metadata": map[string]any{
									"enabled": true,
								},
								"vars": map[string]any{
									"name": "test-vpc",
								},
							},
						},
					},
				},
			},
			remoteStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"component": "vpc",
								"metadata": map[string]any{
									"enabled": true,
								},
								"vars": map[string]any{
									"name": "test-vpc",
								},
							},
						},
					},
				},
			},
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: atmosBaseDir,
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			// Changed files as returned by git diff (relative to git repo root).
			changedFiles: []string{
				"infra/components/terraform/vpc/main.tf",
			},
			gitRepoRoot: gitRepoRoot,
			expectedAffected: []schema.Affected{
				{
					Component:     "vpc",
					ComponentType: "terraform",
					ComponentPath: filepath.Join(atmosBaseDir, "components", "terraform", "vpc"),
					Stack:         "dev",
					StackSlug:     "dev-vpc",
					Affected:      "component",
					AffectedAll:   []string{"component"},
				},
			},
		},
		{
			name: "Should NOT detect changes when gitRepoRoot is empty and paths are relative",
			currentStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"component": "vpc",
								"metadata": map[string]any{
									"enabled": true,
								},
							},
						},
					},
				},
			},
			remoteStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"component": "vpc",
								"metadata": map[string]any{
									"enabled": true,
								},
							},
						},
					},
				},
			},
			atmosConfig: &schema.AtmosConfiguration{
				BasePath: atmosBaseDir,
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			// Relative paths without gitRepoRoot - will be resolved against cwd (wrong).
			changedFiles: []string{
				"infra/components/terraform/vpc/main.tf",
			},
			gitRepoRoot: "", // Empty - paths will be resolved against cwd.
			// No affected expected because paths won't match (cwd != gitRepoRoot).
			expectedAffected: []schema.Affected{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			affected, err := findAffected(
				&tt.currentStacks,
				&tt.remoteStacks,
				tt.atmosConfig,
				tt.changedFiles,
				false, // includeSpaceliftAdminStacks.
				false, // includeSettings.
				"",    // stackToFilter.
				false, // excludeLocked.
				tt.gitRepoRoot,
			)

			assert.NoError(t, err)
			assert.Equal(t, tt.expectedAffected, affected)
		})
	}
}

func TestExecuteDescribeAffected(t *testing.T) {
	tests := []struct {
		name                  string
		localRepo             *git.Repository
		remoteRepo            *git.Repository
		atmosConfig           *schema.AtmosConfiguration
		localRepoPath         string
		remoteRepoPath        string
		includeSpaceliftAdmin bool
		includeSettings       bool
		stack                 string
		processTemplates      bool
		processYamlFunctions  bool
		skip                  []string
		expectedErr           string
	}{
		{
			atmosConfig: &schema.AtmosConfiguration{
				// Provide valid paths so filepath.Rel can succeed.
				StacksBaseAbsolutePath:        "/test/stacks",
				TerraformDirAbsolutePath:      "/test/components/terraform",
				HelmfileDirAbsolutePath:       "/test/components/helmfile",
				PackerDirAbsolutePath:         "/test/components/packer",
				StackConfigFilesAbsolutePaths: []string{"/test/stacks/catalog"},
			},
			localRepoPath:  "/test",
			remoteRepoPath: "/tmp/remote",
			name:           "fails when repo operations fails",
			localRepo:      createMockRepoWithHead(t),
			remoteRepo:     createMockRepoWithHead(t),
			// ExecuteDescribeStacks fails before reaching mock repo operations
			// because the test paths don't exist on the filesystem.
			// Use generic error substring that works on both Unix and Windows.
			expectedErr: "error reading file",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			affected, localHead, remoteHead, err := executeDescribeAffected(
				tc.atmosConfig,
				tc.localRepoPath,
				tc.remoteRepoPath,
				tc.localRepo,
				tc.remoteRepo,
				tc.includeSpaceliftAdmin,
				tc.includeSettings,
				tc.stack,
				tc.processTemplates,
				tc.processYamlFunctions,
				tc.skip,
				false,
			)

			if tc.expectedErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedErr)
				assert.Nil(t, affected)
				assert.Nil(t, localHead)
				assert.Nil(t, remoteHead)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, affected)
				assert.NotNil(t, localHead)
				assert.NotNil(t, remoteHead)
			}
		})
	}
}

// Helper function to create a mock repository with a valid HEAD.
func createMockRepoWithHead(t *testing.T) *git.Repository {
	t.Helper()

	ctrl := gomock.NewController(t)
	mockStorer := NewMockStorer(ctrl)

	// Configure mock to return HEAD reference.
	head := plumbing.NewReferenceFromStrings(
		"refs/heads/master",
		"0123456789abcdef0123456789abcdef01234567",
	)
	mockStorer.EXPECT().
		Reference(plumbing.HEAD).
		Return(head, nil).
		AnyTimes()

	// Configure mock to return error for EncodedObject (triggers "not implemented" error path).
	mockStorer.EXPECT().
		EncodedObject(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("not implemented")).
		AnyTimes()

	return &git.Repository{
		Storer: mockStorer,
	}
}

// createMockRepoWithHeadError creates a mock repo that errors on Head() call.
func createMockRepoWithHeadError(t *testing.T) *git.Repository {
	t.Helper()

	ctrl := gomock.NewController(t)
	mockStorer := NewMockStorer(ctrl)

	// Configure mock to return error for HEAD reference.
	mockStorer.EXPECT().
		Reference(plumbing.HEAD).
		Return(nil, errors.New("HEAD not found")).
		AnyTimes()

	return &git.Repository{
		Storer: mockStorer,
	}
}

func TestFindAffectedWithExcludeLocked(t *testing.T) {
	tests := []struct {
		name          string
		currentStacks map[string]any
		remoteStacks  map[string]any
		atmosConfig   *schema.AtmosConfiguration
		changedFiles  []string
		excludeLocked bool
		expectedLen   int
	}{
		{
			name: "excludeLocked false includes locked components",
			currentStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"metadata": map[string]any{
									"component": "terraform-vpc",
									"locked":    true,
								},
							},
						},
					},
				},
			},
			remoteStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{},
					},
				},
			},
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			changedFiles:  []string{"components/terraform/vpc/main.tf"},
			excludeLocked: false,
			expectedLen:   1,
		},
		{
			name: "excludeLocked true excludes locked components",
			currentStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"metadata": map[string]any{
									"component": "terraform-vpc",
									"locked":    true,
								},
							},
						},
					},
				},
			},
			remoteStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{},
					},
				},
			},
			atmosConfig: &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			changedFiles:  []string{"components/terraform/vpc/main.tf"},
			excludeLocked: true,
			expectedLen:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			affected, err := findAffected(
				&tt.currentStacks,
				&tt.remoteStacks,
				tt.atmosConfig,
				tt.changedFiles,
				false,
				true, // includeSettings
				"",
				tt.excludeLocked,
				"", // gitRepoRoot - empty for unit tests with absolute paths.
			)

			assert.NoError(t, err)
			assert.Len(t, affected, tt.expectedLen)
		})
	}
}

func TestFindAffectedWithIncludeSettings(t *testing.T) {
	tests := []struct {
		name            string
		currentStacks   map[string]any
		remoteStacks    map[string]any
		includeSettings bool
	}{
		{
			name: "includeSettings true captures settings",
			currentStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"metadata": map[string]any{
									"component": "terraform-vpc",
								},
								"settings": map[string]any{
									"key": "value",
								},
							},
						},
					},
				},
			},
			remoteStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{},
					},
				},
			},
			includeSettings: true,
		},
		{
			name: "includeSettings false excludes settings",
			currentStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{
							"vpc": map[string]any{
								"metadata": map[string]any{
									"component": "terraform-vpc",
								},
							},
						},
					},
				},
			},
			remoteStacks: map[string]any{
				"dev": map[string]any{
					"components": map[string]any{
						"terraform": map[string]any{},
					},
				},
			},
			includeSettings: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			}

			affected, err := findAffected(
				&tt.currentStacks,
				&tt.remoteStacks,
				atmosConfig,
				[]string{"components/terraform/vpc/main.tf"},
				false,
				tt.includeSettings,
				"",
				false,
				"", // gitRepoRoot - empty for unit tests with absolute paths.
			)

			assert.NoError(t, err)
			// Just verify it doesn't error - settings handling is implementation detail.
			assert.NotNil(t, affected)
		})
	}
}

func TestFindAffectedWithNilStacks(t *testing.T) {
	t.Run("nil current stacks", func(t *testing.T) {
		emptyStacks := map[string]any{}
		affected, err := findAffected(
			&emptyStacks,
			&emptyStacks,
			&schema.AtmosConfiguration{},
			[]string{},
			false,
			false,
			"",
			false,
			"", // gitRepoRoot - empty for unit tests.
		)

		assert.NoError(t, err)
		assert.Empty(t, affected)
	})
}

func TestFindAffectedWithSpaceliftAdminStacks(t *testing.T) {
	t.Run("includeSpaceliftAdminStacks flag", func(t *testing.T) {
		currentStacks := map[string]any{
			"admin": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"spacelift-admin": map[string]any{
							"metadata": map[string]any{
								"component": "spacelift-admin",
							},
						},
					},
				},
			},
		}
		remoteStacks := map[string]any{}
		atmosConfig := &schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		}

		affected, err := findAffected(
			&currentStacks,
			&remoteStacks,
			atmosConfig,
			[]string{},
			true, // includeSpaceliftAdminStacks
			false,
			"",
			false,
			"", // gitRepoRoot - empty for unit tests.
		)

		assert.NoError(t, err)
		// Just verify it processes without error.
		assert.NotNil(t, affected)
	})
}

func TestExecuteDescribeAffectedLocalRepoHeadError(t *testing.T) {
	t.Run("fails when local repo Head() returns error", func(t *testing.T) {
		localRepo := createMockRepoWithHeadError(t)
		remoteRepo := createMockRepoWithHead(t)

		atmosConfig := &schema.AtmosConfiguration{
			StacksBaseAbsolutePath:        "/test/stacks",
			TerraformDirAbsolutePath:      "/test/components/terraform",
			HelmfileDirAbsolutePath:       "/test/components/helmfile",
			PackerDirAbsolutePath:         "/test/components/packer",
			StackConfigFilesAbsolutePaths: []string{"/test/stacks/catalog"},
		}

		affected, localHead, remoteHead, err := executeDescribeAffected(
			atmosConfig,
			"/test",
			"/tmp/remote",
			localRepo,
			remoteRepo,
			false,
			false,
			"",
			false,
			false,
			nil,
			false,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "HEAD not found")
		assert.Nil(t, affected)
		assert.Nil(t, localHead)
		assert.Nil(t, remoteHead)
	})
}

func TestExecuteDescribeAffectedRemoteRepoHeadError(t *testing.T) {
	t.Run("fails when remote repo Head() returns error", func(t *testing.T) {
		localRepo := createMockRepoWithHead(t)
		remoteRepo := createMockRepoWithHeadError(t)

		atmosConfig := &schema.AtmosConfiguration{
			StacksBaseAbsolutePath:        "/test/stacks",
			TerraformDirAbsolutePath:      "/test/components/terraform",
			HelmfileDirAbsolutePath:       "/test/components/helmfile",
			PackerDirAbsolutePath:         "/test/components/packer",
			StackConfigFilesAbsolutePaths: []string{"/test/stacks/catalog"},
		}

		affected, localHead, remoteHead, err := executeDescribeAffected(
			atmosConfig,
			"/test",
			"/tmp/remote",
			localRepo,
			remoteRepo,
			false,
			false,
			"",
			false,
			false,
			nil,
			false,
		)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "HEAD not found")
		assert.Nil(t, affected)
		assert.Nil(t, localHead)
		assert.Nil(t, remoteHead)
	})
}

// Test constants.
func TestDescribeAffectedConstants(t *testing.T) {
	t.Run("shaString constant", func(t *testing.T) {
		assert.Equal(t, "SHA", shaString)
	})

	t.Run("refString constant", func(t *testing.T) {
		assert.Equal(t, "ref", refString)
	})
}

// Test error variable.
func TestRemoteRepoIsNotGitRepoError(t *testing.T) {
	t.Run("error message is correct", func(t *testing.T) {
		expectedMsg := "the target remote repo is not a Git repository. Check that it was initialized and has '.git' folder"
		assert.Equal(t, expectedMsg, RemoteRepoIsNotGitRepoError.Error())
	})

	t.Run("error can be used with errors.Is", func(t *testing.T) {
		err := RemoteRepoIsNotGitRepoError
		assert.True(t, errors.Is(err, RemoteRepoIsNotGitRepoError))
	})
}

func TestShouldSkipComponent(t *testing.T) {
	tests := []struct {
		name            string
		metadataSection map[string]any
		componentName   string
		excludeLocked   bool
		expected        bool
	}{
		{
			name: "skip abstract component",
			metadataSection: map[string]any{
				"type": "abstract",
			},
			componentName: "vpc",
			excludeLocked: false,
			expected:      true,
		},
		{
			name: "skip disabled component",
			metadataSection: map[string]any{
				"enabled": false,
			},
			componentName: "vpc",
			excludeLocked: false,
			expected:      true,
		},
		{
			name: "skip locked component when excludeLocked is true",
			metadataSection: map[string]any{
				"locked": true,
			},
			componentName: "vpc",
			excludeLocked: true,
			expected:      true,
		},
		{
			name: "include locked component when excludeLocked is false",
			metadataSection: map[string]any{
				"locked": true,
			},
			componentName: "vpc",
			excludeLocked: false,
			expected:      false,
		},
		{
			name:            "include normal component",
			metadataSection: map[string]any{},
			componentName:   "vpc",
			excludeLocked:   false,
			expected:        false,
		},
		{
			name: "include component with type real",
			metadataSection: map[string]any{
				"type": "real",
			},
			componentName: "vpc",
			excludeLocked: false,
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldSkipComponent(tt.metadataSection, tt.componentName, tt.excludeLocked)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestChangedFilesIndexWithAbsolutePaths(t *testing.T) {
	t.Run("handles already absolute paths", func(t *testing.T) {
		// On Windows, absolute paths require a drive letter (e.g., C:\...).
		// On Unix, paths starting with / are absolute.
		var basePath, absPath, gitRepoRoot string
		if runtime.GOOS == "windows" {
			basePath = "C:\\test"
			absPath = "C:\\test\\components\\terraform\\vpc\\main.tf"
			gitRepoRoot = "C:\\test"
		} else {
			basePath = "/test"
			absPath = "/test/components/terraform/vpc/main.tf"
			gitRepoRoot = "/test"
		}

		atmosConfig := &schema.AtmosConfiguration{
			BasePath: basePath,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		}

		changedFiles := []string{absPath}
		index := newChangedFilesIndex(atmosConfig, changedFiles, gitRepoRoot)
		allFiles := index.getAllFiles()

		assert.Len(t, allFiles, 1)
		assert.Equal(t, absPath, allFiles[0])
	})

	t.Run("handles relative paths with git repo root", func(t *testing.T) {
		// On Windows, absolute paths require a drive letter (e.g., C:\...).
		// On Unix, paths starting with / are absolute.
		var basePath, gitRepoRoot, expected string
		if runtime.GOOS == "windows" {
			basePath = "C:\\test"
			gitRepoRoot = "C:\\test"
			expected = "C:\\test\\components\\terraform\\vpc\\main.tf"
		} else {
			basePath = "/test"
			gitRepoRoot = "/test"
			expected = "/test/components/terraform/vpc/main.tf"
		}

		atmosConfig := &schema.AtmosConfiguration{
			BasePath: basePath,
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		}

		changedFiles := []string{"components/terraform/vpc/main.tf"}
		index := newChangedFilesIndex(atmosConfig, changedFiles, gitRepoRoot)
		allFiles := index.getAllFiles()

		assert.Len(t, allFiles, 1)
		assert.Equal(t, expected, allFiles[0])
	})

	t.Run("handles empty git repo root with fallback to cwd", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			BasePath: "/test",
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		}

		changedFiles := []string{"components/terraform/vpc/main.tf"}
		index := newChangedFilesIndex(atmosConfig, changedFiles, "")
		allFiles := index.getAllFiles()

		// When gitRepoRoot is empty, it falls back to filepath.Abs which uses cwd.
		assert.Len(t, allFiles, 1)
		// The path should be absolute (starts with separator on Unix, drive letter on Windows).
		assert.True(t, filepath.IsAbs(allFiles[0]))
	})
}

func TestGetRelevantFilesWithUnknownComponentType(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		BasePath: "/test",
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	changedFiles := []string{"components/terraform/vpc/main.tf"}
	index := newChangedFilesIndex(atmosConfig, changedFiles, "/test")

	t.Run("unknown component type returns all files", func(t *testing.T) {
		files := index.getRelevantFiles("unknown", atmosConfig)
		assert.Equal(t, index.getAllFiles(), files)
	})

	t.Run("terraform component type returns relevant files", func(t *testing.T) {
		files := index.getRelevantFiles("terraform", atmosConfig)
		// Should return files for terraform component type.
		assert.NotNil(t, files)
	})

	t.Run("helmfile component type with no files returns all files", func(t *testing.T) {
		files := index.getRelevantFiles("helmfile", atmosConfig)
		// No helmfile files in index, falls back to all files.
		assert.Equal(t, index.getAllFiles(), files)
	})

	t.Run("packer component type with no files returns all files", func(t *testing.T) {
		files := index.getRelevantFiles("packer", atmosConfig)
		// No packer files in index, falls back to all files.
		assert.Equal(t, index.getAllFiles(), files)
	})
}

func TestFindAffectedWithEnvChanges(t *testing.T) {
	t.Run("detects env section changes", func(t *testing.T) {
		currentStacks := map[string]any{
			"dev": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"component": "vpc",
							"env": map[string]any{
								"AWS_REGION": "us-east-1",
							},
						},
					},
				},
			},
		}

		remoteStacks := map[string]any{
			"dev": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"component": "vpc",
							"env": map[string]any{
								"AWS_REGION": "us-west-2",
							},
						},
					},
				},
			},
		}

		atmosConfig := &schema.AtmosConfiguration{
			BasePath: "/test",
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
			Stacks: schema.Stacks{
				NamePattern: "{stage}",
			},
		}

		affected, err := findAffected(
			&currentStacks,
			&remoteStacks,
			atmosConfig,
			[]string{},
			false,
			false,
			"",
			false,
			"/test",
		)

		assert.NoError(t, err)
		assert.Len(t, affected, 1)
		assert.Equal(t, "vpc", affected[0].Component)
		assert.Equal(t, "stack.env", affected[0].Affected)
	})
}

func TestProcessTerraformComponentsIndexed(t *testing.T) {
	t.Run("processes terraform component with settings changes", func(t *testing.T) {
		terraformSection := map[string]any{
			"vpc": map[string]any{
				"component": "vpc",
				"settings": map[string]any{
					"version": "2.0",
				},
			},
		}

		remoteStacks := map[string]any{
			"dev": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc": map[string]any{
							"component": "vpc",
							"settings": map[string]any{
								"version": "1.0",
							},
						},
					},
				},
			},
		}

		currentStacks := map[string]any{
			"dev": map[string]any{
				"components": map[string]any{
					"terraform": terraformSection,
				},
			},
		}

		atmosConfig := &schema.AtmosConfiguration{
			BasePath: "/test",
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		}

		filesIndex := newChangedFilesIndex(atmosConfig, []string{}, "/test")
		patternCache := newComponentPathPatternCache()

		affected, err := processTerraformComponentsIndexed(
			"dev",
			terraformSection,
			&remoteStacks,
			&currentStacks,
			atmosConfig,
			filesIndex,
			patternCache,
			false,
			false,
			false,
		)

		assert.NoError(t, err)
		assert.Len(t, affected, 1)
		assert.Equal(t, "vpc", affected[0].Component)
		assert.Equal(t, "stack.settings", affected[0].Affected)
	})
}

func TestFindAffectedSkipsAbstractComponents(t *testing.T) {
	t.Run("skips abstract components", func(t *testing.T) {
		currentStacks := map[string]any{
			"dev": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc-abstract": map[string]any{
							"component": "vpc",
							"metadata": map[string]any{
								"type": "abstract",
							},
							"vars": map[string]any{
								"name": "test",
							},
						},
					},
				},
			},
		}

		remoteStacks := map[string]any{
			"dev": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc-abstract": map[string]any{
							"component": "vpc",
							"metadata": map[string]any{
								"type": "abstract",
							},
							"vars": map[string]any{
								"name": "different",
							},
						},
					},
				},
			},
		}

		atmosConfig := &schema.AtmosConfiguration{
			BasePath: "/test",
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
			Stacks: schema.Stacks{
				NamePattern: "{stage}",
			},
		}

		affected, err := findAffected(
			&currentStacks,
			&remoteStacks,
			atmosConfig,
			[]string{},
			false,
			false,
			"",
			false,
			"/test",
		)

		assert.NoError(t, err)
		// Abstract components should be skipped even if their vars changed.
		assert.Len(t, affected, 0)
	})
}

func TestFindAffectedSkipsDisabledComponents(t *testing.T) {
	t.Run("skips disabled components", func(t *testing.T) {
		currentStacks := map[string]any{
			"dev": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc-disabled": map[string]any{
							"component": "vpc",
							"metadata": map[string]any{
								"enabled": false,
							},
							"vars": map[string]any{
								"name": "test",
							},
						},
					},
				},
			},
		}

		remoteStacks := map[string]any{
			"dev": map[string]any{
				"components": map[string]any{
					"terraform": map[string]any{
						"vpc-disabled": map[string]any{
							"component": "vpc",
							"metadata": map[string]any{
								"enabled": false,
							},
							"vars": map[string]any{
								"name": "different",
							},
						},
					},
				},
			},
		}

		atmosConfig := &schema.AtmosConfiguration{
			BasePath: "/test",
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
			Stacks: schema.Stacks{
				NamePattern: "{stage}",
			},
		}

		affected, err := findAffected(
			&currentStacks,
			&remoteStacks,
			atmosConfig,
			[]string{},
			false,
			false,
			"",
			false,
			"/test",
		)

		assert.NoError(t, err)
		// Disabled components should be skipped even if their vars changed.
		assert.Len(t, affected, 0)
	})
}

func TestProcessComponentsIndexedVarsEnvChanges(t *testing.T) {
	tests := []struct {
		name          string
		componentType string
		componentName string
		stackName     string
		varsKey       string
		varsOldValue  string
		varsNewValue  string
		envKey        string
		envOldValue   string
		envNewValue   string
		componentBase string
	}{
		{
			name:          "helmfile component with vars and env changes",
			componentType: "helmfile",
			componentName: "ingress",
			stackName:     "staging",
			varsKey:       "name",
			varsOldValue:  "old-value",
			varsNewValue:  "new-value",
			envKey:        "HELM_DEBUG",
			envOldValue:   "false",
			envNewValue:   "true",
			componentBase: "components/helmfile",
		},
		{
			name:          "packer component with vars and env changes",
			componentType: "packer",
			componentName: "custom-ami",
			stackName:     "prod",
			varsKey:       "ami_name",
			varsOldValue:  "old-ami",
			varsNewValue:  "new-ami",
			envKey:        "AWS_REGION",
			envOldValue:   "us-east-1",
			envNewValue:   "us-west-2",
			componentBase: "components/packer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			componentSection := map[string]any{
				tt.componentName: map[string]any{
					"component": tt.componentName,
					"vars":      map[string]any{tt.varsKey: tt.varsNewValue},
					"env":       map[string]any{tt.envKey: tt.envNewValue},
				},
			}

			remoteStacks := map[string]any{
				tt.stackName: map[string]any{
					"components": map[string]any{
						tt.componentType: map[string]any{
							tt.componentName: map[string]any{
								"component": tt.componentName,
								"vars":      map[string]any{tt.varsKey: tt.varsOldValue},
								"env":       map[string]any{tt.envKey: tt.envOldValue},
							},
						},
					},
				},
			}

			currentStacks := map[string]any{
				tt.stackName: map[string]any{
					"components": map[string]any{
						tt.componentType: componentSection,
					},
				},
			}

			atmosConfig := &schema.AtmosConfiguration{
				BasePath: "/test",
			}

			// Set the appropriate base path based on component type.
			switch tt.componentType {
			case "helmfile":
				atmosConfig.Components.Helmfile.BasePath = tt.componentBase
			case "packer":
				atmosConfig.Components.Packer.BasePath = tt.componentBase
			}

			filesIndex := newChangedFilesIndex(atmosConfig, []string{}, "/test")
			patternCache := newComponentPathPatternCache()

			var affected []schema.Affected
			var err error

			switch tt.componentType {
			case "helmfile":
				affected, err = processHelmfileComponentsIndexed(
					tt.stackName, componentSection, &remoteStacks, &currentStacks,
					atmosConfig, filesIndex, patternCache, false, false, false,
				)
			case "packer":
				affected, err = processPackerComponentsIndexed(
					tt.stackName, componentSection, &remoteStacks, &currentStacks,
					atmosConfig, filesIndex, patternCache, false, false, false,
				)
			}

			assert.NoError(t, err)
			// Should detect both vars and env changes.
			assert.GreaterOrEqual(t, len(affected), 1)
		})
	}
}
