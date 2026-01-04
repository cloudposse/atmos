package cmd

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/provisioner/source"
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
	ctrl := gomock.NewController(t)

	// Save originals and restore after test.
	origInitFunc := initCliConfigFunc
	defer func() { initCliConfigFunc = origInitFunc }()

	// Create mock and set expectation.
	mockLoader := NewMockConfigLoader(ctrl)
	mockLoader.EXPECT().
		InitCliConfig(gomock.Any(), gomock.Any()).
		Return(schema.AtmosConfiguration{}, errors.New("mock config error"))

	// Wire mock to function variable.
	initCliConfigFunc = mockLoader.InitCliConfig

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
	ctrl := gomock.NewController(t)

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

	// Create mocks.
	mockLoader := NewMockConfigLoader(ctrl)
	mockDescriber := NewMockComponentDescriber(ctrl)
	mockMerger := NewMockAuthMerger(ctrl)
	mockCreator := NewMockAuthCreator(ctrl)

	// Config init succeeds.
	mockLoader.EXPECT().
		InitCliConfig(gomock.Any(), gomock.Any()).
		Return(schema.AtmosConfiguration{}, nil)

	// First describe call (in InitConfigAndAuth) succeeds, second fails.
	gomock.InOrder(
		mockDescriber.EXPECT().
			DescribeComponent(gomock.Any(), gomock.Any()).
			Return(map[string]any{}, nil),
		mockDescriber.EXPECT().
			DescribeComponent(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("mock describe error")),
	)

	// Auth merge succeeds.
	mockMerger.EXPECT().
		MergeComponentAuth(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&schema.AuthConfig{}, nil)

	// Auth create succeeds.
	mockCreator.EXPECT().
		CreateAuthManager(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil)

	// Wire mocks to function variables.
	initCliConfigFunc = mockLoader.InitCliConfig
	describeComponentFunc = mockDescriber.DescribeComponent
	mergeAuthFunc = mockMerger.MergeComponentAuth
	createAuthFunc = mockCreator.CreateAuthManager

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
	ctrl := gomock.NewController(t)

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

	// Create mocks.
	mockLoader := NewMockConfigLoader(ctrl)
	mockDescriber := NewMockComponentDescriber(ctrl)
	mockMerger := NewMockAuthMerger(ctrl)
	mockCreator := NewMockAuthCreator(ctrl)

	// Config init succeeds.
	mockLoader.EXPECT().
		InitCliConfig(gomock.Any(), gomock.Any()).
		Return(schema.AtmosConfiguration{}, nil)

	// Describe component returns config without source.
	mockDescriber.EXPECT().
		DescribeComponent(gomock.Any(), gomock.Any()).
		Return(map[string]any{"vars": map[string]any{"foo": "bar"}}, nil).
		Times(2) // Called in InitConfigAndAuth and executePull.

	// Auth merge succeeds.
	mockMerger.EXPECT().
		MergeComponentAuth(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&schema.AuthConfig{}, nil)

	// Auth create succeeds.
	mockCreator.EXPECT().
		CreateAuthManager(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil)

	// Wire mocks to function variables.
	initCliConfigFunc = mockLoader.InitCliConfig
	describeComponentFunc = mockDescriber.DescribeComponent
	mergeAuthFunc = mockMerger.MergeComponentAuth
	createAuthFunc = mockCreator.CreateAuthManager

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
	assert.ErrorIs(t, err, errUtils.ErrSourceMissing)
}

// TestExecutePull_Success tests the complete happy path for executePull.
func TestExecutePull_Success(t *testing.T) {
	ctrl := gomock.NewController(t)

	// Save originals and restore after test.
	origInitFunc := initCliConfigFunc
	origDescribeFunc := describeComponentFunc
	origMergeFunc := mergeAuthFunc
	origCreateFunc := createAuthFunc
	origProvisionFunc := provisionSourceFunc
	defer func() {
		initCliConfigFunc = origInitFunc
		describeComponentFunc = origDescribeFunc
		mergeAuthFunc = origMergeFunc
		createAuthFunc = origCreateFunc
		provisionSourceFunc = origProvisionFunc
	}()

	// Create mocks.
	mockLoader := NewMockConfigLoader(ctrl)
	mockDescriber := NewMockComponentDescriber(ctrl)
	mockMerger := NewMockAuthMerger(ctrl)
	mockCreator := NewMockAuthCreator(ctrl)
	mockProvisioner := NewMockSourceProvisioner(ctrl)

	// Config init succeeds with valid config.
	mockLoader.EXPECT().
		InitCliConfig(gomock.Any(), gomock.Any()).
		Return(schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: "components/terraform",
				},
			},
		}, nil)

	// Describe component returns config with source.
	componentConfig := map[string]any{
		"source": map[string]any{
			"uri":     "github.com/cloudposse/terraform-null-label//exports",
			"version": "0.25.0",
		},
		"vars": map[string]any{"foo": "bar"},
	}
	mockDescriber.EXPECT().
		DescribeComponent(gomock.Any(), gomock.Any()).
		Return(componentConfig, nil).
		Times(2) // Called in InitConfigAndAuth and executePull.

	// Auth merge succeeds.
	mockMerger.EXPECT().
		MergeComponentAuth(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&schema.AuthConfig{}, nil)

	// Auth create succeeds.
	mockCreator.EXPECT().
		CreateAuthManager(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil)

	// Capture provisioner params for verification.
	var capturedParams *source.ProvisionParams
	mockProvisioner.EXPECT().
		Provision(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, params *source.ProvisionParams) error {
			capturedParams = params
			return nil
		})

	// Wire mocks to function variables.
	initCliConfigFunc = mockLoader.InitCliConfig
	describeComponentFunc = mockDescriber.DescribeComponent
	mergeAuthFunc = mockMerger.MergeComponentAuth
	createAuthFunc = mockCreator.CreateAuthManager
	provisionSourceFunc = mockProvisioner.Provision

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

	err := cmd.ParseFlags([]string{"--stack", "dev", "--force"})
	require.NoError(t, err)

	args := []string{"vpc"}

	err = executePull(cmd, args, cfg, parser)

	require.NoError(t, err)
	require.NotNil(t, capturedParams, "ProvisionSource should have been called")
	assert.Equal(t, "terraform", capturedParams.ComponentType)
	assert.Equal(t, "vpc", capturedParams.Component)
	assert.Equal(t, "dev", capturedParams.Stack)
	assert.True(t, capturedParams.Force)
	assert.NotNil(t, capturedParams.ComponentConfig)
	assert.NotNil(t, capturedParams.ComponentConfig["source"])
}
