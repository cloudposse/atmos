package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guards for the schema fields these tests rely on.
var (
	_ = schema.Terraform{ConfigurationProviders: nil}
	_ = schema.ConfigAndStacksInfo{ComponentSettingsSection: nil}
)

// configurationProject writes a v1.234.1-style project with one Terraform component and returns
// the processed configuration and component info for `app` in `dev`.
func configurationProject(t *testing.T, terraform, component string) (schema.AtmosConfiguration, schema.ConfigAndStacksInfo) {
	t.Helper()
	return pinnedConfigurationProject(t, "", terraform, component)
}

// pinnedConfigurationProject is configurationProject with an `edition` pin ("" for none).
func pinnedConfigurationProject(t *testing.T, pin, terraform, component string) (schema.AtmosConfiguration, schema.ConfigAndStacksInfo) {
	t.Helper()
	info := writeConfigurationProject(t, pin, terraform, component)
	atmosConfig, err := cfg.InitCliConfig(info, true)
	require.NoError(t, err)
	processed, err := ProcessStacks(&atmosConfig, info, true, true, true, nil, nil)
	require.NoError(t, err)
	return atmosConfig, processed
}

// writeConfigurationProject writes the project into a temporary directory, makes it the working
// directory, and returns the component info for `app` in `dev`.
func writeConfigurationProject(t *testing.T, pin, terraform, component string) schema.ConfigAndStacksInfo {
	t.Helper()
	edition := ""
	if pin != "" {
		edition = "edition: \"" + pin + "\"\n"
	}
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "stacks"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "components", "terraform", "app"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "components", "terraform", "app", "main.tf"), []byte("variable \"a\" {}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(edition+`
base_path: "."
stacks:
  base_path: stacks
  included_paths: ["*.yaml"]
  name_pattern: "{stage}"
components:
  terraform:
    base_path: components/terraform
`+terraform), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "dev.yaml"), []byte(`
vars:
  stage: dev
terraform:
  backend_type: s3
  backend:
    s3:
      bucket: my-tfstate
      key: terraform.tfstate
      region: us-east-1
components:
  terraform:
    app:
`+component), 0o644))
	t.Chdir(dir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
	ClearFindStacksMapCache()
	t.Cleanup(ClearFindStacksMapCache)

	return schema.ConfigAndStacksInfo{ComponentFromArg: "app", ComponentType: "terraform", Stack: "dev"}
}

// TestUnknownEnabledTerraformSectionIsNotAProvider reproduces an unrecognized map with
// `enabled: true` under components.terraform. Version 1.234.1 ignored it; under editions before the
// unknown-sections transition it must still not select a configuration provider, fail stack
// processing, or appear as a provider section.
func TestUnknownEnabledTerraformSectionIsNotAProvider(t *testing.T) {
	config, info := pinnedConfigurationProject(t, "2026-11-26", `    team_defaults:
      enabled: true
      owner: platform
`, `      vars:
        a: 1
`)
	require.Nil(t, config.ComponentConfigurations, "no configuration provider session may start")
	require.NotContains(t, config.Components.Terraform.ConfigurationProviders, "team_defaults")
	require.False(t, config.HasComponentConfiguration())
	require.Equal(t, "dev-app.terraform.tfvars.json", constructTerraformComponentVarfileName(&info))
}

// TestUnknownTerraformSectionsFailConfigurationLoading covers the current edition end to end: the
// loader records the unknown keys and configuration loading reports them, while an earlier pin keeps
// loading the project.
func TestUnknownTerraformSectionsFailConfigurationLoading(t *testing.T) {
	for name, terraform := range map[string]string{
		"enabled mapping":    "    team_defaults:\n      enabled: true\n      owner: platform\n",
		"misspelled setting": "    base_pth: components/terraform\n",
		"unmarked mapping":   "    team_notes:\n      owner: platform\n",
	} {
		t.Run(name, func(t *testing.T) {
			for _, pin := range []string{"", "2026-11-27"} {
				info := writeConfigurationProject(t, pin, terraform, "      vars:\n        a: 1\n")
				_, err := cfg.InitCliConfig(info, true)
				require.ErrorIs(t, err, errUtils.ErrUnknownTerraformSection, "pin %q", pin)
			}
			config, _ := pinnedConfigurationProject(t, "2026-11-26", terraform, "      vars:\n        a: 1\n")
			require.False(t, config.HasComponentConfiguration())
		})
	}
}

// TestKnownTerraformSettingsAreNotUnknownSections is the negative path: built-in settings, including
// nested ones, load under the current edition.
func TestKnownTerraformSettingsAreNotUnknownSections(t *testing.T) {
	config, _ := pinnedConfigurationProject(t, "", "    apply_auto_approve: true\n    init:\n      reconfigure: never\n    plan_diff: {}\n", "      vars:\n        a: 1\n")
	require.Empty(t, config.Components.Terraform.UnknownSections)
	require.True(t, config.Components.Terraform.ApplyAutoApprove)
}
