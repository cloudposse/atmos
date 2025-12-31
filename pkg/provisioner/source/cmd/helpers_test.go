package cmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestParseCommonFlags_MissingStack tests that ParseCommonFlags returns an error when --stack is not provided.
func TestParseCommonFlags_MissingStack(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	parser := flags.NewStandardParser(
		flags.WithStringFlag("stack", "s", "", "Stack name"),
		flags.WithStringFlag("identity", "", "", "Identity"),
		flags.WithBoolFlag("force", "f", false, "Force"),
	)
	parser.RegisterFlags(cmd)

	// Don't set --stack flag.
	opts, err := ParseCommonFlags(cmd, parser)

	require.Error(t, err)
	assert.Nil(t, opts)
	assert.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
}

// TestParseCommonFlags_WithStack tests that ParseCommonFlags works when --stack is provided.
func TestParseCommonFlags_WithStack(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	parser := flags.NewStandardParser(
		flags.WithStringFlag("stack", "s", "", "Stack name"),
		flags.WithStringFlag("identity", "", "", "Identity"),
		flags.WithBoolFlag("force", "f", false, "Force"),
	)
	parser.RegisterFlags(cmd)

	// Parse flags.
	err := cmd.ParseFlags([]string{"--stack", "dev-us-east-1"})
	require.NoError(t, err)

	opts, err := ParseCommonFlags(cmd, parser)

	require.NoError(t, err)
	require.NotNil(t, opts)
	assert.Equal(t, "dev-us-east-1", opts.Stack)
}

// TestParseCommonFlags_WithAllFlags tests parsing all common flags.
func TestParseCommonFlags_WithAllFlags(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}

	parser := flags.NewStandardParser(
		flags.WithStringFlag("stack", "s", "", "Stack name"),
		flags.WithStringFlag("identity", "", "", "Identity"),
		flags.WithBoolFlag("force", "f", false, "Force"),
	)
	parser.RegisterFlags(cmd)

	err := cmd.ParseFlags([]string{"--stack", "prod-us-west-2", "--identity", "my-identity", "--force"})
	require.NoError(t, err)

	opts, err := ParseCommonFlags(cmd, parser)

	require.NoError(t, err)
	require.NotNil(t, opts)
	assert.Equal(t, "prod-us-west-2", opts.Stack)
	assert.Equal(t, "my-identity", opts.Identity)
	assert.True(t, opts.Force)
}

// TestProvisionSource_NoSource tests that ProvisionSource returns nil when no source is configured.
func TestProvisionSource_NoSource(t *testing.T) {
	ctx := context.Background()

	// Create a temp directory for the base path.
	tempDir := t.TempDir()

	opts := &ProvisionSourceOptions{
		AtmosConfig: &schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: tempDir,
				},
			},
		},
		ComponentType:   "terraform",
		Component:       "vpc",
		Stack:           "dev",
		ComponentConfig: map[string]any{}, // No source configured.
		AuthContext:     nil,
		Force:           false,
	}

	err := ProvisionSource(ctx, opts)
	assert.NoError(t, err, "ProvisionSource should return nil when no source is configured")
}

// TestProvisionSource_TargetExists tests that ProvisionSource skips when target exists and force=false.
func TestProvisionSource_TargetExists(t *testing.T) {
	ctx := context.Background()

	// Create a temp directory with existing component.
	tempDir := t.TempDir()
	componentDir := filepath.Join(tempDir, "vpc")
	err := os.MkdirAll(componentDir, 0o755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(componentDir, "main.tf"), []byte("# existing"), 0o644)
	require.NoError(t, err)

	opts := &ProvisionSourceOptions{
		AtmosConfig: &schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{
					BasePath: tempDir,
				},
			},
		},
		ComponentType: "terraform",
		Component:     "vpc",
		Stack:         "dev",
		ComponentConfig: map[string]any{
			"source": map[string]any{
				"uri": "github.com/example/terraform-aws-vpc",
			},
		},
		AuthContext: nil,
		Force:       false, // Not forcing.
	}

	err = ProvisionSource(ctx, opts)
	assert.NoError(t, err, "ProvisionSource should skip when target exists and force=false")

	// Verify existing file was not modified.
	content, err := os.ReadFile(filepath.Join(componentDir, "main.tf"))
	require.NoError(t, err)
	assert.Equal(t, "# existing", string(content))
}

// TestInitConfigAndAuth_ConfigError tests that InitConfigAndAuth returns error when config init fails.
func TestInitConfigAndAuth_ConfigError(t *testing.T) {
	ctrl := gomock.NewController(t)

	// Save original and restore after test.
	origFunc := initCliConfigFunc
	defer func() { initCliConfigFunc = origFunc }()

	// Create mock and set expectation.
	mockLoader := NewMockConfigLoader(ctrl)
	mockLoader.EXPECT().
		InitCliConfig(gomock.Any(), gomock.Any()).
		Return(schema.AtmosConfiguration{}, errors.New("mock config error"))

	// Wire mock to function variable.
	initCliConfigFunc = mockLoader.InitCliConfig

	atmosConfig, authContext, err := InitConfigAndAuth("vpc", "dev", "", nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToInitConfig)
	assert.Nil(t, atmosConfig)
	assert.Nil(t, authContext)
}

// TestInitConfigAndAuth_WithGlobalFlags tests that global flags are passed to config loader.
func TestInitConfigAndAuth_WithGlobalFlags(t *testing.T) {
	ctrl := gomock.NewController(t)

	// Save original and restore after test.
	origFunc := initCliConfigFunc
	defer func() { initCliConfigFunc = origFunc }()

	var capturedInfo schema.ConfigAndStacksInfo

	// Create mock and capture config info.
	mockLoader := NewMockConfigLoader(ctrl)
	mockLoader.EXPECT().
		InitCliConfig(gomock.Any(), gomock.Any()).
		DoAndReturn(func(configInfo schema.ConfigAndStacksInfo, validate bool) (schema.AtmosConfiguration, error) {
			capturedInfo = configInfo
			// Return error to short-circuit the rest of the function.
			return schema.AtmosConfiguration{}, errors.New("test stop")
		})

	// Wire mock to function variable.
	initCliConfigFunc = mockLoader.InitCliConfig

	globalFlags := &global.Flags{
		BasePath:   "/custom/base",
		Config:     []string{"/custom/config"},
		ConfigPath: []string{"/custom/config/path"},
		Profile:    []string{"test-profile"},
	}

	_, _, _ = InitConfigAndAuth("vpc", "dev", "", globalFlags)

	// Verify global flags were passed.
	assert.Equal(t, "/custom/base", capturedInfo.AtmosBasePath)
	assert.Equal(t, []string{"/custom/config"}, capturedInfo.AtmosConfigFilesFromArg)
	assert.Equal(t, []string{"/custom/config/path"}, capturedInfo.AtmosConfigDirsFromArg)
	assert.Equal(t, []string{"test-profile"}, capturedInfo.ProfilesFromArg)
	assert.Equal(t, "vpc", capturedInfo.ComponentFromArg)
	assert.Equal(t, "dev", capturedInfo.Stack)
}

// TestInitConfigAndAuth_DescribeComponentError tests that InitConfigAndAuth handles describe component errors.
func TestInitConfigAndAuth_DescribeComponentError(t *testing.T) {
	ctrl := gomock.NewController(t)

	// Save originals and restore after test.
	origInitFunc := initCliConfigFunc
	origDescribeFunc := describeComponentFunc
	defer func() {
		initCliConfigFunc = origInitFunc
		describeComponentFunc = origDescribeFunc
	}()

	// Create mocks.
	mockLoader := NewMockConfigLoader(ctrl)
	mockDescriber := NewMockComponentDescriber(ctrl)

	// Config init succeeds.
	mockLoader.EXPECT().
		InitCliConfig(gomock.Any(), gomock.Any()).
		Return(schema.AtmosConfiguration{}, nil)

	// Describe component fails.
	mockDescriber.EXPECT().
		DescribeComponent(gomock.Any(), gomock.Any()).
		Return(nil, errors.New("mock describe error"))

	// Wire mocks to function variables.
	initCliConfigFunc = mockLoader.InitCliConfig
	describeComponentFunc = mockDescriber.DescribeComponent

	atmosConfig, authContext, err := InitConfigAndAuth("vpc", "dev", "", nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load component config")
	assert.Nil(t, atmosConfig)
	assert.Nil(t, authContext)
}

// TestInitConfigAndAuth_AuthMergeError tests that InitConfigAndAuth handles auth merge errors.
func TestInitConfigAndAuth_AuthMergeError(t *testing.T) {
	ctrl := gomock.NewController(t)

	// Save originals and restore after test.
	origInitFunc := initCliConfigFunc
	origDescribeFunc := describeComponentFunc
	origMergeFunc := mergeAuthFunc
	defer func() {
		initCliConfigFunc = origInitFunc
		describeComponentFunc = origDescribeFunc
		mergeAuthFunc = origMergeFunc
	}()

	// Create mocks.
	mockLoader := NewMockConfigLoader(ctrl)
	mockDescriber := NewMockComponentDescriber(ctrl)
	mockMerger := NewMockAuthMerger(ctrl)

	// Config init succeeds.
	mockLoader.EXPECT().
		InitCliConfig(gomock.Any(), gomock.Any()).
		Return(schema.AtmosConfiguration{}, nil)

	// Describe component succeeds.
	mockDescriber.EXPECT().
		DescribeComponent(gomock.Any(), gomock.Any()).
		Return(map[string]any{"component": "vpc"}, nil)

	// Auth merge fails.
	mockMerger.EXPECT().
		MergeComponentAuth(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("mock merge error"))

	// Wire mocks to function variables.
	initCliConfigFunc = mockLoader.InitCliConfig
	describeComponentFunc = mockDescriber.DescribeComponent
	mergeAuthFunc = mockMerger.MergeComponentAuth

	atmosConfig, authContext, err := InitConfigAndAuth("vpc", "dev", "", nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to merge component auth")
	assert.Nil(t, atmosConfig)
	assert.Nil(t, authContext)
}

// TestInitConfigAndAuth_AuthCreateError tests that InitConfigAndAuth handles auth creation errors.
func TestInitConfigAndAuth_AuthCreateError(t *testing.T) {
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

	// Describe component succeeds.
	mockDescriber.EXPECT().
		DescribeComponent(gomock.Any(), gomock.Any()).
		Return(map[string]any{"component": "vpc"}, nil)

	// Auth merge succeeds.
	mockMerger.EXPECT().
		MergeComponentAuth(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&schema.AuthConfig{}, nil)

	// Auth create fails.
	mockCreator.EXPECT().
		CreateAuthManager(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errors.New("mock auth create error"))

	// Wire mocks to function variables.
	initCliConfigFunc = mockLoader.InitCliConfig
	describeComponentFunc = mockDescriber.DescribeComponent
	mergeAuthFunc = mockMerger.MergeComponentAuth
	createAuthFunc = mockCreator.CreateAuthManager

	atmosConfig, authContext, err := InitConfigAndAuth("vpc", "dev", "", nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "mock auth create error")
	assert.Nil(t, atmosConfig)
	assert.Nil(t, authContext)
}

// TestInitConfigAndAuth_Success tests the successful path of InitConfigAndAuth.
func TestInitConfigAndAuth_Success(t *testing.T) {
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
		Return(schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{BasePath: "components/terraform"},
			},
		}, nil)

	// Describe component succeeds.
	mockDescriber.EXPECT().
		DescribeComponent(gomock.Any(), gomock.Any()).
		Return(map[string]any{"component": "vpc"}, nil)

	// Auth merge succeeds.
	mockMerger.EXPECT().
		MergeComponentAuth(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&schema.AuthConfig{}, nil)

	// Auth create returns nil (no auth manager).
	mockCreator.EXPECT().
		CreateAuthManager(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil)

	// Wire mocks to function variables.
	initCliConfigFunc = mockLoader.InitCliConfig
	describeComponentFunc = mockDescriber.DescribeComponent
	mergeAuthFunc = mockMerger.MergeComponentAuth
	createAuthFunc = mockCreator.CreateAuthManager

	atmosConfig, authContext, err := InitConfigAndAuth("vpc", "dev", "", nil)

	require.NoError(t, err)
	require.NotNil(t, atmosConfig)
	assert.Equal(t, "components/terraform", atmosConfig.Components.Terraform.BasePath)
	assert.Nil(t, authContext) // No auth manager means no auth context.
}

// TestDescribeComponent tests the DescribeComponent wrapper function.
func TestDescribeComponent(t *testing.T) {
	ctrl := gomock.NewController(t)

	// Save original and restore after test.
	origDescribeFunc := describeComponentFunc
	defer func() { describeComponentFunc = origDescribeFunc }()

	// Create mock and set expectation.
	mockDescriber := NewMockComponentDescriber(ctrl)
	mockDescriber.EXPECT().
		DescribeComponent("vpc", "dev").
		Return(map[string]any{
			"component": "vpc",
			"stack":     "dev",
			"vars":      map[string]any{"foo": "bar"},
		}, nil)

	// Wire mock to function variable.
	describeComponentFunc = mockDescriber.DescribeComponent

	result, err := DescribeComponent("vpc", "dev")

	require.NoError(t, err)
	assert.Equal(t, "vpc", result["component"])
	assert.Equal(t, "dev", result["stack"])
}
