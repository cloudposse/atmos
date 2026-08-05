package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDescribeCommand tests that DescribeCommand creates a valid cobra command.
func TestDescribeCommand(t *testing.T) {
	cfg := &Config{
		ComponentType: "terraform",
		TypeLabel:     "Terraform",
	}

	cmd := DescribeCommand(cfg)

	require.NotNil(t, cmd)
	assert.Equal(t, "describe [component]", cmd.Use)
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

func TestExecuteDescribe_SuccessUsesFormattedYAMLPrinter(t *testing.T) {
	origDescribeFunc := describeComponentFunc
	origInitCliConfigFunc := initCliConfigFunc
	defer func() {
		describeComponentFunc = origDescribeFunc
		initCliConfigFunc = origInitCliConfigFunc
	}()

	describeComponentFunc = func(component, stack string) (map[string]any, error) {
		assert.Equal(t, "weather", component)
		assert.Equal(t, "dev", stack)
		return map[string]any{
			"source": map[string]any{
				"uri":     "file://../components/weather",
				"version": "v1.0.0",
			},
		}, nil
	}
	initCliConfigFunc = func(info schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
		assert.Equal(t, "weather", info.ComponentFromArg)
		assert.Equal(t, "dev", info.Stack)
		assert.False(t, processStacks)
		return schema.AtmosConfiguration{}, nil
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

	output := captureDescribeStdout(t, func() {
		err = executeDescribe(cmd, []string{"weather"}, cfg, parser)
	})

	require.NoError(t, err)
	assert.Contains(t, output, "components:")
	assert.Contains(t, output, "terraform:")
	assert.Contains(t, output, "weather:")
	assert.Contains(t, output, "source:")
}

func captureDescribeStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = writer
	t.Cleanup(func() { os.Stdout = old })

	fn()

	require.NoError(t, writer.Close())
	os.Stdout = old

	var buffer bytes.Buffer
	_, err = io.Copy(&buffer, reader)
	require.NoError(t, err)
	return buffer.String()
}
