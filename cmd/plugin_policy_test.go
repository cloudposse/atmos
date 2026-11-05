package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPluginCLIEnforcesCorporateRegistryBeforeSideEffects(t *testing.T) {
	for _, arguments := range [][]string{
		{"plugin", "install", "oci://public.example.com/team/auth", "--as=auth"},
		{"plugin", "search", "auth"},
		{"plugin", "registry", "add", "public", "oci://public.example.com/catalog:stable"},
		{"plugin", "registry", "login", "public.example.com", "--username=unused", "--password-stdin"},
		{"plugin", "test", "not-created"},
	} {
		t.Run(arguments[1]+"-"+arguments[2], func(t *testing.T) {
			config := pluginCoverageProject(t)
			t.Cleanup(ci.SwapRegistryForTest())
			config.Plugins.Registries = map[string]schema.PluginRegistry{
				"public": {Catalog: "oci://public.example.com/catalog:stable"},
			}
			writePluginCoverageConfig(t, config)
			directory := filepath.Join(config.BasePath, "policy.d")
			require.NoError(t, os.Mkdir(directory, 0o700))
			t.Setenv("ATMOS_POLICY_DIR", directory)
			const document = `apiVersion: atmos/v1
kind: Policy
metadata: {name: company}
spec:
  policies:
    approved-registries:
      on: plugin
      action: deny
      rule: input.registry != 'registry.example.com'
      reason: Use the company plugin registry.
`
			require.NoError(t, os.WriteFile(filepath.Join(directory, "plugins.yaml"), []byte(document), 0o600))
			configPath := filepath.Join(config.BasePath, "atmos.yaml")
			before, err := os.ReadFile(configPath)
			require.NoError(t, err)
			os.Args = append([]string{"atmos"}, arguments...)
			RootCmd.SetArgs(arguments)
			command, _, err := RootCmd.Find(arguments)
			require.NoError(t, err)
			previousContext := command.Context()
			t.Cleanup(func() { command.SetContext(previousContext) })
			command.SetContext(t.Context())
			err = Execute()
			require.ErrorIs(t, err, errUtils.ErrPolicyDenied)
			formatted := errUtils.Format(err, errUtils.DefaultFormatterConfig())
			require.Contains(t, formatted, "company/approved-registries")
			require.Contains(t, formatted, "Use the company plugin registry.")
			after, err := os.ReadFile(configPath)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.NoFileExists(t, pluginlock.Path(config.BasePath))
			require.NoDirExists(t, filepath.Join(config.BasePath, "keyring"), "login policy must precede credential storage")
		})
	}
}
