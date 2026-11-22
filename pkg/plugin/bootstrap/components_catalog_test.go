package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/component/configuration"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestConfigurationProvidersReadsDeclaredExports(t *testing.T) {
	atmos := localComponentFixture(t)
	disabled := false
	atmos.Plugins.Packages["off"] = schema.PluginPackage{Source: "oci://registry.example.test/atmos/off", Version: "1.0.0", Enabled: &disabled}
	names, complete, err := ConfigurationProviders(t.Context(), atmos)
	require.NoError(t, err)
	require.True(t, complete, "a disabled package is not consulted")
	require.Equal(t, map[string]bool{"fixture/hcl": true}, names, "only discovery exports name configuration providers")
}

func TestConfigurationProvidersWithoutPackagesReadsNothing(t *testing.T) {
	// The project directory does not exist: nothing may be read when no package is declared.
	atmos := &schema.AtmosConfiguration{BasePathConfigDir: filepath.Join(t.TempDir(), "absent")}
	names, complete, err := ConfigurationProviders(t.Context(), atmos)
	require.NoError(t, err)
	require.True(t, complete)
	require.Empty(t, names)
}

func TestConfigurationProvidersReportsUnknownExports(t *testing.T) {
	atmos := localComponentFixture(t)
	// A registry package with no lock entry has unknown exports until it is installed.
	atmos.Plugins.Packages["remote"] = schema.PluginPackage{Source: "oci://registry.example.test/atmos/remote", Version: "1.0.0"}
	_, complete, err := ConfigurationProviders(t.Context(), atmos)
	require.NoError(t, err)
	require.False(t, complete)

	broken := localComponentFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(broken.Plugins.Packages["fixture"].Path, "plugin.yaml"), []byte("kind: [unterminated\n"), 0o600))
	_, _, err = ConfigurationProviders(t.Context(), broken)
	require.Error(t, err)
}

// TestAttachKeepsDeclaredPluginProviderSection is the negative path of ignoring unknown sections: a
// section that selects a provider a declared package exports still starts the provider session,
// while an unrelated mapping beside it is dropped.
func TestAttachKeepsDeclaredPluginProviderSection(t *testing.T) {
	atmos := localComponentFixture(t)
	atmos.BasePathAbsolute = t.TempDir()
	atmos.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{
		"hcl":           {Enabled: true, Provider: "fixture/hcl"},
		"team_defaults": {Enabled: true, Options: map[string]any{"owner": "platform"}},
	}
	require.NoError(t, configuration.Attach(t.Context(), atmos, "list"))
	require.NotNil(t, atmos.ComponentConfigurations)
	require.Contains(t, atmos.Components.Terraform.ConfigurationProviders, "hcl")
	require.NotContains(t, atmos.Components.Terraform.ConfigurationProviders, "team_defaults")
}
