package schema

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestComponentConfigurationSerialization(t *testing.T) {
	config := Terraform{Command: "tofu", ConfigurationProviders: map[string]ComponentConfiguration{"terragrunt": {Enabled: true, Mode: "native", Discovery: ComponentDiscoveryConfig{Enabled: true, IncludedPaths: []string{"live/**"}}, Options: map[string]any{"limit": json.Number("9007199254740993")}}}}
	raw, err := json.Marshal(config)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"limit":9007199254740993`)
	require.Contains(t, string(raw), `"terragrunt":{`)
	require.NotContains(t, string(raw), "ConfigurationProviders")
	data, err := yaml.Marshal(config)
	require.NoError(t, err)
	var decoded Terraform
	require.NoError(t, yaml.Unmarshal(data, &decoded))
	require.True(t, decoded.ConfigurationProviders["terragrunt"].Discovery.Enabled)
	require.Equal(t, []string{"live/**"}, decoded.ConfigurationProviders["terragrunt"].Discovery.IncludedPaths)
	config.ConfigurationProviders["command"] = ComponentConfiguration{}
	_, err = json.Marshal(config)
	require.ErrorIs(t, err, ErrComponentConfigurationCollision)
	_, err = json.Marshal(ComponentConfiguration{Options: map[string]any{"enabled": false}})
	require.ErrorIs(t, err, ErrComponentConfigurationCollision)
	// An empty omitempty field is still reserved and must not be replaced by an option.
	_, err = json.Marshal(ComponentConfiguration{Options: map[string]any{"mode": "native"}})
	require.ErrorIs(t, err, ErrComponentConfigurationCollision)
	_, err = json.Marshal(ComponentConfiguration{Options: map[string]any{"provider": "other"}})
	require.ErrorIs(t, err, ErrComponentConfigurationCollision)
	raw, err = json.Marshal(ComponentConfiguration{Enabled: true, Options: map[string]any{"version": "1"}})
	require.NoError(t, err)
	require.Contains(t, string(raw), `"version":"1"`)
	_, err = json.Marshal(ComponentConfiguration{Options: map[string]any{"bad": make(chan int)}})
	require.Error(t, err)
}

func TestComponentDiscoveryRequiresBothOptIns(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, discovery := range []bool{false, true} {
			config := AtmosConfiguration{Components: Components{Terraform: Terraform{ConfigurationProviders: map[string]ComponentConfiguration{
				"terragrunt": {Enabled: enabled, Discovery: ComponentDiscoveryConfig{Enabled: discovery}},
			}}}}
			require.Equal(t, enabled && discovery, config.HasComponentDiscovery())
		}
	}
	require.False(t, (&AtmosConfiguration{}).HasComponentDiscovery())
}

func TestComponentExecutionIsEmptyCountsErrorHandling(t *testing.T) {
	var none *ComponentExecution
	if !none.IsEmpty() || !(&ComponentExecution{}).IsEmpty() {
		t.Fatal("an absent or zero execution adds nothing")
	}
	if (&ComponentExecution{Errors: &ComponentErrors{}}).IsEmpty() {
		t.Fatal("error handling alone is execution metadata")
	}
	if (&ComponentExecution{BeforeHooks: []ComponentHook{{Name: "h"}}}).IsEmpty() {
		t.Fatal("a hook is execution metadata")
	}
}

func TestConfigurationOwned(t *testing.T) {
	for _, test := range []struct {
		name     string
		settings map[string]any
		want     bool
	}{
		{name: "absent", settings: map[string]any{}},
		{name: "nil settings"},
		{name: "explicit false", settings: map[string]any{"configuration": false}},
		{name: "explicit null", settings: map[string]any{"configuration": nil}},
		// settings is free-form: a user's own configuration data is never provider ownership.
		{name: "user true", settings: map[string]any{"configuration": true}},
		{name: "user notes", settings: map[string]any{"configuration": map[string]any{"notes": "internal tooling"}}},
		{name: "user provider", settings: map[string]any{"configuration": map[string]any{"provider": "terragrunt"}}},
		{name: "non-boolean marker", settings: map[string]any{"configuration": map[string]any{ConfigurationOwnedKey: "true"}}},
		{name: "marked", settings: map[string]any{"configuration": map[string]any{"provider": "terragrunt", ConfigurationOwnedKey: true}}, want: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, ConfigurationOwned(test.settings))
		})
	}
}

func TestSelectsConfigurationProvider(t *testing.T) {
	config := &AtmosConfiguration{}
	config.Components.Terraform.ConfigurationProviders = map[string]ComponentConfiguration{
		"terragrunt": {Enabled: true},
		"tg":         {Enabled: true, Provider: "custom"},
		"off":        {Enabled: false},
	}
	require.True(t, config.SelectsConfigurationProvider("terragrunt"))
	require.True(t, config.SelectsConfigurationProvider("custom"))
	require.False(t, config.SelectsConfigurationProvider("tg"), "a section with provider set selects that provider, not its own name")
	require.False(t, config.SelectsConfigurationProvider("off"))
	require.False(t, config.SelectsConfigurationProvider("aws"))
	require.False(t, config.SelectsConfigurationProvider(""))
	var unset *AtmosConfiguration
	require.False(t, unset.SelectsConfigurationProvider("terragrunt"))
}

func TestMarkConfigurationOwnership(t *testing.T) {
	enabled := &AtmosConfiguration{}
	enabled.Components.Terraform.ConfigurationProviders = map[string]ComponentConfiguration{"terragrunt": {Enabled: true}}

	t.Run("discovered terraform unit is marked without changing the shared map", func(t *testing.T) {
		shared := map[string]any{"provider": "terragrunt", "source": "live/app/terragrunt.hcl"}
		settings := map[string]any{"configuration": shared}
		enabled.MarkConfigurationOwnership("terraform", settings)
		require.True(t, ConfigurationOwned(settings))
		require.Equal(t, "live/app/terragrunt.hcl", settings["configuration"].(map[string]any)["source"])
		require.NotContains(t, shared, ConfigurationOwnedKey, "the provider's cached map must not change")
		marked := settings["configuration"]
		enabled.MarkConfigurationOwnership("terraform", settings)
		require.Equal(t, marked, settings["configuration"], "marking is idempotent")
	})

	t.Run("user data stays exactly as written", func(t *testing.T) {
		for _, configuration := range []any{
			map[string]any{"notes": "internal tooling"},
			map[string]any{"provider": "aws", "notes": "team-owned"},
			true,
			"scalar",
		} {
			settings := map[string]any{"configuration": configuration}
			(&AtmosConfiguration{}).MarkConfigurationOwnership("terraform", settings)
			require.Equal(t, map[string]any{"configuration": configuration}, settings)
			require.False(t, ConfigurationOwned(settings))
		}
	})

	t.Run("a written marker is removed", func(t *testing.T) {
		forged := map[string]any{"notes": "x", ConfigurationOwnedKey: true}
		settings := map[string]any{"configuration": forged}
		(&AtmosConfiguration{}).MarkConfigurationOwnership("terraform", settings)
		require.False(t, ConfigurationOwned(settings))
		require.Equal(t, map[string]any{"notes": "x"}, settings["configuration"])
		require.Contains(t, forged, ConfigurationOwnedKey, "the source manifest map must not change")
	})

	t.Run("only terraform components can be provider-owned", func(t *testing.T) {
		settings := map[string]any{"configuration": map[string]any{"provider": "terragrunt"}}
		enabled.MarkConfigurationOwnership("helmfile", settings)
		require.False(t, ConfigurationOwned(settings))
	})

	t.Run("a disabled provider does not own its units", func(t *testing.T) {
		disabled := &AtmosConfiguration{}
		disabled.Components.Terraform.ConfigurationProviders = map[string]ComponentConfiguration{"terragrunt": {Enabled: false}}
		settings := map[string]any{"configuration": map[string]any{"provider": "terragrunt", ConfigurationOwnedKey: true}}
		disabled.MarkConfigurationOwnership("terraform", settings)
		require.False(t, ConfigurationOwned(settings))
	})

	t.Run("missing settings", func(t *testing.T) {
		enabled.MarkConfigurationOwnership("terraform", nil)
		settings := map[string]any{}
		enabled.MarkConfigurationOwnership("terraform", settings)
		require.Empty(t, settings)
	})
}
