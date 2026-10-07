package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// A step that failed to decode (here: an unknown retry key) must make only its own command fail,
// not registration of the other commands.
func TestCustomCommandWithStepLoadErrorIsStubbed(t *testing.T) {
	_ = NewTestKit(t)

	parentCmd := &cobra.Command{Use: "atmos"}
	loadErr := schema.Tasks{{Name: "s", Command: "true"}}
	loadErr[0].LoadError = schema.ErrInvalidRetryConfig

	err := processCustomCommands(schema.AtmosConfiguration{}, []schema.Command{
		{Name: "broken", Steps: loadErr},
		{Name: "healthy", Steps: schema.Tasks{{Name: "s", Type: schema.TaskTypeShell, Command: "true"}}},
	}, parentCmd)
	require.NoError(t, err)

	healthy, _, findErr := parentCmd.Find([]string{"healthy"})
	require.NoError(t, findErr)
	assert.Equal(t, "healthy", healthy.Name())

	stub, _, findErr := parentCmd.Find([]string{"broken"})
	require.NoError(t, findErr)
	require.NotNil(t, stub.RunE)
	runErr := stub.RunE(stub, nil)
	require.ErrorIs(t, runErr, schema.ErrInvalidRetryConfig)
}
