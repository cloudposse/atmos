package terraform

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

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
