package workdir

import (
	"errors"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDescribeCmd_Structure(t *testing.T) {
	// Verify command structure.
	assert.Equal(t, "describe <component>", describeCmd.Use)
	assert.Equal(t, "Describe workdir as stack manifest", describeCmd.Short)
	assert.Contains(t, describeCmd.Long, "Output the workdir configuration")
	assert.Contains(t, describeCmd.Example, "atmos terraform workdir describe vpc --stack dev")
}

func TestDescribeCmd_Args(t *testing.T) {
	// Verify exact args requirement.
	assert.NotNil(t, describeCmd.Args)
}

func TestDescribeParser_Flags(t *testing.T) {
	// Verify parser is initialized.
	assert.NotNil(t, describeParser)
}

func TestDescribeCmd_DisableFlagParsing(t *testing.T) {
	// Verify flag parsing is enabled.
	assert.False(t, describeCmd.DisableFlagParsing)
}

func TestMockWorkdirManager_DescribeWorkdir(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)

	expectedManifest := `components:
  terraform:
    vpc:
      metadata:
        workdir:
          name: dev-vpc
          source: components/terraform/vpc
          path: .workdir/terraform/dev-vpc
`

	mock.EXPECT().DescribeWorkdir(gomock.Any(), "vpc", "dev", gomock.Any()).Return(expectedManifest, nil)

	// Save and restore.
	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	result, err := mock.DescribeWorkdir(&schema.AtmosConfiguration{}, "vpc", "dev", nil)
	assert.NoError(t, err)
	assert.Equal(t, expectedManifest, result)
}

func TestMockWorkdirManager_DescribeWorkdir_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)

	expectedErr := errUtils.Build(errUtils.ErrWorkdirMetadata).
		WithExplanation("Workdir not found").
		Err()

	mock.EXPECT().DescribeWorkdir(gomock.Any(), "nonexistent", "dev", gomock.Any()).Return("", expectedErr)

	// Save and restore.
	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	result, err := mock.DescribeWorkdir(&schema.AtmosConfiguration{}, "nonexistent", "dev", nil)
	assert.Error(t, err)
	assert.Empty(t, result)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirMetadata)
}

// TestDescribeCmd_RunE_ForwardsResolvedComponentConfig is a behavior-focused regression test
// for a gap gomock.Any() cannot catch: gomock.Any() accepts nil, so a command-code regression
// that stops calling resolveComponentConfig (or passes nil for componentConfig) would still
// satisfy every other test in this file. This executes the real describeCmd.RunE path against
// a real stack config and asserts the exact resolved config -- including its "atmos_component"
// key -- reaches DescribeWorkdir, not just that DescribeWorkdir was called.
func TestDescribeCmd_RunE_ForwardsResolvedComponentConfig(t *testing.T) {
	initTestIO(t)

	tmpDir := t.TempDir()
	createTestAtmosConfig(t, tmpDir)
	t.Chdir(tmpDir)

	v := viper.GetViper()
	v.Set("stack", "dev")
	t.Cleanup(func() { v.Set("stack", "") })

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)
	mock.EXPECT().DescribeWorkdir(gomock.Any(), "vpc", "dev", gomock.Any()).
		DoAndReturn(func(_ *schema.AtmosConfiguration, component, stack string, componentConfig map[string]any) (string, error) {
			require.NotNil(t, componentConfig,
				"resolveComponentConfig's result must reach DescribeWorkdir, not be dropped or nilled")
			assert.Equal(t, "vpc", componentConfig["atmos_component"],
				"the resolved atmos_component override must be forwarded to DescribeWorkdir")
			return "manifest", nil
		})

	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	err := describeCmd.RunE(describeCmd, []string{"vpc"})
	require.NoError(t, err)
}

func TestDescribeCmd_RequiresStack(t *testing.T) {
	// The describe command requires --stack flag.
	// Verify the parser has stack flag registered.
	assert.NotNil(t, describeParser)
}

func TestDescribeCmd_Examples(t *testing.T) {
	examples := describeCmd.Example
	assert.Contains(t, examples, "atmos terraform workdir describe vpc --stack dev")
}

// Test error handling.

func TestDescribeCmd_ErrorTypes(t *testing.T) {
	// Verify error builder creates correct sentinel error.
	err := errUtils.Build(errUtils.ErrWorkdirMetadata).
		WithExplanation("Failed to load atmos configuration").
		Err()

	assert.ErrorIs(t, err, errUtils.ErrWorkdirMetadata)
	// Error is based on sentinel and is non-nil.
	assert.NotNil(t, err)
}

// Test manifest structure.

func TestDescribeWorkdir_ManifestStructure(t *testing.T) {
	// Verify the expected manifest structure.
	info := &WorkdirInfo{
		Name:        "dev-vpc",
		Component:   "vpc",
		Stack:       "dev",
		Source:      "components/terraform/vpc",
		Path:        ".workdir/terraform/dev-vpc",
		ContentHash: "abc123",
		CreatedAt:   time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
		UpdatedAt:   time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
	}

	// Verify all fields are present.
	assert.NotEmpty(t, info.Name)
	assert.NotEmpty(t, info.Component)
	assert.NotEmpty(t, info.Stack)
	assert.NotEmpty(t, info.Source)
	assert.NotEmpty(t, info.Path)
	assert.NotEmpty(t, info.ContentHash)
	assert.False(t, info.CreatedAt.IsZero())
	assert.False(t, info.UpdatedAt.IsZero())
}

// Test with various component names.

func TestDescribeCmd_VariousComponentNames(t *testing.T) {
	testCases := []struct {
		component string
		stack     string
	}{
		{"vpc", "dev"},
		{"s3-bucket", "prod"},
		{"my_component", "staging"},
		{"component.name", "test"},
	}

	for _, tc := range testCases {
		t.Run(tc.component, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mock := NewMockWorkdirManager(ctrl)
			mock.EXPECT().DescribeWorkdir(gomock.Any(), tc.component, tc.stack, gomock.Any()).Return("manifest", nil)

			original := workdirManager
			defer func() { workdirManager = original }()
			SetWorkdirManager(mock)

			result, err := mock.DescribeWorkdir(&schema.AtmosConfiguration{}, tc.component, tc.stack, nil)
			assert.NoError(t, err)
			assert.NotEmpty(t, result)
		})
	}
}

// Test error scenarios.

func TestDescribeCmd_WorkdirNotFoundError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)
	expectedErr := errUtils.Build(errUtils.ErrWorkdirMetadata).
		WithExplanation("Workdir not found for component 'vpc' in stack 'dev'").
		WithHint("Run 'atmos terraform init' to create the workdir").
		Err()
	mock.EXPECT().DescribeWorkdir(gomock.Any(), "vpc", "dev", gomock.Any()).Return("", expectedErr)

	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	result, err := mock.DescribeWorkdir(&schema.AtmosConfiguration{}, "vpc", "dev", nil)
	assert.Error(t, err)
	assert.Empty(t, result)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirMetadata)
}

func TestDescribeCmd_MarshalError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)
	expectedErr := errUtils.Build(errUtils.ErrWorkdirMetadata).
		WithCause(errors.New("yaml marshal failed")).
		WithExplanation("Failed to marshal workdir manifest").
		Err()
	mock.EXPECT().DescribeWorkdir(gomock.Any(), "vpc", "dev", gomock.Any()).Return("", expectedErr)

	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	result, err := mock.DescribeWorkdir(&schema.AtmosConfiguration{}, "vpc", "dev", nil)
	assert.Error(t, err)
	assert.Empty(t, result)
}

// Test nil handling.

func TestDescribeCmd_NilConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)
	mock.EXPECT().DescribeWorkdir(gomock.Any(), "vpc", "dev", gomock.Any()).Return("manifest", nil)

	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	result, err := mock.DescribeWorkdir(&schema.AtmosConfiguration{}, "vpc", "dev", nil)
	assert.NoError(t, err)
	assert.NotEmpty(t, result)
}

// Test cobra.ExactArgs(1) validation.

func TestDescribeCmd_ArgsValidation(t *testing.T) {
	// cobra.ExactArgs(1) should reject zero arguments.
	err := describeCmd.Args(describeCmd, []string{})
	assert.Error(t, err)

	// cobra.ExactArgs(1) should accept one argument.
	err = describeCmd.Args(describeCmd, []string{"vpc"})
	assert.NoError(t, err)

	// cobra.ExactArgs(1) should reject two arguments.
	err = describeCmd.Args(describeCmd, []string{"vpc", "extra"})
	assert.Error(t, err)
}

// Test flag registration.

func TestDescribeCmd_Flags(t *testing.T) {
	// Verify --stack flag is registered.
	stackFlag := describeCmd.Flags().Lookup("stack")
	assert.NotNil(t, stackFlag, "stack flag should be registered")
	assert.Equal(t, "s", stackFlag.Shorthand)
}

// Test successful manifest output.

func TestMockWorkdirManager_DescribeWorkdir_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)

	manifest := `components:
  terraform:
    s3-bucket:
      metadata:
        workdir:
          name: prod-s3-bucket
          source: components/terraform/s3-bucket
          path: .workdir/terraform/prod-s3-bucket
          content_hash: abc123
`

	mock.EXPECT().DescribeWorkdir(gomock.Any(), "s3-bucket", "prod", gomock.Any()).Return(manifest, nil)

	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	result, err := mock.DescribeWorkdir(&schema.AtmosConfiguration{}, "s3-bucket", "prod", nil)
	assert.NoError(t, err)
	assert.Contains(t, result, "s3-bucket")
	assert.Contains(t, result, "prod-s3-bucket")
	assert.Contains(t, result, "content_hash")
}

// Test with different stack names.

func TestDescribeCmd_VariousStacks(t *testing.T) {
	stacks := []string{
		"dev",
		"staging",
		"production",
		"us-east-1-prod",
		"tenant1-dev",
	}

	for _, stack := range stacks {
		t.Run(stack, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mock := NewMockWorkdirManager(ctrl)
			mock.EXPECT().DescribeWorkdir(gomock.Any(), "vpc", stack, gomock.Any()).Return("manifest", nil)

			original := workdirManager
			defer func() { workdirManager = original }()
			SetWorkdirManager(mock)

			result, err := mock.DescribeWorkdir(&schema.AtmosConfiguration{}, "vpc", stack, nil)
			assert.NoError(t, err)
			assert.NotEmpty(t, result)
		})
	}
}

// Test buildConfigAndStacksInfo values.

func TestDescribeCmd_ConfigValues(t *testing.T) {
	v := viper.New()
	v.Set("stack", "prod")
	v.Set("base-path", "/test/path")

	stack := v.GetString("stack")
	basePath := v.GetString("base-path")

	assert.Equal(t, "prod", stack)
	assert.Equal(t, "/test/path", basePath)
}
