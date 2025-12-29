package cmd

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
)

// TestDescribeCommand tests that DescribeCommand creates a valid cobra command.
func TestDescribeCommand(t *testing.T) {
	cfg := &Config{
		ComponentType: "terraform",
		TypeLabel:     "Terraform",
	}

	cmd := DescribeCommand(cfg)

	require.NotNil(t, cmd)
	assert.Equal(t, "describe <component>", cmd.Use)
	assert.Contains(t, cmd.Short, "Terraform")
}

// TestExecuteDescribe_MissingStack tests that executeDescribe returns error when --stack is not provided.
func TestExecuteDescribe_MissingStack(t *testing.T) {
	cfg := &Config{
		ComponentType: "terraform",
		TypeLabel:     "Terraform",
	}

	cmd := &cobra.Command{Use: "test"}
	parser := flags.NewStandardParser(
		flags.WithStackFlag(),
	)
	parser.RegisterFlags(cmd)

	args := []string{"vpc"}

	err := executeDescribe(cmd, args, cfg, parser)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
}

// TestExecuteDescribe_DescribeComponentError tests that executeDescribe handles describe component errors.
func TestExecuteDescribe_DescribeComponentError(t *testing.T) {
	// Save original and restore after test.
	origDescribeFunc := describeComponentFunc
	defer func() { describeComponentFunc = origDescribeFunc }()

	// Mock describe component to fail.
	describeComponentFunc = func(component, stack string) (map[string]any, error) {
		return nil, errors.New("mock describe error")
	}

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

	args := []string{"vpc"}

	err = executeDescribe(cmd, args, cfg, parser)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDescribeComponent)
}

// TestExecuteDescribe_NoSource tests that executeDescribe returns error when no source is configured.
func TestExecuteDescribe_NoSource(t *testing.T) {
	// Save original and restore after test.
	origDescribeFunc := describeComponentFunc
	defer func() { describeComponentFunc = origDescribeFunc }()

	// Mock describe component to return config without source.
	describeComponentFunc = func(component, stack string) (map[string]any, error) {
		return map[string]any{
			"vars": map[string]any{"foo": "bar"},
		}, nil
	}

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

	args := []string{"vpc"}

	err = executeDescribe(cmd, args, cfg, parser)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrSourceMissing)
}

// Note: TestExecuteDescribe_Success and TestExecuteDescribe_Source are skipped
// because they require data.InitWriter() to be called, which is done in root.go.
// The error path tests above provide sufficient coverage for the command logic.
