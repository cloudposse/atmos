package pro

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestProCommitCmd_Initialization(t *testing.T) {
	t.Run("command is properly initialized", func(t *testing.T) {
		assert.NotNil(t, commitCmd)
		assert.Equal(t, "commit", commitCmd.Use)
		assert.Contains(t, commitCmd.Short, "Commit changes")
		assert.False(t, commitCmd.FParseErrWhitelist.UnknownFlags)
	})

	t.Run("has required flags", func(t *testing.T) {
		messageFlag := commitCmd.Flags().Lookup("message")
		assert.NotNil(t, messageFlag)
		assert.Equal(t, "m", messageFlag.Shorthand)

		commentFlag := commitCmd.Flags().Lookup("comment")
		assert.NotNil(t, commentFlag)

		addFlag := commitCmd.Flags().Lookup("add")
		assert.NotNil(t, addFlag)

		allFlag := commitCmd.Flags().Lookup("all")
		assert.NotNil(t, allFlag)
		assert.Equal(t, "A", allFlag.Shorthand)
	})

	t.Run("message flag is required", func(t *testing.T) {
		messageFlag := commitCmd.Flags().Lookup("message")
		require.NotNil(t, messageFlag)

		// Verify the flag has the required annotation.
		annotations := messageFlag.Annotations
		_, hasRequired := annotations["cobra_annotation_bash_completion_one_required_flag"]
		assert.True(t, hasRequired, "message flag should be marked as required")
	})
}

func TestProCommitCmd_MutuallyExclusiveFlags(t *testing.T) {
	t.Run("add and all flags conflict", func(t *testing.T) {
		// Point at a valid atmos config fixture.
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "../../tests/fixtures/scenarios/atmos-pro")
		t.Setenv("ATMOS_BASE_PATH", "../../tests/fixtures/scenarios/atmos-pro")

		// Set both conflicting flags.
		require.NoError(t, commitCmd.Flags().Set("message", "test"))
		require.NoError(t, commitCmd.Flags().Set("add", "*.tf"))
		require.NoError(t, commitCmd.Flags().Set("all", "true"))

		err := commitCmd.RunE(commitCmd, []string{})
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrStagingFlagConflict)
	})
}
