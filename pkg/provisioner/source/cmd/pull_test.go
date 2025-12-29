package cmd

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestPullCommand tests that PullCommand creates a valid cobra command.
func TestPullCommand(t *testing.T) {
	cfg := &Config{
		ComponentType: "terraform",
		TypeLabel:     "Terraform",
	}

	cmd := PullCommand(cfg)

	require.NotNil(t, cmd)
	assert.Equal(t, "pull <component>", cmd.Use)
	assert.Contains(t, cmd.Short, "Terraform")
}

// TestExecutePull_MissingStack tests that executePull returns error when --stack is not provided.
func TestExecutePull_MissingStack(t *testing.T) {
	cfg := &Config{
		ComponentType: "terraform",
		TypeLabel:     "Terraform",
	}

	cmd := &cobra.Command{Use: "test"}
	parser := flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
		flags.WithBoolFlag("force", "f", false, "Force"),
	)
	parser.RegisterFlags(cmd)

	args := []string{"vpc"}

	err := executePull(cmd, args, cfg, parser)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
}

// TestExecutePull_InitConfigError tests that executePull handles config init errors.
func TestExecutePull_InitConfigError(t *testing.T) {
	// Save originals and restore after test.
	origInitFunc := initCliConfigFunc
	defer func() { initCliConfigFunc = origInitFunc }()

	// Mock config init to fail.
	initCliConfigFunc = func(configInfo schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, errors.New("mock config error")
	}

	cfg := &Config{
		ComponentType: "terraform",
		TypeLabel:     "Terraform",
	}

	cmd := &cobra.Command{Use: "test"}
	parser := flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
		flags.WithBoolFlag("force", "f", false, "Force"),
	)
	parser.RegisterFlags(cmd)

	err := cmd.ParseFlags([]string{"--stack", "dev"})
	require.NoError(t, err)

	args := []string{"vpc"}

	err = executePull(cmd, args, cfg, parser)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToInitConfig)
}

// TestExecutePull_DescribeComponentError tests that executePull handles describe component errors.
func TestExecutePull_DescribeComponentError(t *testing.T) {
	// Save originals and restore after test.
	origInitFunc := initCliConfigFunc
	origDescribeFunc := describeComponentFunc
	origMergeFunc := mergeAuthFunc
	origCreateFunc := createAuthFunc
	defer func() {
		initCliConfigFunc = origInitFunc
		describeComponentFunc = origDescribeFunc
		mergeAuthFunc = origMergeFunc
		createAuthFunc = origCreateFunc
	}()

	// Mock config init to succeed.
	initCliConfigFunc = func(configInfo schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}

	// First describe call (in InitConfigAndAuth) succeeds.
	describeCallCount := 0
	describeComponentFunc = func(component, stack string) (map[string]any, error) {
		describeCallCount++
		if describeCallCount == 1 {
			// First call in InitConfigAndAuth.
			return map[string]any{}, nil
		}
		// Second call in executePull.
		return nil, errors.New("mock describe error")
	}

	// Mock auth merge to succeed.
	mergeAuthFunc = func(globalAuth *schema.AuthConfig, componentConfig map[string]any, atmosConfig *schema.AtmosConfiguration, sectionName string) (*schema.AuthConfig, error) {
		return &schema.AuthConfig{}, nil
	}

	// Mock auth create to succeed.
	createAuthFunc = func(identity string, authConfig *schema.AuthConfig, flagValue string) (auth.AuthManager, error) {
		return nil, nil
	}

	cfg := &Config{
		ComponentType: "terraform",
		TypeLabel:     "Terraform",
	}

	cmd := &cobra.Command{Use: "test"}
	parser := flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
		flags.WithBoolFlag("force", "f", false, "Force"),
	)
	parser.RegisterFlags(cmd)

	err := cmd.ParseFlags([]string{"--stack", "dev"})
	require.NoError(t, err)

	args := []string{"vpc"}

	err = executePull(cmd, args, cfg, parser)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDescribeComponent)
}

// TestExecutePull_NoSource tests that executePull returns error when no source is configured.
func TestExecutePull_NoSource(t *testing.T) {
	// Save originals and restore after test.
	origInitFunc := initCliConfigFunc
	origDescribeFunc := describeComponentFunc
	origMergeFunc := mergeAuthFunc
	origCreateFunc := createAuthFunc
	defer func() {
		initCliConfigFunc = origInitFunc
		describeComponentFunc = origDescribeFunc
		mergeAuthFunc = origMergeFunc
		createAuthFunc = origCreateFunc
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

	// Mock auth merge to succeed.
	mergeAuthFunc = func(globalAuth *schema.AuthConfig, componentConfig map[string]any, atmosConfig *schema.AtmosConfiguration, sectionName string) (*schema.AuthConfig, error) {
		return &schema.AuthConfig{}, nil
	}

	// Mock auth create to succeed.
	createAuthFunc = func(identity string, authConfig *schema.AuthConfig, flagValue string) (auth.AuthManager, error) {
		return nil, nil
	}

	cfg := &Config{
		ComponentType: "terraform",
		TypeLabel:     "Terraform",
	}

	cmd := &cobra.Command{Use: "test"}
	parser := flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
		flags.WithBoolFlag("force", "f", false, "Force"),
	)
	parser.RegisterFlags(cmd)

	err := cmd.ParseFlags([]string{"--stack", "dev"})
	require.NoError(t, err)

	args := []string{"vpc"}

	err = executePull(cmd, args, cfg, parser)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMetadataSourceMissing)
}
