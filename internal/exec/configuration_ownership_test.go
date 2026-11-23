package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

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
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "stacks"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "components", "terraform", "app"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "components", "terraform", "app", "main.tf"), []byte("variable \"a\" {}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(`
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

	info := schema.ConfigAndStacksInfo{ComponentFromArg: "app", ComponentType: "terraform", Stack: "dev"}
	atmosConfig, err := cfg.InitCliConfig(info, true)
	require.NoError(t, err)
	processed, err := ProcessStacks(&atmosConfig, info, true, true, true, nil, nil)
	require.NoError(t, err)
	return atmosConfig, processed
}

// TestUnknownEnabledTerraformSectionIsNotAProvider reproduces an unrecognized map with
// `enabled: true` under components.terraform. Version 1.234.1 ignored it; it must not select a
// configuration provider, fail stack processing, or appear as a provider section.
func TestUnknownEnabledTerraformSectionIsNotAProvider(t *testing.T) {
	config, info := configurationProject(t, `    team_defaults:
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
