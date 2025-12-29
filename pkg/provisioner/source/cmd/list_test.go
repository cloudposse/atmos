package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
)

// TestListCommand tests that ListCommand creates a valid cobra command.
func TestListCommand(t *testing.T) {
	cfg := &Config{
		ComponentType: "terraform",
		TypeLabel:     "Terraform",
	}

	cmd := ListCommand(cfg)

	require.NotNil(t, cmd)
	assert.Equal(t, "list", cmd.Use)
	assert.Contains(t, cmd.Short, "Terraform")
}

// TestExecuteList_MissingStack tests that executeList returns error when --stack is not provided.
func TestExecuteList_MissingStack(t *testing.T) {
	cfg := &Config{
		ComponentType: "terraform",
		TypeLabel:     "Terraform",
	}

	cmd := &cobra.Command{Use: "test"}
	parser := flags.NewStandardParser(
		flags.WithStackFlag(),
	)
	parser.RegisterFlags(cmd)

	err := executeList(cmd, cfg, parser)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
}

// TestExecuteList_NotImplemented tests that executeList returns not implemented error.
func TestExecuteList_NotImplemented(t *testing.T) {
	cfg := &Config{
		ComponentType: "terraform",
		TypeLabel:     "Terraform",
	}

	cmd := &cobra.Command{Use: "test"}
	parser := flags.NewStandardParser(
		flags.WithStackFlag(),
	)
	parser.RegisterFlags(cmd)

	err := cmd.ParseFlags([]string{"--stack", "dev"})
	require.NoError(t, err)

	err = executeList(cmd, cfg, parser)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrNotImplemented)
}
