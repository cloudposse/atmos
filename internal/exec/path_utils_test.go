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
	tests := []struct {
		name        string
		atmosConfig schema.AtmosConfiguration
		info        schema.ConfigAndStacksInfo
		want        string
	}{
		{
			name: "workdir path from provisioner takes precedence",
			atmosConfig: schema.AtmosConfiguration{
				BasePath: "/base",
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			info: schema.ConfigAndStacksInfo{
				ComponentFolderPrefix: "",
				FinalComponent:        "vpc",
				ComponentSection: map[string]any{
					provWorkdir.WorkdirPathKey: "/workdir/terraform/dev-vpc",
				},
			},
			want: "/workdir/terraform/dev-vpc",
		},
		{
			name: "empty workdir path falls back to standard path",
			atmosConfig: schema.AtmosConfiguration{
				BasePath: "/base",
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			info: schema.ConfigAndStacksInfo{
				ComponentFolderPrefix: "",
				FinalComponent:        "vpc",
				ComponentSection: map[string]any{
					provWorkdir.WorkdirPathKey: "",
				},
			},
			want: "/base/components/terraform/vpc",
		},
		{
			name: "no workdir path uses standard path",
			atmosConfig: schema.AtmosConfiguration{
				BasePath: "/base",
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath: "components/terraform",
					},
				},
			},
			info: schema.ConfigAndStacksInfo{
				ComponentFolderPrefix: "",
				FinalComponent:        "vpc",
				ComponentSection:      map[string]any{},
			},
			want: "/base/components/terraform/vpc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := constructTerraformComponentWorkingDir(&tt.atmosConfig, &tt.info)
			assert.Equal(t, filepath.FromSlash(tt.want), got)
		})
	}
}

// TestConstructHelmfileComponentWorkingDir_WithWorkdirPath tests workdir path resolution for helmfile.
func TestConstructHelmfileComponentWorkingDir_WithWorkdirPath(t *testing.T) {
	// Test workdir path takes precedence.
	atmosConfig := schema.AtmosConfiguration{
		BasePath: "/base",
		Components: schema.Components{
			Helmfile: schema.Helmfile{
				BasePath: "components/helmfile",
			},
		},
	}
	info := schema.ConfigAndStacksInfo{
		ComponentFolderPrefix: "",
		FinalComponent:        "nginx",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: "/workdir/helmfile/dev-nginx",
		},
	}
	got := constructHelmfileComponentWorkingDir(&atmosConfig, &info)
	assert.Equal(t, filepath.FromSlash("/workdir/helmfile/dev-nginx"), got)

	// Test standard path when no workdir.
	info2 := schema.ConfigAndStacksInfo{
		ComponentFolderPrefix: "",
		FinalComponent:        "nginx",
		ComponentSection:      map[string]any{},
	}
	got2 := constructHelmfileComponentWorkingDir(&atmosConfig, &info2)
	assert.Equal(t, filepath.FromSlash("/base/components/helmfile/nginx"), got2)
}

// TestConstructPackerComponentWorkingDir_WithWorkdirPath tests workdir path resolution for packer.
func TestConstructPackerComponentWorkingDir_WithWorkdirPath(t *testing.T) {
	// Test workdir path takes precedence.
	atmosConfig := schema.AtmosConfiguration{
		BasePath: "/base",
		Components: schema.Components{
			Packer: schema.Packer{
				BasePath: "components/packer",
			},
		},
	}
	info := schema.ConfigAndStacksInfo{
		ComponentFolderPrefix: "",
		FinalComponent:        "ami",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: "/workdir/packer/dev-ami",
		},
	}
	got := constructPackerComponentWorkingDir(&atmosConfig, &info)
	assert.Equal(t, filepath.FromSlash("/workdir/packer/dev-ami"), got)

	// Test standard path when no workdir.
	info2 := schema.ConfigAndStacksInfo{
		ComponentFolderPrefix: "",
		FinalComponent:        "ami",
		ComponentSection:      map[string]any{},
	}
	got2 := constructPackerComponentWorkingDir(&atmosConfig, &info2)
	assert.Equal(t, filepath.FromSlash("/base/components/packer/ami"), got2)
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
	tests := []struct {
		name        string
		atmosConfig schema.AtmosConfiguration
		info        schema.ConfigAndStacksInfo
		want        string
	}{
		{
			name: "basic working dir",
			atmosConfig: schema.AtmosConfiguration{
				BasePath: "/base",
				Components: schema.Components{
					Packer: schema.Packer{
						BasePath: "packer",
					},
				},
			},
			info: schema.ConfigAndStacksInfo{
				ComponentFolderPrefix: "infra",
				FinalComponent:        "app",
			},
			want: "/base/packer/infra/app",
		},
		{
			name: "empty folder prefix",
			atmosConfig: schema.AtmosConfiguration{
				BasePath: "/root",
				Components: schema.Components{
					Packer: schema.Packer{
						BasePath: "packer-templates",
					},
				},
			},
			info: schema.ConfigAndStacksInfo{
				ComponentFolderPrefix: "",
				FinalComponent:        "base",
			},
			want: "/root/packer-templates/base",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := constructPackerComponentWorkingDir(&tt.atmosConfig, &tt.info)
			assert.Equal(t, filepath.FromSlash(tt.want), got)
		})
	}
}

func TestConstructPackerComponentVarfilePath(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig schema.AtmosConfiguration
		info        schema.ConfigAndStacksInfo
		want        string
	}{
		{
			name: "complete path construction",
			atmosConfig: schema.AtmosConfiguration{
				BasePath: "/base",
				Components: schema.Components{
					Packer: schema.Packer{
						BasePath: "packer",
					},
				},
			},
			info: schema.ConfigAndStacksInfo{
				ContextPrefix:         "dev",
				ComponentFolderPrefix: "infra",
				Component:             "app",
				FinalComponent:        "app",
			},
			want: "/base/packer/infra/app/dev-app.packer.vars.json",
		},
		{
			name: "path with replaced folder prefix",
			atmosConfig: schema.AtmosConfiguration{
				BasePath: "/root",
				Components: schema.Components{
					Packer: schema.Packer{
						BasePath: "packer-templates",
					},
				},
			},
			info: schema.ConfigAndStacksInfo{
				ContextPrefix:                 "prod",
				ComponentFolderPrefix:         "platform",
				ComponentFolderPrefixReplaced: "plat",
				Component:                     "base",
				FinalComponent:                "base",
			},
			want: "/root/packer-templates/platform/base/prod-plat-base.packer.vars.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := constructPackerComponentVarfilePath(&tt.atmosConfig, &tt.info)
			assert.Equal(t, filepath.FromSlash(tt.want), got)
		})
	}
}
