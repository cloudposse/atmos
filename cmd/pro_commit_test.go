package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestProCommitCmd_Initialization(t *testing.T) {
	_ = NewTestKit(t)

	t.Run("command is properly initialized", func(t *testing.T) {
		assert.NotNil(t, proCommitCmd)
		assert.Equal(t, "commit", proCommitCmd.Use)
		assert.Contains(t, proCommitCmd.Short, "Commit changes")
		assert.False(t, proCommitCmd.FParseErrWhitelist.UnknownFlags)
	})

	t.Run("has required flags", func(t *testing.T) {
		messageFlag := proCommitCmd.Flags().Lookup("message")
		assert.NotNil(t, messageFlag)
		assert.Equal(t, "m", messageFlag.Shorthand)

		commentFlag := proCommitCmd.Flags().Lookup("comment")
		assert.NotNil(t, commentFlag)

		addFlag := proCommitCmd.Flags().Lookup("add")
		assert.NotNil(t, addFlag)

		allFlag := proCommitCmd.Flags().Lookup("all")
		assert.NotNil(t, allFlag)
		assert.Equal(t, "A", allFlag.Shorthand)
	})

	t.Run("message flag is required", func(t *testing.T) {
		messageFlag := proCommitCmd.Flags().Lookup("message")
		require.NotNil(t, messageFlag)

		// Verify the flag has the required annotation.
		annotations := messageFlag.Annotations
		_, hasRequired := annotations["cobra_annotation_bash_completion_one_required_flag"]
		assert.True(t, hasRequired, "message flag should be marked as required")
	})
}

func TestProCommitCmd_MutuallyExclusiveFlags(t *testing.T) {
	t.Run("add and all flags conflict", func(t *testing.T) {
		_ = NewTestKit(t)

		// Point at a valid atmos config fixture.
		t.Setenv("ATMOS_CLI_CONFIG_PATH", "../tests/fixtures/scenarios/atmos-pro")
		t.Setenv("ATMOS_BASE_PATH", "../tests/fixtures/scenarios/atmos-pro")

		// Set both conflicting flags.
		require.NoError(t, proCommitCmd.Flags().Set("message", "test"))
		require.NoError(t, proCommitCmd.Flags().Set("add", "*.tf"))
		require.NoError(t, proCommitCmd.Flags().Set("all", "true"))

		err := proCommitCmd.RunE(proCommitCmd, []string{})
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrStagingFlagConflict)
	})
}
