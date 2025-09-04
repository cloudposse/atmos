package exec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

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

func TestConstructAnsibleComponentVarfileName(t *testing.T) {
	tests := []struct {
		name string
		info schema.ConfigAndStacksInfo
		want string
	}{
		{
			name: "basic varfile name",
			info: schema.ConfigAndStacksInfo{
				ContextPrefix: "dev-us-east-1",
				Component:     "webapp",
			},
			want: "dev-us-east-1-webapp.ansible.vars.yaml",
		},
		{
			name: "varfile name with folder prefix",
			info: schema.ConfigAndStacksInfo{
				ContextPrefix:                 "prod-us-west-2",
				ComponentFolderPrefixReplaced: "infra",
				Component:                     "database",
			},
			want: "prod-us-west-2-infra-database.ansible.vars.yaml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := constructAnsibleComponentVarfileName(&tt.info)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestConstructAnsibleComponentVarfilePath(t *testing.T) {
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
					Ansible: schema.Ansible{
						BasePath: "ansible",
					},
				},
			},
			info: schema.ConfigAndStacksInfo{
				ContextPrefix:         "dev",
				ComponentFolderPrefix: "infra",
				Component:             "app",
				FinalComponent:        "app",
			},
			want: filepath.Join("/base", "ansible", "infra", "app", "dev-app.ansible.vars.yaml"),
		},
		{
			name: "path with replaced folder prefix",
			atmosConfig: schema.AtmosConfiguration{
				BasePath: "/root",
				Components: schema.Components{
					Ansible: schema.Ansible{
						BasePath: "ansible-playbooks",
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
			want: filepath.Join("/root", "ansible-playbooks", "platform", "base", "prod-plat-base.ansible.vars.yaml"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := constructAnsibleComponentVarfilePath(&tt.atmosConfig, &tt.info)
			assert.Equal(t, tt.want, got)
		})
	}
}
