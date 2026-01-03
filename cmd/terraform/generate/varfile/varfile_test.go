package varfile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewVarfileCommand(t *testing.T) {
	cmd := NewVarfileCommand()

	t.Run("command structure", func(t *testing.T) {
		assert.Equal(t, "varfile <component>", cmd.Use)
		assert.Equal(t, "Generate a varfile for a Terraform component", cmd.Short)
		assert.Contains(t, cmd.Long, "varfile")
	})

	t.Run("requires exactly one arg", func(t *testing.T) {
		// Args should be ExactArgs(1).
		err := cmd.Args(cmd, []string{})
		assert.Error(t, err, "Should error with no args")

		err = cmd.Args(cmd, []string{"component1", "component2"})
		assert.Error(t, err, "Should error with two args")

		err = cmd.Args(cmd, []string{"component"})
		assert.NoError(t, err, "Should pass with exactly one arg")
	})

	t.Run("has expected flags", func(t *testing.T) {
		flags := cmd.Flags()

		// Stack flag.
		stackFlag := flags.Lookup("stack")
		require.NotNil(t, stackFlag, "stack flag should exist")
		assert.Equal(t, "s", stackFlag.Shorthand)
		assert.Equal(t, "", stackFlag.DefValue)

		// File flag.
		fileFlag := flags.Lookup("file")
		require.NotNil(t, fileFlag, "file flag should exist")
		assert.Equal(t, "f", fileFlag.Shorthand)
		assert.Equal(t, "", fileFlag.DefValue)
	})

	t.Run("stack flag is required", func(t *testing.T) {
		// Check that stack flag is marked as required.
		flags := cmd.Flags()
		stackFlag := flags.Lookup("stack")
		require.NotNil(t, stackFlag)

		// Annotations contain the required annotation.
		annotations := stackFlag.Annotations
		_, hasRequired := annotations["cobra_annotation_bash_completion_one_required_flag"]
		assert.True(t, hasRequired, "stack flag should be marked as required")
	})

	t.Run("has RunE function", func(t *testing.T) {
		assert.NotNil(t, cmd.RunE, "RunE should be set")
	})

	t.Run("unknown flags not whitelisted", func(t *testing.T) {
		assert.False(t, cmd.FParseErrWhitelist.UnknownFlags)
	})
}

func TestNewVarfileCommand_FlagValues(t *testing.T) {
	cmd := NewVarfileCommand()

	t.Run("can set and get stack flag", func(t *testing.T) {
		err := cmd.Flags().Set("stack", "dev-us-west-2")
		require.NoError(t, err)

		val, err := cmd.Flags().GetString("stack")
		require.NoError(t, err)
		assert.Equal(t, "dev-us-west-2", val)
	})

	t.Run("can set and get file flag", func(t *testing.T) {
		err := cmd.Flags().Set("file", "output.tfvars")
		require.NoError(t, err)

		val, err := cmd.Flags().GetString("file")
		require.NoError(t, err)
		assert.Equal(t, "output.tfvars", val)
	})
}
