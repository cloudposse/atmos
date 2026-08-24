package terraform

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestLogOrderFlagRegistered(t *testing.T) {
	cases := map[string]*cobra.Command{
		"apply":   applyCmd,
		"deploy":  deployCmd,
		"destroy": destroyCmd,
		"init":    initCmd,
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := c.Flags().Lookup("log-order")
			require.NotNil(t, f, "--log-order must be registered on %s", name)
			assert.Equal(t, "stream", f.DefValue, "default should be stream")
		})
	}
}

func TestDeployConcurrencyFlagsRegistered(t *testing.T) {
	for _, name := range []string{"max-concurrency", "failure-mode", "log-order"} {
		assert.NotNil(t, deployCmd.Flags().Lookup(name), "deploy must register --%s", name)
	}
	assert.Equal(t, "1", deployCmd.Flags().Lookup("max-concurrency").DefValue)
	assert.Equal(t, "fail-fast", deployCmd.Flags().Lookup("failure-mode").DefValue)
}

func TestDeployOptionsFlowToInfo(t *testing.T) {
	v := viper.New()
	v.Set("log-order", "grouped")
	v.Set("max-concurrency", 4)
	v.Set("failure-mode", terraformFailureModeKeepGoing)

	opts, err := ParseTerraformRunOptions(v)
	require.NoError(t, err)

	info := &schema.ConfigAndStacksInfo{}
	applyOptionsToInfo(info, opts)

	assert.Equal(t, "grouped", info.TerraformLogOrder)
	assert.Equal(t, 4, info.MaxConcurrency)
	assert.Equal(t, terraformFailureModeKeepGoing, info.TerraformFailureMode)
}
