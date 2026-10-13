package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
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
		{Name: "broken", Steps: loadErr, Commands: []schema.Command{{Name: "child"}}},
		{Name: "healthy", Steps: schema.Tasks{{Name: "s", Type: schema.TaskTypeShell, Command: "true"}}},
	}, parentCmd)
	require.NoError(t, err)

	healthy, _, findErr := parentCmd.Find([]string{"healthy"})
	require.NoError(t, findErr)
	assert.Equal(t, "healthy", healthy.Name())

	stub, _, findErr := parentCmd.Find([]string{"broken"})
	require.NoError(t, findErr)
	require.NotNil(t, stub.RunE)
	child, _, childErr := parentCmd.Find([]string{"broken", "child"})
	require.NoError(t, childErr)
	require.Equal(t, "child", child.Name())
	require.NotNil(t, child.Run)
	child.Run(child, nil)
	runErr := stub.RunE(stub, nil)
	require.ErrorIs(t, runErr, schema.ErrInvalidRetryConfig)
}

// Invalid parents remain callable stubs, but must not hide fatal errors in their children.
func TestInvalidCustomCommandPropagatesChildFlagConflict(t *testing.T) {
	loadErr := schema.Tasks{{Name: "s", Command: "true"}}
	loadErr[0].LoadError = schema.ErrInvalidRetryConfig

	for _, tt := range []struct {
		name        string
		command     schema.Command
		parentError error
	}{
		{
			name:        "step load error",
			command:     schema.Command{Name: "broken", Steps: loadErr},
			parentError: schema.ErrInvalidRetryConfig,
		},
		{
			name:        "unsupported flag type",
			command:     schema.Command{Name: "broken", Flags: []schema.CommandFlag{{Name: "ratio", Type: "float"}}},
			parentError: errUtils.ErrCustomCommandFlagType,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_ = NewTestKit(t)
			parentCmd := &cobra.Command{Use: "atmos"}
			parentCmd.PersistentFlags().Bool("feature", false, "inherited flag")
			tt.command.Commands = []schema.Command{{
				Name:  "child",
				Flags: []schema.CommandFlag{{Name: "feature", Type: "string"}},
			}}

			err := processCustomCommands(schema.AtmosConfiguration{}, []schema.Command{tt.command}, parentCmd)
			require.ErrorIs(t, err, errUtils.ErrReservedFlagName)
			assert.True(t, errUtils.HasContext(err, "command", "child"))
			assert.True(t, errUtils.HasContext(err, "flag", "feature"))

			stub, _, findErr := parentCmd.Find([]string{"broken"})
			require.NoError(t, findErr)
			require.Equal(t, "broken", stub.Name())
			require.NotNil(t, stub.RunE)
			require.ErrorIs(t, stub.RunE(stub, nil), tt.parentError)
		})
	}
}
