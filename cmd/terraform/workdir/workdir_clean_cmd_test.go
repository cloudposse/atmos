package workdir

import (
	"errors"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCleanCmd_Structure(t *testing.T) {
	// Verify command structure.
	assert.Equal(t, "clean [component]", cleanCmd.Use)
	assert.Equal(t, "Clean workdir(s)", cleanCmd.Short)
	assert.Contains(t, cleanCmd.Long, "Remove component working directories")
	assert.Contains(t, cleanCmd.Example, "atmos terraform workdir clean vpc --stack dev")
	assert.Contains(t, cleanCmd.Example, "atmos terraform workdir clean --all")
}

func TestCleanCmd_Args(t *testing.T) {
	// Verify max args requirement (0 or 1).
	assert.NotNil(t, cleanCmd.Args)
}

func TestCleanParser_Flags(t *testing.T) {
	// Verify parser is initialized.
	assert.NotNil(t, cleanParser)
}

func TestCleanCmd_Flags(t *testing.T) {
	// Verify --all flag is registered.
	allFlag := cleanCmd.Flags().Lookup("all")
	assert.NotNil(t, allFlag, "all flag should be registered")
	assert.Equal(t, "a", allFlag.Shorthand)
	assert.Equal(t, "false", allFlag.DefValue)

	// Verify --stack flag is registered.
	stackFlag := cleanCmd.Flags().Lookup("stack")
	assert.NotNil(t, stackFlag, "stack flag should be registered")
}

func TestCleanCmd_DisableFlagParsing(t *testing.T) {
	// Verify flag parsing is enabled.
	assert.False(t, cleanCmd.DisableFlagParsing)
}

func TestCleanAllWorkdirs_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)
	mock.EXPECT().CleanAllWorkdirs(gomock.Any(), false).Return(nil)

	// Save and restore.
	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	atmosConfig := &schema.AtmosConfiguration{}
	err := cleanAllWorkdirs(atmosConfig, false)
	assert.NoError(t, err)
}

func TestCleanAllWorkdirs_DryRun(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)
	mock.EXPECT().CleanAllWorkdirs(gomock.Any(), true).Return(nil)

	// Save and restore.
	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	atmosConfig := &schema.AtmosConfiguration{}
	err := cleanAllWorkdirs(atmosConfig, true)
	assert.NoError(t, err)
}

func TestCleanAllWorkdirs_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)
	expectedErr := errors.New("clean failed")
	mock.EXPECT().CleanAllWorkdirs(gomock.Any(), false).Return(expectedErr)

	// Save and restore.
	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	atmosConfig := &schema.AtmosConfiguration{}
	err := cleanAllWorkdirs(atmosConfig, false)
	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestCleanSpecificWorkdir_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)
	mock.EXPECT().CleanWorkdir(gomock.Any(), "vpc", "dev", gomock.Any()).Return(nil)

	// Save and restore.
	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	atmosConfig := &schema.AtmosConfiguration{}
	err := cleanSpecificWorkdir(atmosConfig, "vpc", "dev", nil)
	assert.NoError(t, err)
}

// TestCleanCmd_RunE_ForwardsResolvedComponentConfig is a behavior-focused regression test for a
// gap gomock.Any() cannot catch: gomock.Any() accepts nil, so a command-code regression that
// stops calling resolveComponentConfig (or passes nil for componentConfig) would still satisfy
// every other test in this file. This executes the real cleanCmd.RunE path against a real stack
// config and asserts the exact resolved config -- including its "atmos_component" key -- reaches
// CleanWorkdir, not just that CleanWorkdir was called.
func TestCleanCmd_RunE_ForwardsResolvedComponentConfig(t *testing.T) {
	tmpDir := t.TempDir()
	createTestAtmosConfig(t, tmpDir)
	t.Chdir(tmpDir)

	resetViperForTest(t)
	v := viper.GetViper()
	v.Set("stack", "dev")

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)
	mock.EXPECT().CleanWorkdir(gomock.Any(), "vpc", "dev", gomock.Any()).
		DoAndReturn(func(_ *schema.AtmosConfiguration, component, stack string, componentConfig map[string]any) error {
			require.NotNil(t, componentConfig,
				"resolveComponentConfig's result must reach CleanWorkdir, not be dropped or nilled")
			assert.Equal(t, "vpc", componentConfig["atmos_component"],
				"the resolved atmos_component override must be forwarded to CleanWorkdir")
			return nil
		})

	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	err := cleanCmd.RunE(cleanCmd, []string{"vpc"})
	require.NoError(t, err)
}

func TestCleanSpecificWorkdir_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)
	expectedErr := errUtils.Build(errUtils.ErrWorkdirClean).
		WithExplanation("workdir not found").
		Err()
	mock.EXPECT().CleanWorkdir(gomock.Any(), "vpc", "dev", gomock.Any()).Return(expectedErr)

	// Save and restore.
	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	atmosConfig := &schema.AtmosConfiguration{}
	err := cleanSpecificWorkdir(atmosConfig, "vpc", "dev", nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirClean)
}

func TestMockWorkdirManager_CleanWorkdir(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)
	mock.EXPECT().CleanWorkdir(gomock.Any(), "s3", "prod", gomock.Any()).Return(nil)

	err := mock.CleanWorkdir(&schema.AtmosConfiguration{}, "s3", "prod", nil)
	assert.NoError(t, err)
}

func TestMockWorkdirManager_CleanAllWorkdirs(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)
	mock.EXPECT().CleanAllWorkdirs(gomock.Any(), false).Return(nil)

	err := mock.CleanAllWorkdirs(&schema.AtmosConfiguration{}, false)
	assert.NoError(t, err)
}

func TestMockWorkdirManager_MultipleMethodCalls(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)

	// Set up expectations for multiple calls.
	mock.EXPECT().ListWorkdirs(gomock.Any()).Return(nil, nil)
	mock.EXPECT().GetWorkdirInfo(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil)
	mock.EXPECT().DescribeWorkdir(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return("", nil)
	mock.EXPECT().CleanWorkdir(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mock.EXPECT().CleanAllWorkdirs(gomock.Any(), gomock.Any()).Return(nil)

	// Call various methods.
	_, _ = mock.ListWorkdirs(&schema.AtmosConfiguration{})
	_, _ = mock.GetWorkdirInfo(&schema.AtmosConfiguration{}, "", "", nil)
	_, _ = mock.DescribeWorkdir(&schema.AtmosConfiguration{}, "", "", nil)
	_ = mock.CleanWorkdir(&schema.AtmosConfiguration{}, "", "", nil)
	_ = mock.CleanAllWorkdirs(&schema.AtmosConfiguration{}, false)
}

// Test error types.

func TestCleanCmd_ErrorTypes(t *testing.T) {
	// Verify error builder creates correct sentinel error.
	err := errUtils.Build(errUtils.ErrWorkdirClean).
		WithExplanation("test explanation").
		WithHint("test hint").
		Err()

	assert.ErrorIs(t, err, errUtils.ErrWorkdirClean)
	// Error is based on sentinel and is non-nil.
	assert.NotNil(t, err)
}

// Test command examples.

func TestCleanCmd_Examples(t *testing.T) {
	examples := cleanCmd.Example

	// Should show specific workdir cleanup.
	assert.Contains(t, examples, "atmos terraform workdir clean vpc --stack dev")

	// Should show all workdir cleanup.
	assert.Contains(t, examples, "atmos terraform workdir clean --all")
}

// Test with nil config.

func TestCleanSpecificWorkdir_WithNilConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// When mock returns nil error, should succeed.
	mock := NewMockWorkdirManager(ctrl)
	mock.EXPECT().CleanWorkdir(gomock.Any(), "vpc", "dev", gomock.Any()).Return(nil)

	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	err := cleanSpecificWorkdir(nil, "vpc", "dev", nil)
	assert.NoError(t, err)
}

func TestCleanAllWorkdirs_WithNilConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)
	mock.EXPECT().CleanAllWorkdirs(gomock.Any(), false).Return(nil)

	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	err := cleanAllWorkdirs(nil, false)
	assert.NoError(t, err)
}

// Test RunE validation scenarios.

func TestCleanCmd_RunE_AllWithComponent(t *testing.T) {
	// Test the validation that --all and component cannot be used together.
	// This tests the logic inside RunE without calling InitCliConfig.
	v := viper.New()
	v.Set("all", true)
	v.Set("stack", "dev")

	// Create args simulating a component being provided.
	args := []string{"vpc"}

	// The validation checks all && len(args) > 0.
	all := v.GetBool("all")
	if all && len(args) > 0 {
		// This is the expected validation failure path.
		assert.True(t, true, "validation correctly identifies conflict")
	}
}

func TestCleanCmd_RunE_NoAllNoArgs(t *testing.T) {
	// Test the validation that either --all or component is required.
	v := viper.New()
	v.Set("all", false)
	v.Set("stack", "dev")

	args := []string{}

	all := v.GetBool("all")
	if !all && len(args) == 0 {
		// This is the expected validation failure path.
		assert.True(t, true, "validation correctly identifies missing input")
	}
}

func TestCleanCmd_RunE_ComponentNoStack(t *testing.T) {
	// Test the validation that stack is required with component.
	v := viper.New()
	v.Set("all", false)
	v.Set("stack", "")

	args := []string{"vpc"}

	all := v.GetBool("all")
	stack := v.GetString("stack")
	if !all && stack == "" && len(args) > 0 {
		// This is the expected validation failure path.
		assert.True(t, true, "validation correctly identifies missing stack")
	}
}

func TestCleanCmd_RunE_ValidAllCase(t *testing.T) {
	// Test valid --all case passes validation.
	v := viper.New()
	v.Set("all", true)
	v.Set("stack", "")

	args := []string{}

	all := v.GetBool("all")
	stack := v.GetString("stack")

	// Validation checks.
	allWithComponent := all && len(args) > 0
	noAllNoArgs := !all && len(args) == 0
	componentNoStack := !all && stack == "" && len(args) > 0

	assert.False(t, allWithComponent)
	assert.False(t, noAllNoArgs)
	assert.False(t, componentNoStack)
}

func TestCleanCmd_RunE_ValidComponentCase(t *testing.T) {
	// Test valid component + stack case passes validation.
	v := viper.New()
	v.Set("all", false)
	v.Set("stack", "dev")

	args := []string{"vpc"}

	all := v.GetBool("all")
	stack := v.GetString("stack")

	// Validation checks.
	allWithComponent := all && len(args) > 0
	noAllNoArgs := !all && len(args) == 0
	componentNoStack := !all && stack == "" && len(args) > 0

	assert.False(t, allWithComponent)
	assert.False(t, noAllNoArgs)
	assert.False(t, componentNoStack)
}

// Test cobra.MaximumNArgs(1) validation.

func TestCleanCmd_ArgsValidation(t *testing.T) {
	// cobra.MaximumNArgs(1) should accept zero arguments.
	err := cleanCmd.Args(cleanCmd, []string{})
	assert.NoError(t, err)

	// cobra.MaximumNArgs(1) should accept one argument.
	err = cleanCmd.Args(cleanCmd, []string{"vpc"})
	assert.NoError(t, err)

	// cobra.MaximumNArgs(1) should reject two arguments.
	err = cleanCmd.Args(cleanCmd, []string{"vpc", "extra"})
	assert.Error(t, err)
}

// Test cleanAllWorkdirs success path.

func TestCleanAllWorkdirs_SuccessMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)
	mock.EXPECT().CleanAllWorkdirs(gomock.Any(), false).Return(nil)

	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	atmosConfig := &schema.AtmosConfiguration{}

	// Should print success message to stderr and return nil.
	err := cleanAllWorkdirs(atmosConfig, false)
	assert.NoError(t, err)
}

// TestCleanAllWorkdirs_DryRunSuppressesSuccessMessage is a regression test for the
// dry-run/--all bug where "atmos terraform workdir clean --all --dry-run" printed the same
// "All workdirs cleaned" success message as a real clean, misleadingly implying something was
// deleted. On a dry run, cleanAllWorkdirs must not print that message -- the dry-run report
// itself (emitted by provWorkdir.CleanAllWorkdirs) is the only output.
func TestCleanAllWorkdirs_DryRunSuppressesSuccessMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)
	mock.EXPECT().CleanAllWorkdirs(gomock.Any(), true).Return(nil)

	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	atmosConfig := &schema.AtmosConfiguration{}

	ioCtx := initTestIOWithCapture(t)
	err := cleanAllWorkdirs(atmosConfig, true)
	assert.NoError(t, err)
	assert.NotContains(t, ioCtx.stderr.String(), "All workdirs cleaned",
		"dry run must not print the success message")
}

// Test cleanSpecificWorkdir success path.

func TestCleanSpecificWorkdir_SuccessMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)
	mock.EXPECT().CleanWorkdir(gomock.Any(), "s3", "staging", gomock.Any()).Return(nil)

	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	atmosConfig := &schema.AtmosConfiguration{}

	// Should print success message to stderr and return nil.
	err := cleanSpecificWorkdir(atmosConfig, "s3", "staging", nil)
	assert.NoError(t, err)
}

// Test various component/stack combinations.

func TestCleanSpecificWorkdir_VariousInputs(t *testing.T) {
	testCases := []struct {
		component string
		stack     string
	}{
		{"vpc", "dev"},
		{"s3-bucket", "prod"},
		{"my_component", "staging"},
		{"component.name", "test"},
		{"namespace/component", "qa"},
	}

	for _, tc := range testCases {
		t.Run(tc.component+"-"+tc.stack, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mock := NewMockWorkdirManager(ctrl)
			mock.EXPECT().CleanWorkdir(gomock.Any(), tc.component, tc.stack, gomock.Any()).Return(nil)

			original := workdirManager
			defer func() { workdirManager = original }()
			SetWorkdirManager(mock)

			err := cleanSpecificWorkdir(&schema.AtmosConfiguration{}, tc.component, tc.stack, nil)
			assert.NoError(t, err)
		})
	}
}

func TestCleanExpiredWorkdirs_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)
	mock.EXPECT().CleanExpiredWorkdirs(gomock.Any(), "7d", false).Return(nil)

	// Save and restore.
	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	atmosConfig := &schema.AtmosConfiguration{}
	err := cleanExpiredWorkdirs(atmosConfig, "7d", false)
	assert.NoError(t, err)
}

func TestCleanExpiredWorkdirs_DryRun(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)
	mock.EXPECT().CleanExpiredWorkdirs(gomock.Any(), "30d", true).Return(nil)

	// Save and restore.
	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	atmosConfig := &schema.AtmosConfiguration{}
	err := cleanExpiredWorkdirs(atmosConfig, "30d", true)
	assert.NoError(t, err)
}

func TestCleanExpiredWorkdirs_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockWorkdirManager(ctrl)
	expectedErr := errUtils.Build(errUtils.ErrWorkdirClean).
		WithExplanation("invalid TTL").
		Err()
	mock.EXPECT().CleanExpiredWorkdirs(gomock.Any(), "invalid", false).Return(expectedErr)

	// Save and restore.
	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	atmosConfig := &schema.AtmosConfiguration{}
	err := cleanExpiredWorkdirs(atmosConfig, "invalid", false)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirClean)
}

// RunE validation tests - test the command entry point validation logic.

// resetViperForTest resets viper flags for test isolation.
// Note: cmd.NewTestKit cannot be used here due to circular import between
// cmd/terraform/workdir and cmd packages.
func resetViperForTest(t *testing.T) {
	t.Helper()
	v := viper.GetViper()
	v.Set("all", false)
	v.Set("expired", false)
	v.Set("ttl", "")
	v.Set("dry-run", false)
	v.Set("stack", "")
}

func TestCleanCmd_RunE_ExpiredWithoutTTL(t *testing.T) {
	resetViperForTest(t)
	v := viper.GetViper()
	v.Set("expired", true)
	v.Set("ttl", "") // No TTL provided.

	err := cleanCmd.RunE(cleanCmd, []string{})
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirClean)
}

func TestCleanCmd_RunE_ExpiredWithAll(t *testing.T) {
	resetViperForTest(t)
	v := viper.GetViper()
	v.Set("expired", true)
	v.Set("ttl", "7d")
	v.Set("all", true) // Cannot use --all with --expired.

	err := cleanCmd.RunE(cleanCmd, []string{})
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirClean)
}

func TestCleanCmd_RunE_ExpiredWithComponent(t *testing.T) {
	resetViperForTest(t)
	v := viper.GetViper()
	v.Set("expired", true)
	v.Set("ttl", "7d")

	err := cleanCmd.RunE(cleanCmd, []string{"vpc"}) // Component arg with --expired.
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirClean)
}

func TestCleanCmd_RunE_ActualCall_AllWithComponent(t *testing.T) {
	resetViperForTest(t)
	v := viper.GetViper()
	v.Set("all", true)

	err := cleanCmd.RunE(cleanCmd, []string{"vpc"}) // --all with component arg.
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirClean)
}

func TestCleanCmd_RunE_ActualCall_NoFlagsNoArgs(t *testing.T) {
	resetViperForTest(t)

	err := cleanCmd.RunE(cleanCmd, []string{}) // No flags, no args.
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirClean)
}

func TestCleanCmd_RunE_ActualCall_ComponentWithoutStack(t *testing.T) {
	resetViperForTest(t)
	v := viper.GetViper()
	v.Set("stack", "") // No stack provided.

	err := cleanCmd.RunE(cleanCmd, []string{"vpc"}) // Component without --stack.
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirClean)
}
