package terraform

import (
	os "os"
	filepath "path/filepath"
	testing "testing"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	schema "github.com/cloudposse/atmos/pkg/schema"
	assert "github.com/stretchr/testify/assert"
	require "github.com/stretchr/testify/require"
)

func TestValidateBackendConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		info        *schema.ConfigAndStacksInfo
		expectedErr error
	}{
		{
			name: "Valid backend config",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType:    "s3",
				ComponentBackendSection: map[string]any{"bucket": "test-bucket"},
			},
			expectedErr: nil,
		},
		{
			name: "Missing backend type",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType:    "",
				ComponentBackendSection: map[string]any{"bucket": "test-bucket"},
			},
			expectedErr: errUtils.ErrMissingTerraformBackendType,
		},
		{
			name: "Missing backend section",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType:    "s3",
				ComponentBackendSection: nil,
			},
			expectedErr: errUtils.ErrMissingTerraformBackendConfig,
		},
		{
			name: "Both missing",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType:    "",
				ComponentBackendSection: nil,
			},
			expectedErr: errUtils.ErrMissingTerraformBackendType,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateBackendConfig(tt.info)
			if tt.expectedErr != nil {
				assert.ErrorIs(t, err, tt.expectedErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateBackendTypeRequirements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		info        *schema.ConfigAndStacksInfo
		expectedErr error
	}{
		{
			name: "S3 backend with workspace_key_prefix",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType: cfg.BackendTypeS3,
				ComponentBackendSection: map[string]any{
					"workspace_key_prefix": "terraform-state",
				},
			},
			expectedErr: nil,
		},
		{
			name: "S3 backend without workspace_key_prefix",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType:    cfg.BackendTypeS3,
				ComponentBackendSection: map[string]any{},
			},
			expectedErr: errUtils.ErrMissingTerraformWorkspaceKeyPrefix,
		},
		{
			name: "GCS backend with bucket",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType: cfg.BackendTypeGCS,
				ComponentBackendSection: map[string]any{
					"bucket": "test-bucket",
				},
			},
			expectedErr: nil,
		},
		{
			name: "GCS backend without bucket",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType:    cfg.BackendTypeGCS,
				ComponentBackendSection: map[string]any{},
			},
			expectedErr: errUtils.ErrGCSBucketRequired,
		},
		{
			name: "Azure backend - no specific requirements",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType:    cfg.BackendTypeAzurerm,
				ComponentBackendSection: map[string]any{},
			},
			expectedErr: nil,
		},
		{
			name: "Local backend - no specific requirements",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType:    "local",
				ComponentBackendSection: map[string]any{},
			},
			expectedErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateBackendTypeRequirements(tt.info)
			if tt.expectedErr != nil {
				assert.ErrorIs(t, err, tt.expectedErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExecuteTerraformGenerateBackendCmd_Deprecated(t *testing.T) {
	t.Parallel()

	err := ExecuteTerraformGenerateBackendCmd(nil, nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDeprecatedCmdNotCallable)
}

func TestWriteBackendConfigFile(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	componentDir := filepath.Join(tempDir, "components", "terraform", "vpc")
	workDir := filepath.Join(tempDir, "workdir", "vpc")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))
	require.NoError(t, os.MkdirAll(workDir, 0o755))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: filepath.Join("components", "terraform"),
			},
		},
	}

	tests := []struct {
		name         string
		info         *schema.ConfigAndStacksInfo
		config       map[string]any
		wantPath     string
		wantContains string
	}{
		{
			name:   "dry-run skips writing",
			info:   &schema.ConfigAndStacksInfo{FinalComponent: "vpc", DryRun: true},
			config: map[string]any{"terraform": map[string]any{"backend": map[string]any{"s3": map[string]any{"bucket": "test-bucket"}}}},
		},
		{
			name:         "writes to component dir",
			info:         &schema.ConfigAndStacksInfo{FinalComponent: "vpc", ComponentSection: map[string]any{}},
			config:       map[string]any{"terraform": map[string]any{"backend": map[string]any{"s3": map[string]any{"bucket": "my-state-bucket", "key": "vpc/terraform.tfstate", "region": "us-east-1"}}}},
			wantPath:     filepath.Join(componentDir, "backend.tf.json"),
			wantContains: "my-state-bucket",
		},
		{
			name: "writes to workdir path",
			info: &schema.ConfigAndStacksInfo{
				FinalComponent:   "vpc",
				ComponentSection: map[string]any{provWorkdir.WorkdirPathKey: workDir},
			},
			config:   map[string]any{"terraform": map[string]any{"backend": map[string]any{"s3": map[string]any{"bucket": "test"}}}},
			wantPath: filepath.Join(workDir, "backend.tf.json"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := writeBackendConfigFile(atmosConfig, tt.info, tt.config)
			assert.NoError(t, err)

			if tt.wantPath != "" {
				assert.FileExists(t, tt.wantPath)
				if tt.wantContains != "" {
					content, readErr := os.ReadFile(tt.wantPath)
					require.NoError(t, readErr)
					assert.Contains(t, string(content), tt.wantContains)
				}
			}
		})
	}
}

func TestValidateBackendTypeRequirementsTypeAssertions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		info        *schema.ConfigAndStacksInfo
		expectedErr error
	}{
		{
			name: "S3 backend with workspace_key_prefix as int (wrong type)",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType: cfg.BackendTypeS3,
				ComponentBackendSection: map[string]any{
					"workspace_key_prefix": 123, // Wrong type, should be string.
				},
			},
			expectedErr: errUtils.ErrMissingTerraformWorkspaceKeyPrefix,
		},
		{
			name: "GCS backend with bucket as int (wrong type)",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType: cfg.BackendTypeGCS,
				ComponentBackendSection: map[string]any{
					"bucket": 123, // Wrong type, should be string.
				},
			},
			expectedErr: errUtils.ErrGCSBucketRequired,
		},
		{
			name: "S3 backend with empty string workspace_key_prefix",
			info: &schema.ConfigAndStacksInfo{
				ComponentBackendType: cfg.BackendTypeS3,
				ComponentBackendSection: map[string]any{
					"workspace_key_prefix": "",
				},
			},
			expectedErr: nil, // Empty string is still a string type.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := validateBackendTypeRequirements(tt.info)
			if tt.expectedErr != nil {
				assert.ErrorIs(t, err, tt.expectedErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConstructTerraformComponentWorkingDir_WithWorkdirPath(t *testing.T) {
	t.Parallel()

	// Test workdir path takes precedence (returned verbatim from config).
	workdirPath := filepath.Join("workdir", "terraform", "dev-vpc")
	atmosConfig := schema.AtmosConfiguration{
		BasePath: "base",
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: filepath.Join("components", "terraform"),
			},
		},
	}
	info := schema.ConfigAndStacksInfo{
		ComponentFolderPrefix: "",
		FinalComponent:        "vpc",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: workdirPath,
		},
	}
	got := ConstructTerraformComponentWorkingDir(&atmosConfig, &info)
	assert.Equal(t, workdirPath, got)

	// Test empty workdir path falls back to standard path.
	info2 := schema.ConfigAndStacksInfo{
		ComponentFolderPrefix: "",
		FinalComponent:        "vpc",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: "",
		},
	}
	got2 := ConstructTerraformComponentWorkingDir(&atmosConfig, &info2)
	assert.Equal(t, filepath.Join("base", "components", "terraform", "vpc"), got2)

	// Test no workdir path uses standard path.
	info3 := schema.ConfigAndStacksInfo{
		ComponentFolderPrefix: "",
		FinalComponent:        "vpc",
		ComponentSection:      map[string]any{},
	}
	got3 := ConstructTerraformComponentWorkingDir(&atmosConfig, &info3)
	assert.Equal(t, filepath.Join("base", "components", "terraform", "vpc"), got3)
}

func TestConstructTerraformComponentVarfilePath_WithWorkdirPath(t *testing.T) {
	t.Parallel()

	// Test varfile path uses workdir path when set (JIT vendored component scenario).
	workdirPath := filepath.Join("workdir", "terraform", "dev-vpc")
	atmosConfig := schema.AtmosConfiguration{
		BasePath: "base",
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: filepath.Join("components", "terraform"),
			},
		},
	}
	info := schema.ConfigAndStacksInfo{
		ContextPrefix:         "tenant1-ue2-dev",
		ComponentFolderPrefix: "",
		Component:             "vpc",
		FinalComponent:        "vpc",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: workdirPath,
		},
	}
	got := ConstructTerraformComponentVarfilePath(&atmosConfig, &info)
	assert.Equal(t, filepath.Join(workdirPath, "tenant1-ue2-dev-vpc.terraform.tfvars.json"), got)

	// Test varfile path uses standard path when no workdir.
	info2 := schema.ConfigAndStacksInfo{
		ContextPrefix:         "tenant1-ue2-dev",
		ComponentFolderPrefix: "",
		Component:             "vpc",
		FinalComponent:        "vpc",
		ComponentSection:      map[string]any{},
	}
	got2 := ConstructTerraformComponentVarfilePath(&atmosConfig, &info2)
	assert.Equal(t, filepath.Join("base", "components", "terraform", "vpc", "tenant1-ue2-dev-vpc.terraform.tfvars.json"), got2)
}

func TestConstructTerraformComponentWorkingDir_JITVendoredComponent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		workdirPath   string
		expectedPath  string
		componentName string
		hasSource     bool
		sourceConfig  map[string]any
	}{
		{
			name:          "JIT vendored component with workdir path",
			workdirPath:   filepath.Join("tmp", "atmos-vendor", "abc123", "modules", "vpc"),
			expectedPath:  filepath.Join("tmp", "atmos-vendor", "abc123", "modules", "vpc"),
			componentName: "vpc",
			hasSource:     true,
			sourceConfig: map[string]any{
				"source": map[string]any{
					"uri": "git::https://github.com/cloudposse/terraform-aws-vpc.git?ref=v1.0.0",
				},
			},
		},
		{
			name:          "JIT vendored component with string source",
			workdirPath:   filepath.Join("tmp", "vendor", "my-component"),
			expectedPath:  filepath.Join("tmp", "vendor", "my-component"),
			componentName: "my-component",
			hasSource:     true,
			sourceConfig: map[string]any{
				"source": "git::https://github.com/org/repo.git?ref=main",
			},
		},
		{
			name:          "Regular component without source (no workdir)",
			workdirPath:   "",
			expectedPath:  filepath.Join("base", "components", "terraform", "vpc"),
			componentName: "vpc",
			hasSource:     false,
			sourceConfig:  map[string]any{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			atmosConfig := schema.AtmosConfiguration{
				BasePath: "base",
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: filepath.Join("components", "terraform"),
					},
				},
			}

			componentSection := tt.sourceConfig
			if tt.workdirPath != "" {
				componentSection[provWorkdir.WorkdirPathKey] = tt.workdirPath
			}

			info := schema.ConfigAndStacksInfo{
				ComponentFolderPrefix: "",
				FinalComponent:        tt.componentName,
				ComponentSection:      componentSection,
			}

			got := ConstructTerraformComponentWorkingDir(&atmosConfig, &info)
			assert.Equal(t, tt.expectedPath, got)
		})
	}
}
