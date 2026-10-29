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
