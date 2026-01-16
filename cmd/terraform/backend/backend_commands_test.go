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
				mci.EXPECT().
					InitConfigAndAuth("vpc", "dev", "").
					Return(&schema.AtmosConfiguration{}, nil, nil)
				mp.EXPECT().
					CreateBackend(gomock.Any()).
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
				mci.EXPECT().
					InitConfigAndAuth("vpc", "dev", "").
					Return(&schema.AtmosConfiguration{}, nil, nil)
				mp.EXPECT().
					CreateBackend(gomock.Any()).
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
				mci.EXPECT().
					InitConfigAndAuth("vpc", "prod", "aws-prod").
					Return(&schema.AtmosConfiguration{}, &schema.AuthContext{AWS: &schema.AWSAuthContext{}}, nil)
				mp.EXPECT().
					CreateBackend(gomock.Any()).
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

// TestExecuteDeleteCommandWithValues tests the delete command helper function.
func TestExecuteDeleteCommandWithValues(t *testing.T) {
	tests := []struct {
		name          string
		component     string
		stack         string
		identity      string
		force         bool
		setupMocks    func(*MockConfigInitializer, *MockProvisioner)
		expectError   bool
		expectedError error
	}{
		{
			name:      "successful delete with force",
			component: "vpc",
			stack:     "dev",
			identity:  "",
			force:     true,
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				mci.EXPECT().
					InitConfigAndAuth("vpc", "dev", "").
					Return(&schema.AtmosConfiguration{}, nil, nil)
				mp.EXPECT().
					DeleteBackend(gomock.Any()).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:      "delete without force flag",
			component: "vpc",
			stack:     "dev",
			identity:  "",
			force:     false,
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				mci.EXPECT().
					InitConfigAndAuth("vpc", "dev", "").
					Return(&schema.AtmosConfiguration{}, nil, nil)
				mp.EXPECT().
					DeleteBackend(gomock.Any()).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:          "missing stack",
			component:     "vpc",
			stack:         "",
			identity:      "",
			force:         true,
			setupMocks:    func(*MockConfigInitializer, *MockProvisioner) {},
			expectError:   true,
			expectedError: errUtils.ErrRequiredFlagNotProvided,
		},
		{
			name:      "config init failure",
			component: "vpc",
			stack:     "dev",
			identity:  "",
			force:     true,
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				mci.EXPECT().
					InitConfigAndAuth("vpc", "dev", "").
					Return(nil, nil, errors.New("config init failed"))
			},
			expectError: true,
		},
		{
			name:      "delete backend failure",
			component: "vpc",
			stack:     "dev",
			identity:  "",
			force:     true,
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				mci.EXPECT().
					InitConfigAndAuth("vpc", "dev", "").
					Return(&schema.AtmosConfiguration{}, nil, nil)
				mp.EXPECT().
					DeleteBackend(gomock.Any()).
					Return(errors.New("delete failed"))
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfigInit, mockProv := setupTestWithMocks(t)
			tt.setupMocks(mockConfigInit, mockProv)

			err := executeDeleteCommandWithValues(tt.component, tt.stack, tt.identity, tt.force)

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

// TestExecuteDescribeCommandWithValues tests the describe command helper function.
func TestExecuteDescribeCommandWithValues(t *testing.T) {
	tests := []struct {
		name          string
		component     string
		stack         string
		identity      string
		format        string
		setupMocks    func(*MockConfigInitializer, *MockProvisioner)
		expectError   bool
		expectedError error
	}{
		{
			name:      "successful describe with yaml format",
			component: "vpc",
			stack:     "dev",
			identity:  "",
			format:    "yaml",
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
			name:      "successful describe with json format",
			component: "vpc",
			stack:     "dev",
			identity:  "",
			format:    "json",
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
			name:          "missing stack",
			component:     "vpc",
			stack:         "",
			identity:      "",
			format:        "yaml",
			setupMocks:    func(*MockConfigInitializer, *MockProvisioner) {},
			expectError:   true,
			expectedError: errUtils.ErrRequiredFlagNotProvided,
		},
		{
			name:      "config init failure",
			component: "vpc",
			stack:     "dev",
			identity:  "",
			format:    "yaml",
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				mci.EXPECT().
					InitConfigAndAuth("vpc", "dev", "").
					Return(nil, nil, errors.New("config init failed"))
			},
			expectError: true,
		},
		{
			name:      "describe backend failure",
			component: "vpc",
			stack:     "dev",
			identity:  "",
			format:    "yaml",
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
			tt.setupMocks(mockConfigInit, mockProv)

			err := executeDescribeCommandWithValues(tt.component, tt.stack, tt.identity, tt.format)

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

// TestExecuteListCommandWithValues tests the list command helper function.
func TestExecuteListCommandWithValues(t *testing.T) {
	tests := []struct {
		name          string
		stack         string
		identity      string
		format        string
		setupMocks    func(*MockConfigInitializer, *MockProvisioner)
		expectError   bool
		expectedError error
	}{
		{
			name:     "successful list with table format",
			stack:    "dev",
			identity: "",
			format:   "table",
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
			name:     "successful list with json format",
			stack:    "dev",
			identity: "",
			format:   "json",
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
			name:          "missing stack",
			stack:         "",
			identity:      "",
			format:        "table",
			setupMocks:    func(*MockConfigInitializer, *MockProvisioner) {},
			expectError:   true,
			expectedError: errUtils.ErrRequiredFlagNotProvided,
		},
		{
			name:     "config init failure",
			stack:    "dev",
			identity: "",
			format:   "table",
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				mci.EXPECT().
					InitConfigAndAuth("", "dev", "").
					Return(nil, nil, errors.New("config init failed"))
			},
			expectError: true,
		},
		{
			name:     "list backends failure",
			stack:    "dev",
			identity: "",
			format:   "table",
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
			tt.setupMocks(mockConfigInit, mockProv)

			err := executeListCommandWithValues(tt.stack, tt.identity, tt.format)

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

// TestExecuteProvisionCommandWithValues tests the provision command helper function.
func TestExecuteProvisionCommandWithValues(t *testing.T) {
	tests := []struct {
		name          string
		component     string
		stack         string
		identity      string
		setupMocks    func(*MockConfigInitializer, *MockProvisioner)
		expectError   bool
		expectedError error
	}{
		{
			name:      "successful provision",
			component: "vpc",
			stack:     "dev",
			identity:  "",
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				mci.EXPECT().
					InitConfigAndAuth("vpc", "dev", "").
					Return(&schema.AtmosConfiguration{}, nil, nil)
				mp.EXPECT().
					CreateBackend(gomock.Any()).
					Return(nil)
			},
			expectError: false,
		},
		{
			name:          "missing stack",
			component:     "vpc",
			stack:         "",
			identity:      "",
			setupMocks:    func(*MockConfigInitializer, *MockProvisioner) {},
			expectError:   true,
			expectedError: errUtils.ErrRequiredFlagNotProvided,
		},
		{
			name:      "config init failure",
			component: "vpc",
			stack:     "dev",
			identity:  "",
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				mci.EXPECT().
					InitConfigAndAuth("vpc", "dev", "").
					Return(nil, nil, errors.New("config init failed"))
			},
			expectError: true,
		},
		{
			name:      "provision failure",
			component: "vpc",
			stack:     "dev",
			identity:  "",
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				mci.EXPECT().
					InitConfigAndAuth("vpc", "dev", "").
					Return(&schema.AtmosConfiguration{}, nil, nil)
				mp.EXPECT().
					CreateBackend(gomock.Any()).
					Return(errors.New("provision failed"))
			},
			expectError: true,
		},
		{
			name:      "with identity",
			component: "vpc",
			stack:     "prod",
			identity:  "aws-prod",
			setupMocks: func(mci *MockConfigInitializer, mp *MockProvisioner) {
				mci.EXPECT().
					InitConfigAndAuth("vpc", "prod", "aws-prod").
					Return(&schema.AtmosConfiguration{}, &schema.AuthContext{AWS: &schema.AWSAuthContext{}}, nil)
				mp.EXPECT().
					CreateBackend(gomock.Any()).
					Return(nil)
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfigInit, mockProv := setupTestWithMocks(t)
			tt.setupMocks(mockConfigInit, mockProv)

			err := executeProvisionCommandWithValues(tt.component, tt.stack, tt.identity)

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
