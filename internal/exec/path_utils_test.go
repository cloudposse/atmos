package exec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestConstructTerraformComponentPlanfileName(t *testing.T) {
	tests := []struct {
		name string
		info schema.ConfigAndStacksInfo
		want string
	}{
		{
			name: "without folder prefix",
			info: schema.ConfigAndStacksInfo{
				ContextPrefix: "ue2-dev",
				Component:     "vpc",
			},
			want: "ue2-dev-vpc.planfile",
		},
		{
			name: "with folder prefix",
			info: schema.ConfigAndStacksInfo{
				ContextPrefix:                 "ue2-dev",
				Component:                     "vpc",
				ComponentFolderPrefixReplaced: "infra",
			},
			want: "ue2-dev-infra-vpc.planfile",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := constructTerraformComponentPlanfileName(&tt.info)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConstructTerraformComponentVarfileName(t *testing.T) {
	tests := []struct {
		name string
		info schema.ConfigAndStacksInfo
		want string
	}{
		{
			name: "without folder prefix",
			info: schema.ConfigAndStacksInfo{
				ContextPrefix: "ue2-dev",
				Component:     "vpc",
			},
			want: "ue2-dev-vpc.terraform.tfvars.json",
		},
		{
			name: "with folder prefix",
			info: schema.ConfigAndStacksInfo{
				ContextPrefix:                 "ue2-dev",
				Component:                     "vpc",
				ComponentFolderPrefixReplaced: "infra",
			},
			want: "ue2-dev-infra-vpc.terraform.tfvars.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := constructTerraformComponentVarfileName(&tt.info)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConstructHelmfileComponentVarfileName(t *testing.T) {
	tests := []struct {
		name string
		info schema.ConfigAndStacksInfo
		want string
	}{
		{
			name: "without folder prefix",
			info: schema.ConfigAndStacksInfo{
				ContextPrefix: "ue2-dev",
				Component:     "nginx",
			},
			want: "ue2-dev-nginx.helmfile.vars.yaml",
		},
		{
			name: "with folder prefix",
			info: schema.ConfigAndStacksInfo{
				ContextPrefix:                 "ue2-dev",
				Component:                     "nginx",
				ComponentFolderPrefixReplaced: "apps",
			},
			want: "ue2-dev-apps-nginx.helmfile.vars.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := constructHelmfileComponentVarfileName(&tt.info)
			assert.Equal(t, tt.want, got)
		})
	}
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
			want: filepath.Join("/base", "packer", "infra", "app"),
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
			want: filepath.Join("/root", "packer-templates", "", "base"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := constructPackerComponentWorkingDir(&tt.atmosConfig, &tt.info)
			assert.Equal(t, tt.want, got)
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
			want: filepath.Join("/base", "packer", "infra", "app", "dev-app.packer.vars.json"),
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
			want: filepath.Join("/root", "packer-templates", "platform", "base", "prod-plat-base.packer.vars.json"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := constructPackerComponentVarfilePath(&tt.atmosConfig, &tt.info)
			assert.Equal(t, tt.want, got)
		})
	}
}
