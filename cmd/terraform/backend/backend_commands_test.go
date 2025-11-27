package backend

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
)

// setupTestWithMocks creates gomock controller, mock dependencies, and cleanup.
func setupTestWithMocks(t *testing.T) (*MockConfigInitializer, *MockProvisioner) {
	t.Helper()

	ctrl := gomock.NewController(t)

	mockConfigInit := NewMockConfigInitializer(ctrl)
	mockProv := NewMockProvisioner(ctrl)

	// Inject mocks.
	SetConfigInitializer(mockConfigInit)
	SetProvisioner(mockProv)

	// Register cleanup.
	t.Cleanup(func() {
		ResetDependencies()
		ctrl.Finish()
	})

	return mockConfigInit, mockProv
}

// setupViperForTest resets Viper and sets test values.
func setupViperForTest(t *testing.T, values map[string]any) {
	t.Helper()

	// Save current state.
	oldViper := viper.GetViper()
	oldKeys := make(map[string]any)
	for _, key := range oldViper.AllKeys() {
		oldKeys[key] = oldViper.Get(key)
	}

	// Reset and set new values.
	viper.Reset()
	for k, v := range values {
		viper.Set(k, v)
	}

	// Register cleanup to restore.
	t.Cleanup(func() {
		viper.Reset()
		for key, val := range oldKeys {
			viper.Set(key, val)
		}
	})
}

// TestExecuteProvisionCommand tests the shared provision command implementation.
func TestExecuteProvisionCommand(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		viperValues   map[string]any
		setupMocks    func(*MockConfigInitializer, *MockProvisioner)
		expectError   bool
		expectedError error
	}{
		{
			name: "successful provision",
			args: []string{"vpc"},
			viperValues: map[string]any{
				"stack":    "dev",
				"identity": "",
			},
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				atmosConfig := &schema.AtmosConfiguration{}
				mci.EXPECT().
					InitConfigAndAuth("vpc", "dev", "").
					Return(atmosConfig, nil, nil)
				mp.EXPECT().
					CreateBackend(atmosConfig, "vpc", "dev", gomock.Any(), nil).
					Return(nil)
			},
			expectError: false,
		},
		{
			name: "missing stack flag",
			args: []string{"vpc"},
			viperValues: map[string]any{
				"stack":    "",
				"identity": "",
			},
			setupMocks:    func(*MockConfigInitializer, *MockProvisioner) {},
			expectError:   true,
			expectedError: errUtils.ErrRequiredFlagNotProvided,
		},
		{
			name: "config init failure",
			args: []string{"vpc"},
			viperValues: map[string]any{
				"stack":    "dev",
				"identity": "",
			},
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				mci.EXPECT().
					InitConfigAndAuth("vpc", "dev", "").
					Return(nil, nil, errors.New("config init failed"))
			},
			expectError: true,
		},
		{
			name: "provision failure",
			args: []string{"vpc"},
			viperValues: map[string]any{
				"stack":    "dev",
				"identity": "",
			},
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				atmosConfig := &schema.AtmosConfiguration{}
				mci.EXPECT().
					InitConfigAndAuth("vpc", "dev", "").
					Return(atmosConfig, nil, nil)
				mp.EXPECT().
					CreateBackend(atmosConfig, "vpc", "dev", gomock.Any(), nil).
					Return(errors.New("provision failed"))
			},
			expectError: true,
		},
		{
			name: "with auth context",
			args: []string{"vpc"},
			viperValues: map[string]any{
				"stack":    "prod",
				"identity": "aws-prod",
			},
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				atmosConfig := &schema.AtmosConfiguration{}
				authCtx := &schema.AuthContext{AWS: &schema.AWSAuthContext{}}
				mci.EXPECT().
					InitConfigAndAuth("vpc", "prod", "aws-prod").
					Return(atmosConfig, authCtx, nil)
				mp.EXPECT().
					CreateBackend(atmosConfig, "vpc", "prod", gomock.Any(), authCtx).
					Return(nil)
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfigInit, mockProv := setupTestWithMocks(t)
			setupViperForTest(t, tt.viperValues)
			tt.setupMocks(mockConfigInit, mockProv)

			cmd := &cobra.Command{Use: "test"}
			parser := flags.NewStandardParser(
				flags.WithStackFlag(),
				flags.WithIdentityFlag(),
			)
			parser.RegisterFlags(cmd)
			require.NoError(t, parser.BindToViper(viper.GetViper()))

			err := ExecuteProvisionCommand(cmd, tt.args, parser, "test.RunE")

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedError != nil {
					assert.ErrorIs(t, err, tt.expectedError)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestDeleteCmd_RunE tests the delete command RunE function.
func TestDeleteCmd_RunE(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		viperValues   map[string]any
		setupMocks    func(*MockConfigInitializer, *MockProvisioner)
		expectError   bool
		expectedError error
	}{
		{
			name: "successful delete with force",
			args: []string{"vpc"},
			viperValues: map[string]any{
				"stack":    "dev",
				"identity": "",
				"force":    true,
			},
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				atmosConfig := &schema.AtmosConfiguration{}
				mci.EXPECT().
					InitConfigAndAuth("vpc", "dev", "").
					Return(atmosConfig, nil, nil)
				mp.EXPECT().
					DeleteBackend(atmosConfig, "vpc", "dev", true, gomock.Any(), nil).
					Return(nil)
			},
			expectError: false,
		},
		{
			name: "delete without force flag",
			args: []string{"vpc"},
			viperValues: map[string]any{
				"stack":    "dev",
				"identity": "",
				"force":    false,
			},
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				atmosConfig := &schema.AtmosConfiguration{}
				mci.EXPECT().
					InitConfigAndAuth("vpc", "dev", "").
					Return(atmosConfig, nil, nil)
				mp.EXPECT().
					DeleteBackend(atmosConfig, "vpc", "dev", false, gomock.Any(), nil).
					Return(nil)
			},
			expectError: false,
		},
		{
			name: "missing stack flag",
			args: []string{"vpc"},
			viperValues: map[string]any{
				"stack":    "",
				"identity": "",
				"force":    true,
			},
			setupMocks:    func(*MockConfigInitializer, *MockProvisioner) {},
			expectError:   true,
			expectedError: errUtils.ErrRequiredFlagNotProvided,
		},
		{
			name: "config init failure",
			args: []string{"vpc"},
			viperValues: map[string]any{
				"stack":    "dev",
				"identity": "",
				"force":    true,
			},
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				mci.EXPECT().
					InitConfigAndAuth("vpc", "dev", "").
					Return(nil, nil, errors.New("config init failed"))
			},
			expectError: true,
		},
		{
			name: "delete backend failure",
			args: []string{"vpc"},
			viperValues: map[string]any{
				"stack":    "dev",
				"identity": "",
				"force":    true,
			},
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				atmosConfig := &schema.AtmosConfiguration{}
				mci.EXPECT().
					InitConfigAndAuth("vpc", "dev", "").
					Return(atmosConfig, nil, nil)
				mp.EXPECT().
					DeleteBackend(atmosConfig, "vpc", "dev", true, gomock.Any(), nil).
					Return(errors.New("delete failed"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfigInit, mockProv := setupTestWithMocks(t)
			setupViperForTest(t, tt.viperValues)
			tt.setupMocks(mockConfigInit, mockProv)

			err := deleteCmd.RunE(deleteCmd, tt.args)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedError != nil {
					assert.ErrorIs(t, err, tt.expectedError)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestDescribeCmd_RunE tests the describe command RunE function.
func TestDescribeCmd_RunE(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		viperValues   map[string]any
		setupMocks    func(*MockConfigInitializer, *MockProvisioner)
		expectError   bool
		expectedError error
	}{
		{
			name: "successful describe with yaml format",
			args: []string{"vpc"},
			viperValues: map[string]any{
				"stack":    "dev",
				"identity": "",
				"format":   "yaml",
			},
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				atmosConfig := &schema.AtmosConfiguration{}
				mci.EXPECT().
					InitConfigAndAuth("vpc", "dev", "").
					Return(atmosConfig, nil, nil)
				mp.EXPECT().
					DescribeBackend(atmosConfig, "vpc", map[string]string{"format": "yaml"}).
					Return(nil)
			},
			expectError: false,
		},
		{
			name: "successful describe with json format",
			args: []string{"vpc"},
			viperValues: map[string]any{
				"stack":    "dev",
				"identity": "",
				"format":   "json",
			},
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				atmosConfig := &schema.AtmosConfiguration{}
				mci.EXPECT().
					InitConfigAndAuth("vpc", "dev", "").
					Return(atmosConfig, nil, nil)
				mp.EXPECT().
					DescribeBackend(atmosConfig, "vpc", map[string]string{"format": "json"}).
					Return(nil)
			},
			expectError: false,
		},
		{
			name: "missing stack flag",
			args: []string{"vpc"},
			viperValues: map[string]any{
				"stack":    "",
				"identity": "",
				"format":   "yaml",
			},
			setupMocks:    func(*MockConfigInitializer, *MockProvisioner) {},
			expectError:   true,
			expectedError: errUtils.ErrRequiredFlagNotProvided,
		},
		{
			name: "config init failure",
			args: []string{"vpc"},
			viperValues: map[string]any{
				"stack":    "dev",
				"identity": "",
				"format":   "yaml",
			},
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				mci.EXPECT().
					InitConfigAndAuth("vpc", "dev", "").
					Return(nil, nil, errors.New("config init failed"))
			},
			expectError: true,
		},
		{
			name: "describe backend failure",
			args: []string{"vpc"},
			viperValues: map[string]any{
				"stack":    "dev",
				"identity": "",
				"format":   "yaml",
			},
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				atmosConfig := &schema.AtmosConfiguration{}
				mci.EXPECT().
					InitConfigAndAuth("vpc", "dev", "").
					Return(atmosConfig, nil, nil)
				mp.EXPECT().
					DescribeBackend(atmosConfig, "vpc", map[string]string{"format": "yaml"}).
					Return(errors.New("describe failed"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfigInit, mockProv := setupTestWithMocks(t)
			setupViperForTest(t, tt.viperValues)
			tt.setupMocks(mockConfigInit, mockProv)

			err := describeCmd.RunE(describeCmd, tt.args)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedError != nil {
					assert.ErrorIs(t, err, tt.expectedError)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestListCmd_RunE tests the list command RunE function.
func TestListCmd_RunE(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		viperValues   map[string]any
		setupMocks    func(*MockConfigInitializer, *MockProvisioner)
		expectError   bool
		expectedError error
	}{
		{
			name: "successful list with table format",
			args: []string{},
			viperValues: map[string]any{
				"stack":    "dev",
				"identity": "",
				"format":   "table",
			},
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				atmosConfig := &schema.AtmosConfiguration{}
				mci.EXPECT().
					InitConfigAndAuth("", "dev", "").
					Return(atmosConfig, nil, nil)
				mp.EXPECT().
					ListBackends(atmosConfig, map[string]string{"format": "table"}).
					Return(nil)
			},
			expectError: false,
		},
		{
			name: "successful list with json format",
			args: []string{},
			viperValues: map[string]any{
				"stack":    "dev",
				"identity": "",
				"format":   "json",
			},
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				atmosConfig := &schema.AtmosConfiguration{}
				mci.EXPECT().
					InitConfigAndAuth("", "dev", "").
					Return(atmosConfig, nil, nil)
				mp.EXPECT().
					ListBackends(atmosConfig, map[string]string{"format": "json"}).
					Return(nil)
			},
			expectError: false,
		},
		{
			name: "missing stack flag",
			args: []string{},
			viperValues: map[string]any{
				"stack":    "",
				"identity": "",
				"format":   "table",
			},
			setupMocks:    func(*MockConfigInitializer, *MockProvisioner) {},
			expectError:   true,
			expectedError: errUtils.ErrRequiredFlagNotProvided,
		},
		{
			name: "config init failure",
			args: []string{},
			viperValues: map[string]any{
				"stack":    "dev",
				"identity": "",
				"format":   "table",
			},
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				mci.EXPECT().
					InitConfigAndAuth("", "dev", "").
					Return(nil, nil, errors.New("config init failed"))
			},
			expectError: true,
		},
		{
			name: "list backends failure",
			args: []string{},
			viperValues: map[string]any{
				"stack":    "dev",
				"identity": "",
				"format":   "table",
			},
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				atmosConfig := &schema.AtmosConfiguration{}
				mci.EXPECT().
					InitConfigAndAuth("", "dev", "").
					Return(atmosConfig, nil, nil)
				mp.EXPECT().
					ListBackends(atmosConfig, map[string]string{"format": "table"}).
					Return(errors.New("list failed"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfigInit, mockProv := setupTestWithMocks(t)
			setupViperForTest(t, tt.viperValues)
			tt.setupMocks(mockConfigInit, mockProv)

			err := listCmd.RunE(listCmd, tt.args)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedError != nil {
					assert.ErrorIs(t, err, tt.expectedError)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestSetConfigInitializer tests the SetConfigInitializer function.
func TestSetConfigInitializer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockConfigInitializer(ctrl)
	SetConfigInitializer(mock)

	t.Cleanup(func() {
		ResetDependencies()
	})

	// Verify the mock was set.
	assert.Equal(t, mock, configInit)
}

// TestSetProvisioner tests the SetProvisioner function.
func TestSetProvisioner(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mock := NewMockProvisioner(ctrl)
	SetProvisioner(mock)

	t.Cleanup(func() {
		ResetDependencies()
	})

	// Verify the mock was set.
	assert.Equal(t, mock, prov)
}

// TestResetDependencies tests the ResetDependencies function.
func TestResetDependencies(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Set mocks.
	mockConfigInit := NewMockConfigInitializer(ctrl)
	mockProv := NewMockProvisioner(ctrl)
	SetConfigInitializer(mockConfigInit)
	SetProvisioner(mockProv)

	// Reset.
	ResetDependencies()

	// Verify defaults are restored (type assertion).
	_, isDefaultConfigInit := configInit.(*defaultConfigInitializer)
	assert.True(t, isDefaultConfigInit, "configInit should be reset to defaultConfigInitializer")

	_, isDefaultProv := prov.(*defaultProvisioner)
	assert.True(t, isDefaultProv, "prov should be reset to defaultProvisioner")
}

// TestCreateDescribeComponentFunc_ReturnsNonNil tests that CreateDescribeComponentFunc returns a non-nil function.
func TestCreateDescribeComponentFunc_ReturnsNonNil(t *testing.T) {
	// Test that the function is created correctly with nil authManager.
	describeFunc := CreateDescribeComponentFunc(nil)
	assert.NotNil(t, describeFunc, "describeFunc should not be nil")

	// The function is created but we can't easily test execution
	// without real config - the important thing is it doesn't panic.
}

// TestParseCommonFlags_Success tests successful parsing in ParseCommonFlags.
func TestParseCommonFlags_Success(t *testing.T) {
	// Test successful parsing with all flags.
	setupViperForTest(t, map[string]any{
		"stack":    "test-stack",
		"identity": "test-identity",
	})

	cmd := &cobra.Command{Use: "test"}
	parser := flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
	)
	parser.RegisterFlags(cmd)
	require.NoError(t, parser.BindToViper(viper.GetViper()))

	opts, err := ParseCommonFlags(cmd, parser)
	assert.NoError(t, err)
	assert.NotNil(t, opts)
	assert.Equal(t, "test-stack", opts.Stack)
	assert.Equal(t, "test-identity", opts.Identity)
}
