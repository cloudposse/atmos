package ansible

import (
	"path/filepath"
	"slices"
	"strings"
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

func TestConstructVarfileNameEdgeCases(t *testing.T) {
	t.Run("handles multiple path separators", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ContextPrefix: "staging",
			Component:     "apps/web/frontend",
		}
		result := constructVarfileName(info)
		assert.Equal(t, "staging-apps-web-frontend.ansible.vars.yaml", result)
	})

	t.Run("handles special characters in component name", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ContextPrefix: "prod",
			Component:     "my-component_v2",
		}
		result := constructVarfileName(info)
		assert.Equal(t, "prod-my-component_v2.ansible.vars.yaml", result)
	})

	t.Run("handles long context prefix", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ContextPrefix: "organization-account-region-environment",
			Component:     "service",
		}
		result := constructVarfileName(info)
		assert.Equal(t, "organization-account-region-environment-service.ansible.vars.yaml", result)
	})
}

func TestCheckConfigEdgeCases(t *testing.T) {
	t.Run("returns nil for whitespace-only base path", func(t *testing.T) {
		// Note: This tests current behavior - whitespace is treated as non-empty.
		atmosConfig := &schema.AtmosConfiguration{
			Components: schema.Components{
				Ansible: schema.Ansible{
					BasePath: "   ",
				},
			},
		}

		err := checkConfig(atmosConfig)
		// Current behavior: whitespace-only path is considered valid.
		assert.NoError(t, err)
	})

	t.Run("returns nil for relative path", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Components: schema.Components{
				Ansible: schema.Ansible{
					BasePath: "./components/ansible",
				},
			},
		}

		err := checkConfig(atmosConfig)
		assert.NoError(t, err)
	})

	t.Run("returns nil for absolute path", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Components: schema.Components{
				Ansible: schema.Ansible{
					BasePath: "/opt/ansible/components",
				},
			},
		}

		err := checkConfig(atmosConfig)
		assert.NoError(t, err)
	})
}

func TestConstructWorkingDirEdgeCases(t *testing.T) {
	t.Run("handles empty ansible dir path", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			AnsibleDirAbsolutePath: "",
		}
		info := &schema.ConfigAndStacksInfo{
			ComponentFolderPrefix: "",
			FinalComponent:        "webserver",
		}

		result := constructWorkingDir(atmosConfig, info)
		assert.Equal(t, "webserver", result)
	})

	t.Run("handles empty final component", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			AnsibleDirAbsolutePath: filepath.Join("/project", "ansible"),
		}
		info := &schema.ConfigAndStacksInfo{
			ComponentFolderPrefix: "services",
			FinalComponent:        "",
		}

		result := constructWorkingDir(atmosConfig, info)
		assert.Equal(t, filepath.Join("/project", "ansible", "services"), result)
	})
}

func TestConstructVarfilePathEdgeCases(t *testing.T) {
	t.Run("handles all empty fields", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			AnsibleDirAbsolutePath: "",
		}
		info := &schema.ConfigAndStacksInfo{
			ComponentFolderPrefix: "",
			FinalComponent:        "",
			ContextPrefix:         "",
			Component:             "",
		}

		result := constructVarfilePath(atmosConfig, info)
		assert.Equal(t, "-.ansible.vars.yaml", result)
	})

	t.Run("handles deeply nested component folder prefix", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			AnsibleDirAbsolutePath: filepath.Join("/project", "ansible"),
		}
		info := &schema.ConfigAndStacksInfo{
			ComponentFolderPrefix: filepath.Join("level1", "level2", "level3"),
			FinalComponent:        "component",
			ContextPrefix:         "env",
			Component:             "component",
		}

		result := constructVarfilePath(atmosConfig, info)
		expectedPath := filepath.Join("/project", "ansible", "level1", "level2", "level3", "component", "env-component.ansible.vars.yaml")
		assert.Equal(t, expectedPath, result)
	})
}

func TestGetPlaybookFromSettingsEdgeCases(t *testing.T) {
	t.Run("handles empty ansible section map", func(t *testing.T) {
		settings := schema.AtmosSectionMapType{
			"ansible": map[string]any{},
		}

		playbook, err := GetPlaybookFromSettings(&settings)
		assert.NoError(t, err)
		assert.Empty(t, playbook)
	})

	t.Run("handles nil playbook value", func(t *testing.T) {
		settings := schema.AtmosSectionMapType{
			"ansible": map[string]any{
				"playbook": nil,
			},
		}

		playbook, err := GetPlaybookFromSettings(&settings)
		assert.NoError(t, err)
		assert.Empty(t, playbook)
	})

	t.Run("handles empty string playbook", func(t *testing.T) {
		settings := schema.AtmosSectionMapType{
			"ansible": map[string]any{
				"playbook": "",
			},
		}

		playbook, err := GetPlaybookFromSettings(&settings)
		assert.NoError(t, err)
		assert.Empty(t, playbook)
	})

	t.Run("handles playbook with special characters", func(t *testing.T) {
		settings := schema.AtmosSectionMapType{
			"ansible": map[string]any{
				"playbook": "playbooks/deploy-v2.1_final.yml",
			},
		}

		playbook, err := GetPlaybookFromSettings(&settings)
		assert.NoError(t, err)
		assert.Equal(t, "playbooks/deploy-v2.1_final.yml", playbook)
	})
}

func TestGetInventoryFromSettingsEdgeCases(t *testing.T) {
	t.Run("handles empty ansible section map", func(t *testing.T) {
		settings := schema.AtmosSectionMapType{
			"ansible": map[string]any{},
		}

		inventory, err := GetInventoryFromSettings(&settings)
		assert.NoError(t, err)
		assert.Empty(t, inventory)
	})

	t.Run("handles nil inventory value", func(t *testing.T) {
		settings := schema.AtmosSectionMapType{
			"ansible": map[string]any{
				"inventory": nil,
			},
		}

		inventory, err := GetInventoryFromSettings(&settings)
		assert.NoError(t, err)
		assert.Empty(t, inventory)
	})

	t.Run("handles empty string inventory", func(t *testing.T) {
		settings := schema.AtmosSectionMapType{
			"ansible": map[string]any{
				"inventory": "",
			},
		}

		inventory, err := GetInventoryFromSettings(&settings)
		assert.NoError(t, err)
		assert.Empty(t, inventory)
	})

	t.Run("handles inventory with absolute path", func(t *testing.T) {
		settings := schema.AtmosSectionMapType{
			"ansible": map[string]any{
				"inventory": "/etc/ansible/hosts",
			},
		}

		inventory, err := GetInventoryFromSettings(&settings)
		assert.NoError(t, err)
		assert.Equal(t, "/etc/ansible/hosts", inventory)
	})

	t.Run("handles inventory with dynamic script", func(t *testing.T) {
		settings := schema.AtmosSectionMapType{
			"ansible": map[string]any{
				"inventory": "./inventory/ec2.py",
			},
		}

		inventory, err := GetInventoryFromSettings(&settings)
		assert.NoError(t, err)
		assert.Equal(t, "./inventory/ec2.py", inventory)
	})
}

func TestMaybeAutoGenerateFiles(t *testing.T) {
	t.Run("returns nil when auto_generate_files is disabled", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Components: schema.Components{
				Ansible: schema.Ansible{
					AutoGenerateFiles: false,
				},
			},
		}
		info := &schema.ConfigAndStacksInfo{
			DryRun: false,
			ComponentSection: map[string]any{
				"generate": map[string]any{
					"files": []string{"test.yml"},
				},
			},
		}

		err := maybeAutoGenerateFiles(atmosConfig, info, "/some/path")
		assert.NoError(t, err)
	})

	t.Run("returns nil when in dry-run mode", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Components: schema.Components{
				Ansible: schema.Ansible{
					AutoGenerateFiles: true,
				},
			},
		}
		info := &schema.ConfigAndStacksInfo{
			DryRun: true,
			ComponentSection: map[string]any{
				"generate": map[string]any{
					"files": []string{"test.yml"},
				},
			},
		}

		err := maybeAutoGenerateFiles(atmosConfig, info, "/some/path")
		assert.NoError(t, err)
	})

	t.Run("returns nil when component has no generate section", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Components: schema.Components{
				Ansible: schema.Ansible{
					AutoGenerateFiles: true,
				},
			},
		}
		info := &schema.ConfigAndStacksInfo{
			DryRun: false,
			ComponentSection: map[string]any{
				"vars": map[string]any{
					"foo": "bar",
				},
			},
		}

		err := maybeAutoGenerateFiles(atmosConfig, info, "/some/path")
		assert.NoError(t, err)
	})

	t.Run("returns nil when generate section is nil", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Components: schema.Components{
				Ansible: schema.Ansible{
					AutoGenerateFiles: true,
				},
			},
		}
		info := &schema.ConfigAndStacksInfo{
			DryRun:           false,
			ComponentSection: nil,
		}

		err := maybeAutoGenerateFiles(atmosConfig, info, "/some/path")
		assert.NoError(t, err)
	})

	t.Run("returns nil when generate section is not a map", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Components: schema.Components{
				Ansible: schema.Ansible{
					AutoGenerateFiles: true,
				},
			},
		}
		info := &schema.ConfigAndStacksInfo{
			DryRun: false,
			ComponentSection: map[string]any{
				"generate": "not a map",
			},
		}

		err := maybeAutoGenerateFiles(atmosConfig, info, "/some/path")
		assert.NoError(t, err)
	})

	t.Run("returns nil when both auto_generate_files disabled and dry-run", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Components: schema.Components{
				Ansible: schema.Ansible{
					AutoGenerateFiles: false,
				},
			},
		}
		info := &schema.ConfigAndStacksInfo{
			DryRun: true,
			ComponentSection: map[string]any{
				"generate": map[string]any{
					"files": []string{"test.yml"},
				},
			},
		}

		err := maybeAutoGenerateFiles(atmosConfig, info, "/some/path")
		assert.NoError(t, err)
	})

	t.Run("returns error when directory creation fails", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			Components: schema.Components{
				Ansible: schema.Ansible{
					AutoGenerateFiles: true,
				},
			},
		}
		info := &schema.ConfigAndStacksInfo{
			DryRun: false,
			ComponentSection: map[string]any{
				"generate": map[string]any{
					"files": []string{"test.yml"},
				},
			},
		}

		// Use an invalid path that should fail on MkdirAll.
		// On Unix, a path starting with null byte is invalid.
		err := maybeAutoGenerateFiles(atmosConfig, info, "/dev/null/invalid")
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrCreateDirectory)
	})
}

func TestValidateComponentMetadata(t *testing.T) {
	t.Run("returns nil for non-abstract non-locked component", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentIsAbstract: false,
			ComponentIsLocked:   false,
			SubCommand:          "playbook",
			Component:           "webserver",
			ComponentFolderPrefix: "",
		}

		err := validateComponentMetadata(info)
		assert.NoError(t, err)
	})

	t.Run("returns nil for abstract component with non-playbook subcommand", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentIsAbstract: true,
			ComponentIsLocked:   false,
			SubCommand:          "version",
			Component:           "webserver",
			ComponentFolderPrefix: "",
		}

		err := validateComponentMetadata(info)
		assert.NoError(t, err)
	})

	t.Run("returns nil for locked component with non-playbook subcommand", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentIsAbstract: false,
			ComponentIsLocked:   true,
			SubCommand:          "version",
			Component:           "webserver",
			ComponentFolderPrefix: "",
		}

		err := validateComponentMetadata(info)
		assert.NoError(t, err)
	})

	t.Run("returns error for abstract component with playbook subcommand", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentIsAbstract: true,
			ComponentIsLocked:   false,
			SubCommand:          "playbook",
			Component:           "webserver",
			ComponentFolderPrefix: "",
		}

		err := validateComponentMetadata(info)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAbstractComponentCantBeProvisioned)
	})

	t.Run("returns error for locked component with playbook subcommand", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentIsAbstract: false,
			ComponentIsLocked:   true,
			SubCommand:          "playbook",
			Component:           "webserver",
			ComponentFolderPrefix: "",
		}

		err := validateComponentMetadata(info)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrLockedComponentCantBeProvisioned)
	})

	t.Run("abstract check takes precedence over locked check", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentIsAbstract: true,
			ComponentIsLocked:   true,
			SubCommand:          "playbook",
			Component:           "webserver",
			ComponentFolderPrefix: "",
		}

		err := validateComponentMetadata(info)
		assert.Error(t, err)
		// Abstract error should be returned first.
		assert.ErrorIs(t, err, errUtils.ErrAbstractComponentCantBeProvisioned)
	})

	t.Run("includes component path in error message", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentIsAbstract: true,
			ComponentIsLocked:   false,
			SubCommand:          "playbook",
			Component:           "myapp",
			ComponentFolderPrefix: "services",
		}

		err := validateComponentMetadata(info)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), filepath.Join("services", "myapp"))
	})
}

func TestResolvePlaybookConfig(t *testing.T) {
	t.Run("uses flag values when provided", func(t *testing.T) {
		flags := &Flags{
			Playbook:  "flag-playbook.yml",
			Inventory: "flag-inventory",
		}
		settings := schema.AtmosSectionMapType{
			"ansible": map[string]any{
				"playbook":  "settings-playbook.yml",
				"inventory": "settings-inventory",
			},
		}

		config, err := resolvePlaybookConfig(flags, &settings)
		require.NoError(t, err)
		assert.Equal(t, "flag-playbook.yml", config.Playbook)
		assert.Equal(t, "flag-inventory", config.Inventory)
	})

	t.Run("uses settings values when flags are empty", func(t *testing.T) {
		flags := &Flags{
			Playbook:  "",
			Inventory: "",
		}
		settings := schema.AtmosSectionMapType{
			"ansible": map[string]any{
				"playbook":  "settings-playbook.yml",
				"inventory": "settings-inventory",
			},
		}

		config, err := resolvePlaybookConfig(flags, &settings)
		require.NoError(t, err)
		assert.Equal(t, "settings-playbook.yml", config.Playbook)
		assert.Equal(t, "settings-inventory", config.Inventory)
	})

	t.Run("mixes flag and settings values", func(t *testing.T) {
		flags := &Flags{
			Playbook:  "flag-playbook.yml",
			Inventory: "",
		}
		settings := schema.AtmosSectionMapType{
			"ansible": map[string]any{
				"playbook":  "settings-playbook.yml",
				"inventory": "settings-inventory",
			},
		}

		config, err := resolvePlaybookConfig(flags, &settings)
		require.NoError(t, err)
		assert.Equal(t, "flag-playbook.yml", config.Playbook)
		assert.Equal(t, "settings-inventory", config.Inventory)
	})

	t.Run("returns empty config when nothing is specified", func(t *testing.T) {
		flags := &Flags{
			Playbook:  "",
			Inventory: "",
		}
		settings := schema.AtmosSectionMapType{}

		config, err := resolvePlaybookConfig(flags, &settings)
		require.NoError(t, err)
		assert.Empty(t, config.Playbook)
		assert.Empty(t, config.Inventory)
	})

	t.Run("handles nil settings section", func(t *testing.T) {
		flags := &Flags{
			Playbook:  "playbook.yml",
			Inventory: "",
		}

		config, err := resolvePlaybookConfig(flags, nil)
		require.NoError(t, err)
		assert.Equal(t, "playbook.yml", config.Playbook)
		assert.Empty(t, config.Inventory)
	})
}

func TestBuildCommandArgs(t *testing.T) {
	t.Run("builds playbook command with all options", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			Command:               "ansible",
			SubCommand:            "playbook",
			AdditionalArgsAndFlags: []string{"--check", "-v"},
		}
		playbookConfig := &PlaybookConfig{
			Playbook:  "site.yml",
			Inventory: "hosts.ini",
		}
		varFilePath := "/tmp/vars.yaml"

		cmdArgs, err := buildCommandArgs(info, playbookConfig, varFilePath)
		require.NoError(t, err)
		assert.Equal(t, "ansible-playbook", cmdArgs.Command)
		assert.Contains(t, cmdArgs.Args, "--extra-vars")
		assert.Contains(t, cmdArgs.Args, "@/tmp/vars.yaml")
		assert.Contains(t, cmdArgs.Args, "-i")
		assert.Contains(t, cmdArgs.Args, "hosts.ini")
		assert.Contains(t, cmdArgs.Args, "--check")
		assert.Contains(t, cmdArgs.Args, "-v")
		// Playbook should be last argument.
		assert.Equal(t, "site.yml", cmdArgs.Args[len(cmdArgs.Args)-1])
	})

	t.Run("builds playbook command without inventory", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			Command:               "ansible",
			SubCommand:            "playbook",
			AdditionalArgsAndFlags: []string{},
		}
		playbookConfig := &PlaybookConfig{
			Playbook:  "site.yml",
			Inventory: "",
		}
		varFilePath := "/tmp/vars.yaml"

		cmdArgs, err := buildCommandArgs(info, playbookConfig, varFilePath)
		require.NoError(t, err)
		assert.Equal(t, "ansible-playbook", cmdArgs.Command)
		assert.NotContains(t, cmdArgs.Args, "-i")
		assert.Equal(t, "site.yml", cmdArgs.Args[len(cmdArgs.Args)-1])
	})

	t.Run("returns error when playbook is missing for playbook subcommand", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			Command:               "ansible",
			SubCommand:            "playbook",
			AdditionalArgsAndFlags: []string{},
		}
		playbookConfig := &PlaybookConfig{
			Playbook:  "",
			Inventory: "hosts.ini",
		}
		varFilePath := "/tmp/vars.yaml"

		cmdArgs, err := buildCommandArgs(info, playbookConfig, varFilePath)
		assert.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrAnsiblePlaybookMissing)
		assert.Nil(t, cmdArgs)
	})

	t.Run("builds non-playbook command", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			Command:               "ansible",
			SubCommand:            "galaxy",
			AdditionalArgsAndFlags: []string{"install", "-r", "requirements.yml"},
		}
		playbookConfig := &PlaybookConfig{
			Playbook:  "",
			Inventory: "",
		}
		varFilePath := "/tmp/vars.yaml"

		cmdArgs, err := buildCommandArgs(info, playbookConfig, varFilePath)
		require.NoError(t, err)
		assert.Equal(t, "ansible", cmdArgs.Command)
		assert.Equal(t, []string{"galaxy", "install", "-r", "requirements.yml"}, cmdArgs.Args)
	})

	t.Run("builds version subcommand", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			Command:               "ansible",
			SubCommand:            "--version",
			AdditionalArgsAndFlags: []string{},
		}
		playbookConfig := &PlaybookConfig{}
		varFilePath := "/tmp/vars.yaml"

		cmdArgs, err := buildCommandArgs(info, playbookConfig, varFilePath)
		require.NoError(t, err)
		assert.Equal(t, "ansible", cmdArgs.Command)
		assert.Equal(t, []string{"--version"}, cmdArgs.Args)
	})
}

func TestPrepareEnvVars(t *testing.T) {
	t.Run("adds atmos environment variables", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			CliConfigPath: "/etc/atmos/atmos.yaml",
			BasePath:      "/project",
		}
		envList := []string{"EXISTING=value"}

		envVars, err := prepareEnvVars(atmosConfig, envList)
		require.NoError(t, err)

		assert.Contains(t, envVars, "EXISTING=value")

		// Check for ATMOS_CLI_CONFIG_PATH.
		assert.True(t, slices.Contains(envVars, "ATMOS_CLI_CONFIG_PATH=/etc/atmos/atmos.yaml"), "ATMOS_CLI_CONFIG_PATH should be set")

		// Check for ATMOS_BASE_PATH (should be absolute).
		hasBasePath := slices.ContainsFunc(envVars, func(v string) bool {
			return strings.HasPrefix(v, "ATMOS_BASE_PATH=")
		})
		assert.True(t, hasBasePath, "ATMOS_BASE_PATH should be set")
	})

	t.Run("preserves existing environment variables", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			CliConfigPath: "/etc/atmos.yaml",
			BasePath:      "/project",
		}
		envList := []string{"PATH=/usr/bin", "HOME=/home/user", "ANSIBLE_HOST_KEY_CHECKING=False"}

		envVars, err := prepareEnvVars(atmosConfig, envList)
		require.NoError(t, err)

		assert.Contains(t, envVars, "PATH=/usr/bin")
		assert.Contains(t, envVars, "HOME=/home/user")
		assert.Contains(t, envVars, "ANSIBLE_HOST_KEY_CHECKING=False")
	})

	t.Run("handles empty environment list", func(t *testing.T) {
		atmosConfig := &schema.AtmosConfiguration{
			CliConfigPath: "/etc/atmos.yaml",
			BasePath:      "/project",
		}

		envVars, err := prepareEnvVars(atmosConfig, nil)
		require.NoError(t, err)
		assert.NotEmpty(t, envVars)
		// Should have at least ATMOS_CLI_CONFIG_PATH and ATMOS_BASE_PATH.
		assert.GreaterOrEqual(t, len(envVars), 2)
	})
}
