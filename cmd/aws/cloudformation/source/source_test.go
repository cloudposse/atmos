package source

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestSourceFactoryUsesGenerationWorkdir guards the command-factory wiring:
// explicit source management must share execution's implicit isolation policy.
func TestSourceFactoryUsesGenerationWorkdir(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	config.Components.CloudFormation.AutoGenerateFiles = true
	section := map[string]any{"generate": map[string]any{"template.yaml": "Resources: {}"}}
	require.NotNil(t, cloudFormationConfig.PrepareComponentConfig)
	result, err := cloudFormationConfig.PrepareComponentConfig(config, section)
	require.NoError(t, err)
	assert.Equal(t, true, result["provision"].(map[string]any)["workdir"].(map[string]any)["enabled"])
	assert.NotContains(t, section, "provision")
}
