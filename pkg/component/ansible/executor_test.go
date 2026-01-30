package ansible

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCheckConfig(t *testing.T) {
	t.Run("returns error when base path is empty", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Components: schema.Components{
				Ansible: schema.Ansible{
					BasePath: "",
				},
			},
		}

		err := checkConfig(atmosConfig)
		assert.ErrorIs(t, err, errUtils.ErrMissingAnsibleBasePath)
	})

	t.Run("returns nil when base path is set", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Components: schema.Components{
				Ansible: schema.Ansible{
					BasePath: "components/ansible",
				},
			},
		}

		err := checkConfig(atmosConfig)
		assert.NoError(t, err)
	})
}

func TestGetPlaybookFromSettings(t *testing.T) {
	t.Run("returns empty string when settings is nil", func(t *testing.T) {
		playbook, err := GetPlaybookFromSettings(nil)
		assert.NoError(t, err)
		assert.Empty(t, playbook)
	})

	t.Run("returns empty string when ansible section is missing", func(t *testing.T) {
		settings := schema.AtmosSectionMapType{
			"other": map[string]any{
				"key": "value",
			},
		}

		playbook, err := GetPlaybookFromSettings(&settings)
		assert.NoError(t, err)
		assert.Empty(t, playbook)
	})

	t.Run("returns empty string when ansible section is not a map", func(t *testing.T) {
		settings := schema.AtmosSectionMapType{
			"ansible": "not a map",
		}

		playbook, err := GetPlaybookFromSettings(&settings)
		assert.NoError(t, err)
		assert.Empty(t, playbook)
	})

	t.Run("returns empty string when playbook is missing", func(t *testing.T) {
		settings := schema.AtmosSectionMapType{
			"ansible": map[string]any{
				"inventory": "hosts",
			},
		}

		playbook, err := GetPlaybookFromSettings(&settings)
		assert.NoError(t, err)
		assert.Empty(t, playbook)
	})

	t.Run("returns empty string when playbook is not a string", func(t *testing.T) {
		settings := schema.AtmosSectionMapType{
			"ansible": map[string]any{
				"playbook": 123,
			},
		}

		playbook, err := GetPlaybookFromSettings(&settings)
		assert.NoError(t, err)
		assert.Empty(t, playbook)
	})

	t.Run("returns playbook when set correctly", func(t *testing.T) {
		settings := schema.AtmosSectionMapType{
			"ansible": map[string]any{
				"playbook": "site.yml",
			},
		}

		playbook, err := GetPlaybookFromSettings(&settings)
		assert.NoError(t, err)
		assert.Equal(t, "site.yml", playbook)
	})

	t.Run("returns playbook with full path", func(t *testing.T) {
		settings := schema.AtmosSectionMapType{
			"ansible": map[string]any{
				"playbook": "playbooks/deploy.yml",
			},
		}

		playbook, err := GetPlaybookFromSettings(&settings)
		assert.NoError(t, err)
		assert.Equal(t, "playbooks/deploy.yml", playbook)
	})
}

func TestGetInventoryFromSettings(t *testing.T) {
	t.Run("returns empty string when settings is nil", func(t *testing.T) {
		inventory, err := GetInventoryFromSettings(nil)
		assert.NoError(t, err)
		assert.Empty(t, inventory)
	})

	t.Run("returns empty string when ansible section is missing", func(t *testing.T) {
		settings := schema.AtmosSectionMapType{
			"other": map[string]any{
				"key": "value",
			},
		}

		inventory, err := GetInventoryFromSettings(&settings)
		assert.NoError(t, err)
		assert.Empty(t, inventory)
	})

	t.Run("returns empty string when ansible section is not a map", func(t *testing.T) {
		settings := schema.AtmosSectionMapType{
			"ansible": "not a map",
		}

		inventory, err := GetInventoryFromSettings(&settings)
		assert.NoError(t, err)
		assert.Empty(t, inventory)
	})

	t.Run("returns empty string when inventory is missing", func(t *testing.T) {
		settings := schema.AtmosSectionMapType{
			"ansible": map[string]any{
				"playbook": "site.yml",
			},
		}

		inventory, err := GetInventoryFromSettings(&settings)
		assert.NoError(t, err)
		assert.Empty(t, inventory)
	})

	t.Run("returns empty string when inventory is not a string", func(t *testing.T) {
		settings := schema.AtmosSectionMapType{
			"ansible": map[string]any{
				"inventory": []string{"host1", "host2"},
			},
		}

		inventory, err := GetInventoryFromSettings(&settings)
		assert.NoError(t, err)
		assert.Empty(t, inventory)
	})

	t.Run("returns inventory when set correctly", func(t *testing.T) {
		settings := schema.AtmosSectionMapType{
			"ansible": map[string]any{
				"inventory": "hosts.ini",
			},
		}

		inventory, err := GetInventoryFromSettings(&settings)
		assert.NoError(t, err)
		assert.Equal(t, "hosts.ini", inventory)
	})

	t.Run("returns inventory with directory path", func(t *testing.T) {
		settings := schema.AtmosSectionMapType{
			"ansible": map[string]any{
				"inventory": "inventories/production",
			},
		}

		inventory, err := GetInventoryFromSettings(&settings)
		assert.NoError(t, err)
		assert.Equal(t, "inventories/production", inventory)
	})
}

func TestConstructVarfileName(t *testing.T) {
	tests := []struct {
		name         string
		info         *schema.ConfigAndStacksInfo
		expectedName string
	}{
		{
			name: "basic component",
			info: &schema.ConfigAndStacksInfo{
				ContextPrefix: "dev-us-east-1",
				Component:     "webserver",
			},
			expectedName: "dev-us-east-1-webserver.ansible.vars.yaml",
		},
		{
			name: "component with nested path",
			info: &schema.ConfigAndStacksInfo{
				ContextPrefix: "prod-eu-west-1",
				Component:     "database/postgres",
			},
			expectedName: "prod-eu-west-1-database-postgres.ansible.vars.yaml",
		},
		{
			name: "empty context prefix",
			info: &schema.ConfigAndStacksInfo{
				ContextPrefix: "",
				Component:     "mycomponent",
			},
			expectedName: "-mycomponent.ansible.vars.yaml",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := constructVarfileName(tc.info)
			assert.Equal(t, tc.expectedName, result)
		})
	}
}

func TestConstructVarfilePath(t *testing.T) {
	tests := []struct {
		name         string
		atmosConfig  *schema.AtmosConfiguration
		info         *schema.ConfigAndStacksInfo
		expectedPath string
	}{
		{
			name: "basic path construction",
			atmosConfig: &schema.AtmosConfiguration{
				AnsibleDirAbsolutePath: filepath.Join("/project", "components", "ansible"),
			},
			info: &schema.ConfigAndStacksInfo{
				ComponentFolderPrefix: "",
				FinalComponent:        "webserver",
				ContextPrefix:         "dev-us-east-1",
				Component:             "webserver",
			},
			expectedPath: filepath.Join("/project", "components", "ansible", "webserver", "dev-us-east-1-webserver.ansible.vars.yaml"),
		},
		{
			name: "with component folder prefix",
			atmosConfig: &schema.AtmosConfiguration{
				AnsibleDirAbsolutePath: filepath.Join("/project", "components", "ansible"),
			},
			info: &schema.ConfigAndStacksInfo{
				ComponentFolderPrefix: "network",
				FinalComponent:        "vpc",
				ContextPrefix:         "prod-eu-west-1",
				Component:             "vpc",
			},
			expectedPath: filepath.Join("/project", "components", "ansible", "network", "vpc", "prod-eu-west-1-vpc.ansible.vars.yaml"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := constructVarfilePath(tc.atmosConfig, tc.info)
			assert.Equal(t, tc.expectedPath, result)
		})
	}
}

func TestConstructWorkingDir(t *testing.T) {
	tests := []struct {
		name         string
		atmosConfig  *schema.AtmosConfiguration
		info         *schema.ConfigAndStacksInfo
		expectedPath string
	}{
		{
			name: "basic working directory",
			atmosConfig: &schema.AtmosConfiguration{
				AnsibleDirAbsolutePath: filepath.Join("/project", "components", "ansible"),
			},
			info: &schema.ConfigAndStacksInfo{
				ComponentFolderPrefix: "",
				FinalComponent:        "webserver",
			},
			expectedPath: filepath.Join("/project", "components", "ansible", "webserver"),
		},
		{
			name: "with component folder prefix",
			atmosConfig: &schema.AtmosConfiguration{
				AnsibleDirAbsolutePath: filepath.Join("/project", "components", "ansible"),
			},
			info: &schema.ConfigAndStacksInfo{
				ComponentFolderPrefix: "database",
				FinalComponent:        "postgres",
			},
			expectedPath: filepath.Join("/project", "components", "ansible", "database", "postgres"),
		},
		{
			name: "nested folder prefix",
			atmosConfig: &schema.AtmosConfiguration{
				AnsibleDirAbsolutePath: filepath.Join("/opt", "atmos", "ansible"),
			},
			info: &schema.ConfigAndStacksInfo{
				ComponentFolderPrefix: filepath.Join("infra", "core"),
				FinalComponent:        "networking",
			},
			expectedPath: filepath.Join("/opt", "atmos", "ansible", "infra", "core", "networking"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := constructWorkingDir(tc.atmosConfig, tc.info)
			assert.Equal(t, tc.expectedPath, result)
		})
	}
}

func TestFlagsStruct(t *testing.T) {
	t.Run("can be created with zero values", func(t *testing.T) {
		flags := Flags{}
		assert.Empty(t, flags.Playbook)
		assert.Empty(t, flags.Inventory)
	})

	t.Run("can be created with values", func(t *testing.T) {
		flags := Flags{
			Playbook:  "site.yml",
			Inventory: "hosts.ini",
		}
		assert.Equal(t, "site.yml", flags.Playbook)
		assert.Equal(t, "hosts.ini", flags.Inventory)
	})
}

func TestGetSettingsIntegration(t *testing.T) {
	t.Run("extracts both playbook and inventory from same settings", func(t *testing.T) {
		settings := schema.AtmosSectionMapType{
			"ansible": map[string]any{
				"playbook":  "deploy.yml",
				"inventory": "production",
			},
		}

		playbook, err := GetPlaybookFromSettings(&settings)
		require.NoError(t, err)
		assert.Equal(t, "deploy.yml", playbook)

		inventory, err := GetInventoryFromSettings(&settings)
		require.NoError(t, err)
		assert.Equal(t, "production", inventory)
	})

	t.Run("handles mixed valid and invalid settings", func(t *testing.T) {
		settings := schema.AtmosSectionMapType{
			"ansible": map[string]any{
				"playbook":  "site.yml",
				"inventory": 12345, // Invalid type.
				"other":     "value",
			},
		}

		playbook, err := GetPlaybookFromSettings(&settings)
		require.NoError(t, err)
		assert.Equal(t, "site.yml", playbook)

		inventory, err := GetInventoryFromSettings(&settings)
		require.NoError(t, err)
		assert.Empty(t, inventory) // Should return empty due to type mismatch.
	})
}

func TestPathConstructionConsistency(t *testing.T) {
	t.Run("working dir and varfile path share base directory", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			AnsibleDirAbsolutePath: filepath.Join("/project", "components", "ansible"),
		}
		info := &schema.ConfigAndStacksInfo{
			ComponentFolderPrefix: "services",
			FinalComponent:        "api",
			ContextPrefix:         "dev",
			Component:             "api",
		}

		workingDir := constructWorkingDir(atmosConfig, info)
		varfilePath := constructVarfilePath(atmosConfig, info)

		// The varfile should be inside the working directory.
		assert.True(t, len(varfilePath) > len(workingDir))
		// The varfile's parent directory should be the working directory.
		assert.Equal(t, workingDir, filepath.Dir(varfilePath))
	})
}

func TestGetGenerateSectionFromComponent(t *testing.T) {
	t.Run("returns nil when component section is nil", func(t *testing.T) {
		result := getGenerateSectionFromComponent(nil)
		assert.Nil(t, result)
	})

	t.Run("returns nil when generate section is missing", func(t *testing.T) {
		componentSection := map[string]any{
			"vars": map[string]any{
				"foo": "bar",
			},
		}
		result := getGenerateSectionFromComponent(componentSection)
		assert.Nil(t, result)
	})

	t.Run("returns nil when generate section is not a map", func(t *testing.T) {
		componentSection := map[string]any{
			"generate": "not a map",
		}
		result := getGenerateSectionFromComponent(componentSection)
		assert.Nil(t, result)
	})

	t.Run("returns generate section when present", func(t *testing.T) {
		generateSection := map[string]any{
			"files": []string{"file1.yml", "file2.yml"},
		}
		componentSection := map[string]any{
			"generate": generateSection,
		}
		result := getGenerateSectionFromComponent(componentSection)
		assert.Equal(t, generateSection, result)
	})

	t.Run("returns generate section with complex content", func(t *testing.T) {
		generateSection := map[string]any{
			"providers": map[string]any{
				"aws": map[string]any{
					"region": "us-east-1",
				},
			},
			"backend": map[string]any{
				"type": "s3",
			},
		}
		componentSection := map[string]any{
			"vars": map[string]any{
				"foo": "bar",
			},
			"generate": generateSection,
		}
		result := getGenerateSectionFromComponent(componentSection)
		assert.Equal(t, generateSection, result)
	})
}
