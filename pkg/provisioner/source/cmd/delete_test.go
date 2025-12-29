package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDeleteCommand tests that DeleteCommand creates a valid cobra command.
func TestDeleteCommand(t *testing.T) {
	cfg := &Config{
		ComponentType: "terraform",
		TypeLabel:     "Terraform",
	}

	cmd := DeleteCommand(cfg)

	require.NotNil(t, cmd)
	assert.Equal(t, "delete <component>", cmd.Use)
	assert.Contains(t, cmd.Short, "Terraform")
}

// TestParseDeleteFlags_MissingStack tests that parseDeleteFlags returns error when --stack is not provided.
func TestParseDeleteFlags_MissingStack(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	parser := flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithBoolFlag("force", "f", false, "Force deletion"),
	)
	parser.RegisterFlags(cmd)

	opts, err := parseDeleteFlags(cmd, parser)

	require.Error(t, err)
	assert.Nil(t, opts)
	assert.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
}

// TestParseDeleteFlags_MissingForce tests that parseDeleteFlags returns error when --force is not provided.
func TestParseDeleteFlags_MissingForce(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	parser := flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithBoolFlag("force", "f", false, "Force deletion"),
	)
	parser.RegisterFlags(cmd)

	err := cmd.ParseFlags([]string{"--stack", "dev"})
	require.NoError(t, err)

	opts, err := parseDeleteFlags(cmd, parser)

	require.Error(t, err)
	assert.Nil(t, opts)
	assert.ErrorIs(t, err, errUtils.ErrForceRequired)
}

// TestParseDeleteFlags_Success tests that parseDeleteFlags works with valid flags.
func TestParseDeleteFlags_Success(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	parser := flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithBoolFlag("force", "f", false, "Force deletion"),
	)
	parser.RegisterFlags(cmd)

	err := cmd.ParseFlags([]string{"--stack", "dev", "--force"})
	require.NoError(t, err)

	opts, err := parseDeleteFlags(cmd, parser)

	require.NoError(t, err)
	require.NotNil(t, opts)
	assert.Equal(t, "dev", opts.Stack)
}

// TestDeleteSourceDirectory_DirectoryNotExist tests that deleteSourceDirectory handles non-existent directory.
func TestDeleteSourceDirectory_DirectoryNotExist(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: tempDir,
			},
		},
	}

	componentConfig := map[string]any{
		"source": map[string]any{
			"uri": "github.com/example/vpc",
		},
	}

	// Target directory does not exist.
	err := deleteSourceDirectory(atmosConfig, "terraform", "nonexistent", componentConfig)

	// Should return nil (no error, just a warning).
	assert.NoError(t, err)
}

// TestDeleteSourceDirectory_Success tests that deleteSourceDirectory deletes existing directory.
func TestDeleteSourceDirectory_Success(t *testing.T) {
	tempDir := t.TempDir()

	// Create target directory with a file.
	targetDir := filepath.Join(tempDir, "vpc")
	err := os.MkdirAll(targetDir, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(targetDir, "main.tf"), []byte("# test"), 0o644)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: tempDir,
			},
		},
	}

	componentConfig := map[string]any{
		"source": map[string]any{
			"uri": "github.com/example/vpc",
		},
	}

	err = deleteSourceDirectory(atmosConfig, "terraform", "vpc", componentConfig)

	require.NoError(t, err)

	// Verify directory was deleted.
	_, err = os.Stat(targetDir)
	assert.True(t, os.IsNotExist(err), "Directory should be deleted")
}

// TestInitDeleteContext_NoSource tests that initDeleteContext returns error when no source is configured.
func TestInitDeleteContext_NoSource(t *testing.T) {
	// Save originals and restore after test.
	origInitFunc := initCliConfigFunc
	origDescribeFunc := describeComponentFunc
	defer func() {
		initCliConfigFunc = origInitFunc
		describeComponentFunc = origDescribeFunc
	}()

	// Mock config init to succeed.
	initCliConfigFunc = func(configInfo schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}

	// Mock describe component to return config without source.
	describeComponentFunc = func(component, stack string) (map[string]any, error) {
		return map[string]any{
			"vars": map[string]any{"foo": "bar"},
		}, nil
	}

	atmosConfig, componentConfig, err := initDeleteContext("vpc", "dev", nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMetadataSourceMissing)
	assert.Nil(t, atmosConfig)
	assert.Nil(t, componentConfig)
}

// TestInitDeleteContext_Success tests successful initialization.
func TestInitDeleteContext_Success(t *testing.T) {
	// Save originals and restore after test.
	origInitFunc := initCliConfigFunc
	origDescribeFunc := describeComponentFunc
	defer func() {
		initCliConfigFunc = origInitFunc
		describeComponentFunc = origDescribeFunc
	}()

	// Mock config init to succeed.
	initCliConfigFunc = func(configInfo schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{BasePath: "components/terraform"},
			},
		}, nil
	}

	// Mock describe component to return config with source.
	describeComponentFunc = func(component, stack string) (map[string]any, error) {
		return map[string]any{
			"source": map[string]any{
				"uri": "github.com/example/vpc",
			},
		}, nil
	}

	atmosConfig, componentConfig, err := initDeleteContext("vpc", "dev", nil)

	require.NoError(t, err)
	require.NotNil(t, atmosConfig)
	require.NotNil(t, componentConfig)
	assert.Equal(t, "components/terraform", atmosConfig.Components.Terraform.BasePath)
}

// Note: initDeleteContext uses cfg.InitCliConfig directly (not the mock function),
// so testing with real config is done via integration tests.
// The error paths are covered by the TestInitDeleteContext_NoSource and TestInitDeleteContext_Success tests
// which mock describeComponentFunc.

// TestExecuteDelete_MissingStack tests that executeDelete returns error when --stack is not provided.
func TestExecuteDelete_MissingStack(t *testing.T) {
	cfg := &Config{
		ComponentType: "terraform",
		TypeLabel:     "Terraform",
	}

	cmd := &cobra.Command{Use: "test"}
	parser := flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithBoolFlag("force", "f", false, "Force"),
	)
	parser.RegisterFlags(cmd)

	args := []string{"vpc"}

	err := executeDelete(cmd, args, cfg, parser)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
}

// TestExecuteDelete_MissingForce tests that executeDelete returns error when --force is not provided.
func TestExecuteDelete_MissingForce(t *testing.T) {
	cfg := &Config{
		ComponentType: "terraform",
		TypeLabel:     "Terraform",
	}

	cmd := &cobra.Command{Use: "test"}
	parser := flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithBoolFlag("force", "f", false, "Force"),
	)
	parser.RegisterFlags(cmd)

	err := cmd.ParseFlags([]string{"--stack", "dev"})
	require.NoError(t, err)

	args := []string{"vpc"}

	err = executeDelete(cmd, args, cfg, parser)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrForceRequired)
}
