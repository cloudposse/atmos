package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Engine detection for the backend writer follows the component's resolved command, not the global one,
// and leaves the caller's configuration untouched.
func TestComponentCommandConfigUsesTheResolvedCommand(t *testing.T) {
	config := &schema.AtmosConfiguration{}
	config.Components.Terraform.Command = "terraform"

	resolved := componentCommandConfig(config, &schema.ConfigAndStacksInfo{Command: "/opt/tools/tofu"})
	require.NotSame(t, config, resolved)
	assert.Equal(t, "/opt/tools/tofu", resolved.Components.Terraform.Command)
	assert.Equal(t, "terraform", config.Components.Terraform.Command, "the global configuration is not modified")

	assert.Same(t, config, componentCommandConfig(config, &schema.ConfigAndStacksInfo{}))
	assert.Same(t, config, componentCommandConfig(config, nil))
	assert.Nil(t, componentCommandConfig(nil, &schema.ConfigAndStacksInfo{Command: "tofu"}))
}
