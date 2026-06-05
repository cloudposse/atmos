package exec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestConstructTerraformComponentWorkingDir_WithWorkdirPath tests workdir path resolution.
func TestConstructTerraformComponentWorkingDir_WithWorkdirPath(t *testing.T) {
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
	got := constructTerraformComponentWorkingDir(&atmosConfig, &info)
	assert.Equal(t, workdirPath, got)

	// Test empty workdir path falls back to standard path.
	info2 := schema.ConfigAndStacksInfo{
		ComponentFolderPrefix: "",
		FinalComponent:        "vpc",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: "",
		},
	}
	got2 := constructTerraformComponentWorkingDir(&atmosConfig, &info2)
	assert.Equal(t, filepath.Join("base", "components", "terraform", "vpc"), got2)

	// Test no workdir path uses standard path.
	info3 := schema.ConfigAndStacksInfo{
		ComponentFolderPrefix: "",
		FinalComponent:        "vpc",
		ComponentSection:      map[string]any{},
	}
	got3 := constructTerraformComponentWorkingDir(&atmosConfig, &info3)
	assert.Equal(t, filepath.Join("base", "components", "terraform", "vpc"), got3)
}

// TestConstructHelmfileComponentWorkingDir_WithWorkdirPath tests workdir path resolution for helmfile.
func TestConstructHelmfileComponentWorkingDir_WithWorkdirPath(t *testing.T) {
	// Test workdir path takes precedence (returned verbatim from config).
	workdirPath := filepath.Join("workdir", "helmfile", "dev-nginx")
	atmosConfig := schema.AtmosConfiguration{
		BasePath: "base",
		Components: schema.Components{
			Helmfile: schema.Helmfile{
				BasePath: filepath.Join("components", "helmfile"),
			},
		},
	}
	info := schema.ConfigAndStacksInfo{
		ComponentFolderPrefix: "",
		FinalComponent:        "nginx",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: workdirPath,
		},
	}
	got := constructHelmfileComponentWorkingDir(&atmosConfig, &info)
	assert.Equal(t, workdirPath, got)

	// Test standard path when no workdir.
	info2 := schema.ConfigAndStacksInfo{
		ComponentFolderPrefix: "",
		FinalComponent:        "nginx",
		ComponentSection:      map[string]any{},
	}
	got2 := constructHelmfileComponentWorkingDir(&atmosConfig, &info2)
	assert.Equal(t, filepath.Join("base", "components", "helmfile", "nginx"), got2)
}

// TestConstructPackerComponentWorkingDir_WithWorkdirPath tests workdir path resolution for packer.
func TestConstructPackerComponentWorkingDir_WithWorkdirPath(t *testing.T) {
	// Test workdir path takes precedence (returned verbatim from config).
	workdirPath := filepath.Join("workdir", "packer", "dev-ami")
	atmosConfig := schema.AtmosConfiguration{
		BasePath: "base",
		Components: schema.Components{
			Packer: schema.Packer{
				BasePath: filepath.Join("components", "packer"),
			},
		},
	}
	info := schema.ConfigAndStacksInfo{
		ComponentFolderPrefix: "",
		FinalComponent:        "ami",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: workdirPath,
		},
	}
	got := constructPackerComponentWorkingDir(&atmosConfig, &info)
	assert.Equal(t, workdirPath, got)

	// Test standard path when no workdir.
	info2 := schema.ConfigAndStacksInfo{
		ComponentFolderPrefix: "",
		FinalComponent:        "ami",
		ComponentSection:      map[string]any{},
	}
	got2 := constructPackerComponentWorkingDir(&atmosConfig, &info2)
	assert.Equal(t, filepath.Join("base", "components", "packer", "ami"), got2)
}

func TestConstructPackerComponentVarfileName(t *testing.T) {
	tests := []struct {
		name string
		info schema.ConfigAndStacksInfo
		want string
	}{
		{
			name: "with empty component folder prefix",
			info: schema.ConfigAndStacksInfo{
				ContextPrefix: "dev",
				Component:     "example",
			},
			want: "dev-example.packer.vars.json",
		},
		{
			name: "with component folder prefix",
			info: schema.ConfigAndStacksInfo{
				ContextPrefix:                 "prod",
				Component:                     "webapp",
				ComponentFolderPrefixReplaced: "infra",
			},
			want: "prod-infra-webapp.packer.vars.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := constructPackerComponentVarfileName(&tt.info)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConstructPackerComponentWorkingDir(t *testing.T) {
	// Test basic working dir with folder prefix.
	atmosConfig1 := schema.AtmosConfiguration{
		BasePath: "base",
		Components: schema.Components{
			Packer: schema.Packer{
				BasePath: "packer",
			},
		},
	}
	info1 := schema.ConfigAndStacksInfo{
		ComponentFolderPrefix: "infra",
		FinalComponent:        "app",
	}
	got1 := constructPackerComponentWorkingDir(&atmosConfig1, &info1)
	assert.Equal(t, filepath.Join("base", "packer", "infra", "app"), got1)

	// Test empty folder prefix.
	atmosConfig2 := schema.AtmosConfiguration{
		BasePath: "root",
		Components: schema.Components{
			Packer: schema.Packer{
				BasePath: "packer-templates",
			},
		},
	}
	info2 := schema.ConfigAndStacksInfo{
		ComponentFolderPrefix: "",
		FinalComponent:        "base",
	}
	got2 := constructPackerComponentWorkingDir(&atmosConfig2, &info2)
	assert.Equal(t, filepath.Join("root", "packer-templates", "base"), got2)
}

// TestConstructTerraformComponentVarfilePath_WithWorkdirPath tests varfile path with JIT vendored components.
// This test verifies that varfile paths correctly use workdir paths set by JIT provisioning.
func TestConstructTerraformComponentVarfilePath_WithWorkdirPath(t *testing.T) {
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
	got := constructTerraformComponentVarfilePath(&atmosConfig, &info)
	assert.Equal(t, filepath.Join(workdirPath, "tenant1-ue2-dev-vpc.terraform.tfvars.json"), got)

	// Test varfile path uses standard path when no workdir.
	info2 := schema.ConfigAndStacksInfo{
		ContextPrefix:         "tenant1-ue2-dev",
		ComponentFolderPrefix: "",
		Component:             "vpc",
		FinalComponent:        "vpc",
		ComponentSection:      map[string]any{},
	}
	got2 := constructTerraformComponentVarfilePath(&atmosConfig, &info2)
	assert.Equal(t, filepath.Join("base", "components", "terraform", "vpc", "tenant1-ue2-dev-vpc.terraform.tfvars.json"), got2)
}

// TestConstructTerraformComponentWorkingDir_JITVendoredComponent tests working dir for JIT vendored components.
// This simulates the scenario where a component is downloaded via JIT provisioning
// and the workdir path is set by the source provisioner.
func TestConstructTerraformComponentWorkingDir_JITVendoredComponent(t *testing.T) {
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

			got := constructTerraformComponentWorkingDir(&atmosConfig, &info)
			assert.Equal(t, tt.expectedPath, got)
		})
	}
}

func TestConstructPackerComponentVarfilePath(t *testing.T) {
	// Test complete path construction.
	atmosConfig1 := schema.AtmosConfiguration{
		BasePath: "base",
		Components: schema.Components{
			Packer: schema.Packer{
				BasePath: "packer",
			},
		},
	}
	info1 := schema.ConfigAndStacksInfo{
		ContextPrefix:         "dev",
		ComponentFolderPrefix: "infra",
		Component:             "app",
		FinalComponent:        "app",
	}
	got1 := constructPackerComponentVarfilePath(&atmosConfig1, &info1)
	assert.Equal(t, filepath.Join("base", "packer", "infra", "app", "dev-app.packer.vars.json"), got1)

	// Test path with replaced folder prefix.
	atmosConfig2 := schema.AtmosConfiguration{
		BasePath: "root",
		Components: schema.Components{
			Packer: schema.Packer{
				BasePath: "packer-templates",
			},
		},
	}
	info2 := schema.ConfigAndStacksInfo{
		ContextPrefix:                 "prod",
		ComponentFolderPrefix:         "platform",
		ComponentFolderPrefixReplaced: "plat",
		Component:                     "base",
		FinalComponent:                "base",
	}
	got2 := constructPackerComponentVarfilePath(&atmosConfig2, &info2)
	assert.Equal(t, filepath.Join("root", "packer-templates", "platform", "base", "prod-plat-base.packer.vars.json"), got2)
}

func TestConstructHelmfileComponentVarfileName(t *testing.T) {
	tests := []struct {
		name string
		info schema.ConfigAndStacksInfo
		want string
	}{
		{
			name: "simple component",
			info: schema.ConfigAndStacksInfo{
				ContextPrefix: "tenant1-ue2-dev",
				Component:     "echo-server",
			},
			want: "tenant1-ue2-dev-echo-server.helmfile.vars.yaml",
		},
		{
			name: "with folder prefix replaced",
			info: schema.ConfigAndStacksInfo{
				ContextPrefix:                 "tenant1-ue2-prod",
				Component:                     "nginx",
				ComponentFolderPrefixReplaced: "apps",
			},
			want: "tenant1-ue2-prod-apps-nginx.helmfile.vars.yaml",
		},
		{
			name: "empty folder prefix replaced",
			info: schema.ConfigAndStacksInfo{
				ContextPrefix:                 "dev-ue2-staging",
				Component:                     "monitoring",
				ComponentFolderPrefixReplaced: "",
			},
			want: "dev-ue2-staging-monitoring.helmfile.vars.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := constructHelmfileComponentVarfileName(&tt.info)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConstructHelmfileComponentVarfilePath(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig schema.AtmosConfiguration
		info        schema.ConfigAndStacksInfo
		want        string
	}{
		{
			name: "basic path",
			atmosConfig: schema.AtmosConfiguration{
				BasePath: "project",
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath: filepath.Join("components", "helmfile"),
					},
				},
			},
			info: schema.ConfigAndStacksInfo{
				ContextPrefix:         "tenant1-ue2-dev",
				ComponentFolderPrefix: "",
				Component:             "echo-server",
				FinalComponent:        "echo-server",
			},
			want: filepath.Join("project", "components", "helmfile", "echo-server", "tenant1-ue2-dev-echo-server.helmfile.vars.yaml"),
		},
		{
			name: "with folder prefix",
			atmosConfig: schema.AtmosConfiguration{
				BasePath: "base",
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath: "helmfile",
					},
				},
			},
			info: schema.ConfigAndStacksInfo{
				ContextPrefix:         "prod-us-west-2",
				ComponentFolderPrefix: "apps",
				Component:             "api-gateway",
				FinalComponent:        "api-gateway",
			},
			want: filepath.Join("base", "helmfile", "apps", "api-gateway", "prod-us-west-2-api-gateway.helmfile.vars.yaml"),
		},
		{
			name: "with workdir path (JIT vendored)",
			atmosConfig: schema.AtmosConfiguration{
				BasePath: "project",
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath: filepath.Join("components", "helmfile"),
					},
				},
			},
			info: schema.ConfigAndStacksInfo{
				ContextPrefix:         "tenant1-ue2-dev",
				ComponentFolderPrefix: "",
				Component:             "vendored-chart",
				FinalComponent:        "vendored-chart",
				ComponentSection: map[string]any{
					provWorkdir.WorkdirPathKey: filepath.Join("tmp", "vendor", "chart"),
				},
			},
			want: filepath.Join("tmp", "vendor", "chart", "tenant1-ue2-dev-vendored-chart.helmfile.vars.yaml"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := constructHelmfileComponentVarfilePath(&tt.atmosConfig, &tt.info)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConstructHelmfileComponentWorkingDir_WithFolderPrefix(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig schema.AtmosConfiguration
		info        schema.ConfigAndStacksInfo
		want        string
	}{
		{
			name: "with folder prefix",
			atmosConfig: schema.AtmosConfiguration{
				BasePath: "base",
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath: filepath.Join("components", "helmfile"),
					},
				},
			},
			info: schema.ConfigAndStacksInfo{
				ComponentFolderPrefix: "apps",
				FinalComponent:        "nginx",
			},
			want: filepath.Join("base", "components", "helmfile", "apps", "nginx"),
		},
		{
			name: "deeply nested folder prefix",
			atmosConfig: schema.AtmosConfiguration{
				BasePath: "project",
				Components: schema.Components{
					Helmfile: schema.Helmfile{
						BasePath: "helmfile",
					},
				},
			},
			info: schema.ConfigAndStacksInfo{
				ComponentFolderPrefix: filepath.Join("platform", "monitoring"),
				FinalComponent:        "prometheus",
			},
			want: filepath.Join("project", "helmfile", "platform", "monitoring", "prometheus"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := constructHelmfileComponentWorkingDir(&tt.atmosConfig, &tt.info)
			assert.Equal(t, tt.want, got)
		})
	}
}
