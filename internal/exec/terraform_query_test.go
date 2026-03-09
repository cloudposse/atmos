package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestCreateQueryAuthManager verifies createQueryAuthManager behavior across
// different scenarios using injected auth factory to cover all branches.
func TestCreateQueryAuthManager(t *testing.T) {
	tests := []struct {
		name             string
		factory          func(string, schema.AuthConfig, string, *schema.AtmosConfiguration) (auth.AuthManager, error)
		expectErr        bool
		expectSentinel   error
		expectNilMgr     bool
		expectInfoStored bool
		expectExit       bool
	}{
		{
			name: "no auth configured returns nil manager",
			factory: func(string, schema.AuthConfig, string, *schema.AtmosConfiguration) (auth.AuthManager, error) {
				return nil, nil
			},
			expectNilMgr:     true,
			expectInfoStored: false,
		},
		{
			name: "nonexistent identity returns wrapped error",
			factory: func(string, schema.AuthConfig, string, *schema.AtmosConfiguration) (auth.AuthManager, error) {
				return nil, errUtils.ErrAuthNotConfigured
			},
			expectErr:        true,
			expectSentinel:   errUtils.ErrAuthNotConfigured,
			expectNilMgr:     true,
			expectInfoStored: false,
		},
		{
			name: "non-nil manager is stored in info",
			factory: func(string, schema.AuthConfig, string, *schema.AtmosConfiguration) (auth.AuthManager, error) {
				ctrl := gomock.NewController(t)
				return types.NewMockAuthManager(ctrl), nil
			},
			expectNilMgr:     false,
			expectInfoStored: true,
		},
		{
			name: "ErrUserAborted calls Exit with SIGINT code",
			factory: func(string, schema.AuthConfig, string, *schema.AtmosConfiguration) (auth.AuthManager, error) {
				return nil, errUtils.ErrUserAborted
			},
			expectExit: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Inject test factory.
			original := authManagerFactory
			authManagerFactory = tt.factory
			defer func() { authManagerFactory = original }()

			// Mock os.Exit to capture exit calls.
			var exitCode int
			exitCalled := false
			originalExit := errUtils.OsExit
			errUtils.OsExit = func(code int) {
				exitCode = code
				exitCalled = true
			}
			defer func() { errUtils.OsExit = originalExit }()

			info := &schema.ConfigAndStacksInfo{}
			atmosConfig := &schema.AtmosConfiguration{
				BasePath: t.TempDir(),
			}

			authManager, err := createQueryAuthManager(info, atmosConfig)

			if tt.expectExit {
				assert.True(t, exitCalled, "Exit should have been called")
				assert.Equal(t, errUtils.ExitCodeSIGINT, exitCode)
				return
			}

			assert.False(t, exitCalled, "Exit should not have been called")

			if tt.expectErr {
				assert.Error(t, err)
				if tt.expectSentinel != nil {
					assert.ErrorIs(t, err, tt.expectSentinel)
				}
			} else {
				assert.NoError(t, err)
			}

			if tt.expectNilMgr {
				assert.Nil(t, authManager)
			} else {
				assert.NotNil(t, authManager)
			}

			if tt.expectInfoStored {
				assert.NotNil(t, info.AuthManager, "AuthManager should be stored in info")
			} else {
				assert.Nil(t, info.AuthManager, "AuthManager should not be stored in info")
			}
		})
	}
}

// TestPropagateAuth verifies that propagateAuth extracts AuthContext from
// AuthManager.GetStackInfo() and populates ConfigAndStacksInfo. This is the
// core propagation logic used by ExecuteDescribeStacks for each component.
func TestPropagateAuth(t *testing.T) {
	tests := []struct {
		name              string
		setupManager      func(ctrl *gomock.Controller) auth.AuthManager
		expectAuthContext *schema.AuthContext
		expectAuthManager bool
	}{
		{
			name: "populates AuthContext and AuthManager from manager",
			setupManager: func(ctrl *gomock.Controller) auth.AuthManager {
				mockManager := types.NewMockAuthManager(ctrl)
				mockManager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{
					AuthContext: &schema.AuthContext{
						AWS: &schema.AWSAuthContext{
							Profile: "test-sso-profile",
							Region:  "eu-central-1",
						},
					},
				})
				return mockManager
			},
			expectAuthContext: &schema.AuthContext{
				AWS: &schema.AWSAuthContext{
					Profile: "test-sso-profile",
					Region:  "eu-central-1",
				},
			},
			expectAuthManager: true,
		},
		{
			name: "nil AuthManager is a no-op",
			setupManager: func(_ *gomock.Controller) auth.AuthManager {
				return nil
			},
			expectAuthContext: nil,
			expectAuthManager: false,
		},
		{
			name: "GetStackInfo returns nil leaves AuthContext nil",
			setupManager: func(ctrl *gomock.Controller) auth.AuthManager {
				mockManager := types.NewMockAuthManager(ctrl)
				mockManager.EXPECT().GetStackInfo().Return(nil)
				return mockManager
			},
			expectAuthContext: nil,
			expectAuthManager: true,
		},
		{
			name: "GetStackInfo returns info with nil AuthContext",
			setupManager: func(ctrl *gomock.Controller) auth.AuthManager {
				mockManager := types.NewMockAuthManager(ctrl)
				mockManager.EXPECT().GetStackInfo().Return(&schema.ConfigAndStacksInfo{
					AuthContext: nil,
				})
				return mockManager
			},
			expectAuthContext: nil,
			expectAuthManager: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Start with empty configAndStacksInfo — no pre-populated auth.
			info := schema.ConfigAndStacksInfo{}
			mgr := tt.setupManager(ctrl)

			propagateAuth(&info, mgr)

			assert.Equal(t, tt.expectAuthContext, info.AuthContext)
			if tt.expectAuthManager {
				assert.NotNil(t, info.AuthManager)
			} else {
				assert.Nil(t, info.AuthManager)
			}
		})
	}
}

// TestYamlFunctionAuthPassthrough verifies that when ProcessCustomYamlTags
// receives a stackInfo with AuthContext and AuthManager, those values flow
// through to the underlying YAML function handlers (!terraform.state).
func TestYamlFunctionAuthPassthrough(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)
	expectedAuthContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile: "test-sso-profile",
			Region:  "eu-central-1",
		},
	}

	mockStateGetter := NewMockTerraformStateGetter(ctrl)
	originalGetter := stateGetter
	stateGetter = mockStateGetter
	defer func() { stateGetter = originalGetter }()

	mockStateGetter.EXPECT().
		GetState(
			gomock.Any(),                   // atmosConfig
			gomock.Any(),                   // yamlFunc
			gomock.Any(),                   // stack
			gomock.Any(),                   // component
			gomock.Any(),                   // output
			gomock.Any(),                   // skipCache
			gomock.Eq(expectedAuthContext), // authContext - MUST match
			gomock.Eq(mockManager),         // authManager - MUST match
		).
		Return("test-value", nil).
		Times(1)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	input := schema.AtmosSectionMapType{
		"backend_config": "!terraform.state vpc test-stack bucket_name",
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, &schema.ConfigAndStacksInfo{
		AuthContext: expectedAuthContext,
		AuthManager: mockManager,
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "test-value", result["backend_config"])
}

// TestYamlFunctionNilAuth verifies that !terraform.state receives nil auth
// when no AuthManager is configured (backward compatibility).
func TestYamlFunctionNilAuth(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStateGetter := NewMockTerraformStateGetter(ctrl)
	originalGetter := stateGetter
	stateGetter = mockStateGetter
	defer func() { stateGetter = originalGetter }()

	mockStateGetter.EXPECT().
		GetState(
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Nil(), // authContext nil when no AuthManager.
			gomock.Any(), // authManager parameter.
		).
		Return("test-value", nil).
		Times(1)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	input := schema.AtmosSectionMapType{
		"backend_config": "!terraform.state vpc test-stack bucket_name",
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "test-value", result["backend_config"])
}

// TestTerraformOutputAuthPropagation verifies that !terraform.output also receives
// auth context and auth manager when processing with a non-nil AuthManager.
// This ensures the fix covers !terraform.output in addition to !terraform.state.
func TestTerraformOutputAuthPropagation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)
	expectedAuthContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile: "test-sso-profile",
			Region:  "eu-central-1",
		},
	}

	mockOutputGetter := NewMockTerraformOutputGetter(ctrl)

	originalGetter := outputGetter
	outputGetter = mockOutputGetter
	defer func() { outputGetter = originalGetter }()

	// Expect GetOutput to be called WITH the auth context and auth manager.
	mockOutputGetter.EXPECT().
		GetOutput(
			gomock.Any(),                   // atmosConfig
			gomock.Any(),                   // stack
			gomock.Any(),                   // component
			gomock.Any(),                   // output
			gomock.Any(),                   // skipCache
			gomock.Eq(expectedAuthContext), // authContext - MUST be from AuthManager
			gomock.Eq(mockManager),         // authManager - MUST be the AuthManager itself
		).
		Return("output-value", true, nil).
		Times(1)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	input := schema.AtmosSectionMapType{
		"region": "!terraform.output vpc test-stack aws_region",
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, &schema.ConfigAndStacksInfo{
		AuthContext: expectedAuthContext,
		AuthManager: mockManager,
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "output-value", result["region"])
}

// TestTerraformOutputNoAuth verifies that !terraform.output works with nil auth
// (backward compatibility for users without auth configured).
func TestTerraformOutputNoAuth(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOutputGetter := NewMockTerraformOutputGetter(ctrl)

	originalGetter := outputGetter
	outputGetter = mockOutputGetter
	defer func() { outputGetter = originalGetter }()

	mockOutputGetter.EXPECT().
		GetOutput(
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Any(),
			gomock.Nil(), // authContext nil when no auth.
			gomock.Nil(), // authManager nil when no auth.
		).
		Return("output-value", true, nil).
		Times(1)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	input := schema.AtmosSectionMapType{
		"region": "!terraform.output vpc test-stack aws_region",
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, nil)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "output-value", result["region"])
}

// TestMultipleYamlFunctionsAuthPropagation verifies that auth context is propagated
// to multiple different YAML functions in the same input map. This simulates a real
// stack config where multiple functions depend on auth.
func TestMultipleYamlFunctionsAuthPropagation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := types.NewMockAuthManager(ctrl)
	expectedAuthContext := &schema.AuthContext{
		AWS: &schema.AWSAuthContext{
			Profile: "multi-profile",
			Region:  "us-west-2",
		},
	}

	// Mock both state and output getters.
	mockStateGetter := NewMockTerraformStateGetter(ctrl)
	originalStateGetter := stateGetter
	stateGetter = mockStateGetter
	defer func() { stateGetter = originalStateGetter }()

	mockOutputGetter := NewMockTerraformOutputGetter(ctrl)
	originalOutputGetter := outputGetter
	outputGetter = mockOutputGetter
	defer func() { outputGetter = originalOutputGetter }()

	// Both should receive the auth context.
	mockStateGetter.EXPECT().
		GetState(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(), gomock.Any(),
			gomock.Eq(expectedAuthContext),
			gomock.Eq(mockManager),
		).
		Return("state-bucket", nil).
		Times(1)

	mockOutputGetter.EXPECT().
		GetOutput(
			gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			gomock.Any(),
			gomock.Eq(expectedAuthContext),
			gomock.Eq(mockManager),
		).
		Return("output-region", true, nil).
		Times(1)

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: t.TempDir(),
	}

	input := schema.AtmosSectionMapType{
		"bucket": "!terraform.state vpc test-stack bucket_name",
		"region": "!terraform.output vpc test-stack aws_region",
	}

	result, err := ProcessCustomYamlTags(atmosConfig, input, "test-stack", nil, &schema.ConfigAndStacksInfo{
		AuthContext: expectedAuthContext,
		AuthManager: mockManager,
	})

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "state-bucket", result["bucket"])
	assert.Equal(t, "output-region", result["region"])
}
