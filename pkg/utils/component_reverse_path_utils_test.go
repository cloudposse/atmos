package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExtractComponentInfoFromPath(t *testing.T) {
	// Create a temporary directory structure for testing
	tmpDir := t.TempDir()
	terraformBase := filepath.Join(tmpDir, "components", "terraform")
	helmfileBase := filepath.Join(tmpDir, "components", "helmfile")

	// Create test directories
	require.NoError(t, os.MkdirAll(filepath.Join(terraformBase, "vpc", "security-group"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(terraformBase, "networking", "vpc"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(terraformBase, "simple"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(helmfileBase, "app"), 0o755))

	tests := []struct {
		name          string
		atmosConfig   *schema.AtmosConfiguration
		path          string
		want          *ComponentInfo
		wantErr       bool
		errorContains string
	}{
		{
			name: "terraform component with folder prefix",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath:                 tmpDir,
				TerraformDirAbsolutePath: terraformBase,
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			path: filepath.Join(terraformBase, "vpc", "security-group"),
			want: &ComponentInfo{
				ComponentType: "terraform",
				FolderPrefix:  "vpc",
				ComponentName: "security-group",
				FullComponent: "vpc/security-group",
			},
			wantErr: false,
		},
		{
			name: "terraform component with nested folder prefix",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath:                 tmpDir,
				TerraformDirAbsolutePath: terraformBase,
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			path: filepath.Join(terraformBase, "networking", "vpc"),
			want: &ComponentInfo{
				ComponentType: "terraform",
				FolderPrefix:  "networking",
				ComponentName: "vpc",
				FullComponent: "networking/vpc",
			},
			wantErr: false,
		},
		{
			name: "terraform component without folder prefix",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath:                 tmpDir,
				TerraformDirAbsolutePath: terraformBase,
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			path: filepath.Join(terraformBase, "simple"),
			want: &ComponentInfo{
				ComponentType: "terraform",
				FolderPrefix:  "",
				ComponentName: "simple",
				FullComponent: "simple",
			},
			wantErr: false,
		},
		{
			name: "helmfile component",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath:                 tmpDir,
				TerraformDirAbsolutePath: terraformBase, // Need this so terraform type doesn't match
				HelmfileDirAbsolutePath:  helmfileBase,
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
					Helmfile: schema.Helmfile{
						BasePath: "components/helmfile",
					},
				},
			},
			path: filepath.Join(helmfileBase, "app"),
			want: &ComponentInfo{
				ComponentType: "helmfile",
				FolderPrefix:  "",
				ComponentName: "app",
				FullComponent: "app",
			},
			wantErr: false,
		},
		{
			name: "path not in component directory",
			atmosConfig: &schema.AtmosConfiguration{
				BasePath:                 tmpDir,
				TerraformDirAbsolutePath: terraformBase,
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			path:          "/tmp/random",
			wantErr:       true,
			errorContains: "not within Atmos component directories",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ExtractComponentInfoFromPath(tt.atmosConfig, tt.path)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want.ComponentType, got.ComponentType)
			assert.Equal(t, tt.want.FolderPrefix, got.FolderPrefix)
			assert.Equal(t, tt.want.ComponentName, got.ComponentName)
			assert.Equal(t, tt.want.FullComponent, got.FullComponent)
		})
	}
}

func TestExtractComponentInfoFromPath_CurrentDirectory(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()
	terraformBase := filepath.Join(tmpDir, "components", "terraform")
	componentDir := filepath.Join(terraformBase, "vpc")

	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	// Change to the component directory (t.Chdir automatically restores on cleanup)
	t.Chdir(componentDir)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tmpDir,
		TerraformDirAbsolutePath: terraformBase,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
	}

	// Test with "." (current directory)
	got, err := ExtractComponentInfoFromPath(atmosConfig, ".")
	require.NoError(t, err)

	assert.Equal(t, "terraform", got.ComponentType)
	assert.Equal(t, "", got.FolderPrefix)
	assert.Equal(t, "vpc", got.ComponentName)
	assert.Equal(t, "vpc", got.FullComponent)
}

func TestNormalizePathForResolution(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantAbs bool
		wantErr bool
	}{
		{
			name:    "absolute path",
			path:    "/tmp/test",
			wantAbs: true,
			wantErr: false,
		},
		{
			name:    "relative path",
			path:    "./test",
			wantAbs: true,
			wantErr: false,
		},
		{
			name:    "current directory",
			path:    ".",
			wantAbs: true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizePathForResolution(tt.path)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			if tt.wantAbs {
				assert.True(t, filepath.IsAbs(got), "path should be absolute")
			}
		})
	}
}

func TestExtractComponentInfoFromPath_EnvironmentVariableOverride(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()
	customBase := filepath.Join(tmpDir, "custom", "tf")
	componentDir := filepath.Join(customBase, "vpc")

	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	// Set environment variable override
	t.Setenv("ATMOS_COMPONENTS_TERRAFORM_BASE_PATH", customBase)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tmpDir,
		TerraformDirAbsolutePath: "", // Will be overridden by env var
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform", // This should be overridden
			},
		},
	}

	got, err := ExtractComponentInfoFromPath(atmosConfig, componentDir)
	require.NoError(t, err)

	assert.Equal(t, "terraform", got.ComponentType)
	assert.Equal(t, "", got.FolderPrefix)
	assert.Equal(t, "vpc", got.ComponentName)
	assert.Equal(t, "vpc", got.FullComponent)
}

// TestValidatePathIsNotConfigDirectory tests detection of config directories.
func TestValidatePathIsNotConfigDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	stacksDir := filepath.Join(tmpDir, "stacks")
	workflowsDir := filepath.Join(tmpDir, "workflows")
	componentsDir := filepath.Join(tmpDir, "components", "terraform", "vpc")

	require.NoError(t, os.MkdirAll(stacksDir, 0o755))
	require.NoError(t, os.MkdirAll(workflowsDir, 0o755))
	require.NoError(t, os.MkdirAll(componentsDir, 0o755))

	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		absPath     string
		wantErr     bool
		errContains string
	}{
		{
			name: "path is stacks directory",
			atmosConfig: &schema.AtmosConfiguration{
				StacksBaseAbsolutePath: stacksDir,
			},
			absPath:     stacksDir,
			wantErr:     true,
			errContains: "path is not within Atmos component directories",
		},
		{
			name: "path within stacks directory",
			atmosConfig: &schema.AtmosConfiguration{
				StacksBaseAbsolutePath: stacksDir,
			},
			absPath:     filepath.Join(stacksDir, "dev"),
			wantErr:     true,
			errContains: "path is not within Atmos component directories",
		},
		{
			name: "path is workflows directory",
			atmosConfig: &schema.AtmosConfiguration{
				Workflows: schema.Workflows{
					BasePath: workflowsDir,
				},
			},
			absPath:     workflowsDir,
			wantErr:     true,
			errContains: "path is not within Atmos component directories",
		},
		{
			name: "path within workflows directory",
			atmosConfig: &schema.AtmosConfiguration{
				Workflows: schema.Workflows{
					BasePath: workflowsDir,
				},
			},
			absPath:     filepath.Join(workflowsDir, "deploy.yaml"),
			wantErr:     true,
			errContains: "path is not within Atmos component directories",
		},
		{
			name: "valid component path",
			atmosConfig: &schema.AtmosConfiguration{
				StacksBaseAbsolutePath: stacksDir,
				Workflows: schema.Workflows{
					BasePath: workflowsDir,
				},
			},
			absPath: componentsDir,
			wantErr: false,
		},
		{
			name:        "empty stacks and workflows paths",
			atmosConfig: &schema.AtmosConfiguration{},
			absPath:     componentsDir,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePathIsNotConfigDirectory(tt.atmosConfig, tt.absPath)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestGetStacksBasePath tests the getStacksBasePath function.
func TestGetStacksBasePath(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		expected    string
	}{
		{
			name: "uses pre-resolved absolute path",
			atmosConfig: &schema.AtmosConfiguration{
				StacksBaseAbsolutePath: "/absolute/stacks",
				Stacks: schema.Stacks{
					BasePath: "stacks",
				},
			},
			expected: "/absolute/stacks",
		},
		{
			name: "falls back to resolving relative path",
			atmosConfig: &schema.AtmosConfiguration{
				StacksBaseAbsolutePath: "",
				Stacks: schema.Stacks{
					BasePath: "stacks",
				},
			},
			// This will be resolved to an absolute path.
			expected: "", // We'll check it's not empty.
		},
		{
			name: "empty config returns empty",
			atmosConfig: &schema.AtmosConfiguration{
				StacksBaseAbsolutePath: "",
				Stacks: schema.Stacks{
					BasePath: "",
				},
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getStacksBasePath(tt.atmosConfig)

			if tt.expected == "" && tt.atmosConfig.Stacks.BasePath != "" {
				// For relative path test, just verify it resolved to something.
				assert.NotEmpty(t, result)
				assert.True(t, filepath.IsAbs(result))
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

// TestGetWorkflowsBasePath tests the getWorkflowsBasePath function.
func TestGetWorkflowsBasePath(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		checkAbs    bool
	}{
		{
			name: "resolves relative path",
			atmosConfig: &schema.AtmosConfiguration{
				Workflows: schema.Workflows{
					BasePath: "workflows",
				},
			},
			checkAbs: true,
		},
		{
			name: "handles absolute path",
			atmosConfig: &schema.AtmosConfiguration{
				Workflows: schema.Workflows{
					BasePath: "/absolute/workflows",
				},
			},
			checkAbs: false, // Already absolute.
		},
		{
			name: "empty path returns empty",
			atmosConfig: &schema.AtmosConfiguration{
				Workflows: schema.Workflows{
					BasePath: "",
				},
			},
			checkAbs: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getWorkflowsBasePath(tt.atmosConfig)

			switch {
			case tt.atmosConfig.Workflows.BasePath == "":
				assert.Empty(t, result)
			case tt.checkAbs:
				assert.NotEmpty(t, result)
				assert.True(t, filepath.IsAbs(result))
			default:
				assert.Equal(t, tt.atmosConfig.Workflows.BasePath, result)
			}
		})
	}
}

// TestResolveBasePath tests the resolveBasePath function.
func TestResolveBasePath(t *testing.T) {
	tests := []struct {
		name     string
		basePath string
		wantAbs  bool
		wantSame bool
	}{
		{
			name:     "empty path returns empty",
			basePath: "",
			wantAbs:  false,
			wantSame: true,
		},
		{
			name:     "absolute path unchanged",
			basePath: "/absolute/path",
			wantAbs:  true,
			wantSame: true,
		},
		{
			name:     "relative path becomes absolute",
			basePath: "relative/path",
			wantAbs:  true,
			wantSame: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolveBasePath(tt.basePath)

			if tt.basePath == "" {
				assert.Empty(t, result)
			} else if tt.wantAbs {
				assert.True(t, filepath.IsAbs(result))
			}
			if tt.wantSame {
				assert.Equal(t, tt.basePath, result)
			}
		})
	}
}

// TestParseRelativePathParts tests the parseRelativePathParts function.
func TestParseRelativePathParts(t *testing.T) {
	tests := []struct {
		name     string
		relPath  string
		expected []string
	}{
		{
			name:     "single component",
			relPath:  "vpc",
			expected: []string{"vpc"},
		},
		{
			name:     "nested path",
			relPath:  filepath.Join("networking", "vpc"),
			expected: []string{"networking", "vpc"},
		},
		{
			name:     "deeply nested path",
			relPath:  filepath.Join("aws", "networking", "vpc", "security-group"),
			expected: []string{"aws", "networking", "vpc", "security-group"},
		},
		{
			name:     "empty path",
			relPath:  "",
			expected: nil, // Empty input produces nil slice.
		},
		{
			name:     "path with trailing separator",
			relPath:  "vpc" + string(filepath.Separator),
			expected: []string{"vpc"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseRelativePathParts(tt.relPath)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestBuildComponentInfo tests the buildComponentInfo function.
func TestBuildComponentInfo(t *testing.T) {
	tests := []struct {
		name          string
		componentType string
		parts         []string
		expected      *ComponentInfo
	}{
		{
			name:          "single part - no folder prefix",
			componentType: "terraform",
			parts:         []string{"vpc"},
			expected: &ComponentInfo{
				ComponentType: "terraform",
				FolderPrefix:  "",
				ComponentName: "vpc",
				FullComponent: "vpc",
			},
		},
		{
			name:          "two parts - single folder prefix",
			componentType: "terraform",
			parts:         []string{"networking", "vpc"},
			expected: &ComponentInfo{
				ComponentType: "terraform",
				FolderPrefix:  "networking",
				ComponentName: "vpc",
				FullComponent: "networking/vpc",
			},
		},
		{
			name:          "three parts - nested folder prefix",
			componentType: "helmfile",
			parts:         []string{"aws", "networking", "alb"},
			expected: &ComponentInfo{
				ComponentType: "helmfile",
				FolderPrefix:  "aws/networking",
				ComponentName: "alb",
				FullComponent: "aws/networking/alb",
			},
		},
		{
			name:          "packer component",
			componentType: "packer",
			parts:         []string{"ami", "base"},
			expected: &ComponentInfo{
				ComponentType: "packer",
				FolderPrefix:  "ami",
				ComponentName: "base",
				FullComponent: "ami/base",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildComponentInfo(tt.componentType, tt.parts)

			assert.Equal(t, tt.expected.ComponentType, result.ComponentType)
			assert.Equal(t, tt.expected.FolderPrefix, result.FolderPrefix)
			assert.Equal(t, tt.expected.ComponentName, result.ComponentName)
			assert.Equal(t, tt.expected.FullComponent, result.FullComponent)
		})
	}
}

// TestValidatePathWithinBase tests the validatePathWithinBase function.
func TestValidatePathWithinBase(t *testing.T) {
	tests := []struct {
		name          string
		absPath       string
		basePath      string
		componentType string
		wantRelPath   string
		wantErr       bool
		errContains   string
	}{
		{
			name:          "valid path within base",
			absPath:       "/project/components/terraform/vpc",
			basePath:      "/project/components/terraform",
			componentType: "terraform",
			wantRelPath:   "vpc",
			wantErr:       false,
		},
		{
			name:          "nested path within base",
			absPath:       "/project/components/terraform/networking/vpc",
			basePath:      "/project/components/terraform",
			componentType: "terraform",
			wantRelPath:   filepath.Join("networking", "vpc"),
			wantErr:       false,
		},
		{
			name:          "path outside base (parent directory)",
			absPath:       "/other/path",
			basePath:      "/project/components/terraform",
			componentType: "terraform",
			wantErr:       true,
			errContains:   "not within",
		},
		{
			name:          "path equals base (component base error)",
			absPath:       "/project/components/terraform",
			basePath:      "/project/components/terraform",
			componentType: "terraform",
			wantErr:       true,
			errContains:   "must specify a component directory, not the base directory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := validatePathWithinBase(tt.absPath, tt.basePath, tt.componentType)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantRelPath, result)
			}
		})
	}
}

// TestBuildComponentBaseError tests the buildComponentBaseError function.
func TestBuildComponentBaseError(t *testing.T) {
	tests := []struct {
		name          string
		absPath       string
		basePath      string
		componentType string
	}{
		{
			name:          "terraform base error",
			absPath:       "/project/components/terraform",
			basePath:      "/project/components/terraform",
			componentType: "terraform",
		},
		{
			name:          "helmfile base error",
			absPath:       "/project/components/helmfile",
			basePath:      "/project/components/helmfile",
			componentType: "helmfile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := buildComponentBaseError(tt.absPath, tt.basePath, tt.componentType)

			require.Error(t, err)
			assert.Contains(t, err.Error(), "must specify a component directory, not the base directory")
		})
	}
}

// TestResolveAndCleanBasePath tests the resolveAndCleanBasePath function.
func TestResolveAndCleanBasePath(t *testing.T) {
	tmpDir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmpDir, "components"), 0o755))

	tests := []struct {
		name     string
		basePath string
		wantErr  bool
	}{
		{
			name:     "absolute path",
			basePath: tmpDir,
			wantErr:  false,
		},
		{
			name:     "relative path",
			basePath: "components/terraform",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveAndCleanBasePath(tt.basePath)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.True(t, filepath.IsAbs(result))
			}
		})
	}
}
