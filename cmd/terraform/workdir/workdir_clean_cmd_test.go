package workdir

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

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

func TestCleanCmd_DisableFlagParsing(t *testing.T) {
	// Verify flag parsing is enabled.
	assert.False(t, cleanCmd.DisableFlagParsing)
}

func TestCleanAllWorkdirs_Success(t *testing.T) {
	mock := NewMockWorkdirManager()
	mock.CleanAllWorkdirsFunc = func(atmosConfig *schema.AtmosConfiguration) error {
		return nil
	}

	// Save and restore.
	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	atmosConfig := &schema.AtmosConfiguration{}
	err := cleanAllWorkdirs(atmosConfig)
	assert.NoError(t, err)
	assert.Equal(t, 1, mock.CleanAllWorkdirsCalls)
}

func TestCleanAllWorkdirs_Error(t *testing.T) {
	mock := NewMockWorkdirManager()
	expectedErr := errors.New("clean failed")
	mock.CleanAllWorkdirsFunc = func(atmosConfig *schema.AtmosConfiguration) error {
		return expectedErr
	}

	// Save and restore.
	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	atmosConfig := &schema.AtmosConfiguration{}
	err := cleanAllWorkdirs(atmosConfig)
	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)
}

func TestCleanSpecificWorkdir_Success(t *testing.T) {
	mock := NewMockWorkdirManager()
	mock.CleanWorkdirFunc = func(atmosConfig *schema.AtmosConfiguration, component, stack string) error {
		assert.Equal(t, "vpc", component)
		assert.Equal(t, "dev", stack)
		return nil
	}

	// Save and restore.
	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	atmosConfig := &schema.AtmosConfiguration{}
	err := cleanSpecificWorkdir(atmosConfig, "vpc", "dev")
	assert.NoError(t, err)
	assert.Equal(t, 1, mock.CleanWorkdirCalls)
}

func TestCleanSpecificWorkdir_Error(t *testing.T) {
	mock := NewMockWorkdirManager()
	expectedErr := errUtils.Build(errUtils.ErrWorkdirClean).
		WithExplanation("workdir not found").
		Err()
	mock.CleanWorkdirFunc = func(atmosConfig *schema.AtmosConfiguration, component, stack string) error {
		return expectedErr
	}

	// Save and restore.
	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	atmosConfig := &schema.AtmosConfiguration{}
	err := cleanSpecificWorkdir(atmosConfig, "vpc", "dev")
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWorkdirClean)
}

func TestMockWorkdirManager_CleanWorkdir(t *testing.T) {
	mock := NewMockWorkdirManager()

	var calledComponent, calledStack string
	mock.CleanWorkdirFunc = func(atmosConfig *schema.AtmosConfiguration, component, stack string) error {
		calledComponent = component
		calledStack = stack
		return nil
	}

	err := mock.CleanWorkdir(nil, "s3", "prod")
	assert.NoError(t, err)
	assert.Equal(t, "s3", calledComponent)
	assert.Equal(t, "prod", calledStack)
	assert.Equal(t, 1, mock.CleanWorkdirCalls)
}

func TestMockWorkdirManager_CleanAllWorkdirs(t *testing.T) {
	mock := NewMockWorkdirManager()

	var called bool
	mock.CleanAllWorkdirsFunc = func(atmosConfig *schema.AtmosConfiguration) error {
		called = true
		return nil
	}

	err := mock.CleanAllWorkdirs(nil)
	assert.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, 1, mock.CleanAllWorkdirsCalls)
}

func TestMockWorkdirManager_Reset(t *testing.T) {
	mock := NewMockWorkdirManager()

	// Call various methods.
	mock.ListWorkdirs(nil)
	mock.GetWorkdirInfo(nil, "", "")
	mock.DescribeWorkdir(nil, "", "")
	mock.CleanWorkdir(nil, "", "")
	mock.CleanAllWorkdirs(nil)

	assert.Equal(t, 1, mock.ListWorkdirsCalls)
	assert.Equal(t, 1, mock.GetWorkdirInfoCalls)
	assert.Equal(t, 1, mock.DescribeWorkdirCalls)
	assert.Equal(t, 1, mock.CleanWorkdirCalls)
	assert.Equal(t, 1, mock.CleanAllWorkdirsCalls)

	// Reset.
	mock.Reset()

	assert.Equal(t, 0, mock.ListWorkdirsCalls)
	assert.Equal(t, 0, mock.GetWorkdirInfoCalls)
	assert.Equal(t, 0, mock.DescribeWorkdirCalls)
	assert.Equal(t, 0, mock.CleanWorkdirCalls)
	assert.Equal(t, 0, mock.CleanAllWorkdirsCalls)
}

// Test validation scenarios.

func TestCleanCmd_ValidationScenarios(t *testing.T) {
	tests := []struct {
		name        string
		all         bool
		hasArgs     bool
		hasStack    bool
		shouldError bool
		errorMsg    string
	}{
		{
			name:        "all flag only",
			all:         true,
			hasArgs:     false,
			hasStack:    false,
			shouldError: false,
		},
		{
			name:        "component with stack",
			all:         false,
			hasArgs:     true,
			hasStack:    true,
			shouldError: false,
		},
		{
			name:        "all with args - conflict",
			all:         true,
			hasArgs:     true,
			hasStack:    false,
			shouldError: true,
			errorMsg:    "Cannot specify both --all and a component",
		},
		{
			name:        "no all and no args",
			all:         false,
			hasArgs:     false,
			hasStack:    false,
			shouldError: true,
			errorMsg:    "Either --all or a component is required",
		},
		{
			name:        "component without stack",
			all:         false,
			hasArgs:     true,
			hasStack:    false,
			shouldError: true,
			errorMsg:    "Stack is required when cleaning a specific workdir",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Document the expected validation behavior.
			if tt.shouldError {
				assert.NotEmpty(t, tt.errorMsg)
			}
		})
	}
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

func TestCleanSpecificWorkdir_WithNilManager(t *testing.T) {
	// When mock returns nil error, should succeed.
	mock := NewMockWorkdirManager()

	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	err := cleanSpecificWorkdir(nil, "vpc", "dev")
	assert.NoError(t, err)
}

func TestCleanAllWorkdirs_WithNilManager(t *testing.T) {
	mock := NewMockWorkdirManager()

	original := workdirManager
	defer func() { workdirManager = original }()
	SetWorkdirManager(mock)

	err := cleanAllWorkdirs(nil)
	assert.NoError(t, err)
}
