package ecr

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCreateAuthManager_Success(t *testing.T) {
	// Test that createAuthManager can be created with valid config.
	authConfig := &schema.AuthConfig{
		Realm:        "test-realm",
		Providers:    map[string]schema.Provider{},
		Identities:   map[string]schema.Identity{},
		Integrations: map[string]schema.Integration{},
	}

	manager, err := createAuthManager(authConfig, "")
	require.NoError(t, err)
	assert.NotNil(t, manager)
}

func TestCreateAuthManager_NilConfig(t *testing.T) {
	// Test that createAuthManager fails with nil config.
	_, err := createAuthManager(nil, "")
	require.Error(t, err)
}

func TestLoginCmd_Help(t *testing.T) {
	// Verify command metadata.
	assert.Equal(t, "login [integration]", loginCmd.Use)
	assert.Equal(t, "Login to AWS ECR registries", loginCmd.Short)
	assert.Contains(t, loginCmd.Long, "Login to AWS ECR registries")
}

func TestLoginCmd_HasRegistryFlag(t *testing.T) {
	// Verify --registry flag exists.
	registryFlag := loginCmd.Flags().Lookup("registry")
	require.NotNil(t, registryFlag)
	assert.Equal(t, "r", registryFlag.Shorthand)
}

func TestLoginCmd_HasIdentityFlag(t *testing.T) {
	// Verify --identity flag exists.
	identityFlag := loginCmd.Flags().Lookup("identity")
	require.NotNil(t, identityFlag)
	assert.Equal(t, "i", identityFlag.Shorthand)
}

func TestLoginCmd_TooManyArgs(t *testing.T) {
	// Test that command returns error when too many arguments provided.
	err := loginCmd.Args(loginCmd, []string{"arg1", "arg2"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at most 1 arg")
}

func TestLoginCmd_MaxOneArg(t *testing.T) {
	// Test that command accepts at most 1 argument.
	assert.Nil(t, loginCmd.Args(loginCmd, []string{}))
	assert.Nil(t, loginCmd.Args(loginCmd, []string{"arg1"}))
	assert.NotNil(t, loginCmd.Args(loginCmd, []string{"arg1", "arg2"}))
}

func TestLoginCmd_NoArgsError(t *testing.T) {
	// Verify the error sentinel exists. Behavioral testing requires a full
	// config + auth manager setup which is covered by integration tests.
	assert.NotNil(t, errUtils.ErrECRLoginNoArgs)
}

func TestCreateAuthManager_EmptyConfig(t *testing.T) {
	// Test with nil maps (not empty maps) to verify zero-value behavior.
	authConfig := &schema.AuthConfig{
		Realm: "test-realm",
	}

	manager, err := createAuthManager(authConfig, "")
	require.NoError(t, err)
	assert.NotNil(t, manager)
}

func TestCreateAuthManager_WithProviders(t *testing.T) {
	// Test with config containing providers.
	authConfig := &schema.AuthConfig{
		Realm: "test-realm",
		Providers: map[string]schema.Provider{
			"test-provider": {
				Kind: "mock",
			},
		},
		Identities:   map[string]schema.Identity{},
		Integrations: map[string]schema.Integration{},
	}

	manager, err := createAuthManager(authConfig, "")
	require.NoError(t, err)
	assert.NotNil(t, manager)
}

func TestCreateAuthManager_WithIntegrations(t *testing.T) {
	// Test with config containing integrations.
	authConfig := &schema.AuthConfig{
		Realm:     "test-realm",
		Providers: map[string]schema.Provider{},
		Identities: map[string]schema.Identity{
			"test-identity": {
				Kind: "aws/user",
			},
		},
		Integrations: map[string]schema.Integration{
			"ecr/test": {
				Kind: "aws/ecr",
				Via: &schema.IntegrationVia{
					Identity: "test-identity",
				},
			},
		},
	}

	manager, err := createAuthManager(authConfig, "")
	require.NoError(t, err)
	assert.NotNil(t, manager)
}

func TestLoginCmd_FlagDefaults(t *testing.T) {
	// Verify --registry flag has empty array default.
	registryFlag := loginCmd.Flags().Lookup("registry")
	require.NotNil(t, registryFlag)
	assert.Equal(t, "[]", registryFlag.DefValue) // Empty array stringified.
	assert.Equal(t, "stringArray", registryFlag.Value.Type())
}

func TestLoginCmd_CommandStructure(t *testing.T) {
	// Verify command is properly configured.
	assert.NotNil(t, loginCmd.Args) // Args validator is set.
	assert.NotNil(t, loginCmd.RunE)
	assert.False(t, loginCmd.FParseErrWhitelist.UnknownFlags)
}

func TestLoginCmd_ParentIsEcrCmd(t *testing.T) {
	// Verify that login is a child of ecr command.
	require.NotNil(t, loginCmd.Parent())
	assert.Equal(t, "ecr", loginCmd.Parent().Name())
}

func TestLoginCmd_LongDescription(t *testing.T) {
	// Verify long description contains expected content.
	assert.Contains(t, loginCmd.Long, "named integration")
	assert.Contains(t, loginCmd.Long, "--identity")
	assert.Contains(t, loginCmd.Long, "--registry")
	assert.Contains(t, loginCmd.Long, "Examples:")
}

func TestLoginCmd_ExamplesInLong(t *testing.T) {
	// Verify examples are included in long description.
	assert.Contains(t, loginCmd.Long, "atmos aws ecr login dev/ecr")
	assert.Contains(t, loginCmd.Long, "atmos aws ecr login --identity dev-admin")
	assert.Contains(t, loginCmd.Long, "atmos aws ecr login --registry")
}

func TestLoginCmd_HelpArgIsNotTreatedAsIntegration(t *testing.T) {
	// Verify "help" positional arg shows help instead of being treated as integration name.
	// The command should return nil (help was displayed) rather than an integration error.
	err := executeLoginCommand(loginCmd, []string{"help"})
	assert.NoError(t, err)
}

func TestExecuteWithAuthManager_NoArgs(t *testing.T) {
	// No identity and no integration — should return ErrECRLoginNoArgs.
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Realm: "test",
		},
	}
	err := executeWithAuthManager(context.Background(), atmosConfig, "", "")
	assert.ErrorIs(t, err, errUtils.ErrECRLoginNoArgs)
}

func TestExecuteWithAuthManager_SelectSentinelNoTTY(t *testing.T) {
	// __SELECT__ sentinel now triggers the interactive identity selector. With no
	// TTY (the test environment), it degrades to ErrIdentitySelectionRequiresTTY.
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Realm: "test",
		},
	}
	err := executeWithAuthManager(context.Background(), atmosConfig, cfg.IdentityFlagSelectValue, "")
	assert.ErrorIs(t, err, errUtils.ErrIdentitySelectionRequiresTTY)
}

func TestExecuteWithAuthManager_MutuallyExclusiveFlags(t *testing.T) {
	// Both integration name and --identity should be rejected.
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Realm: "test",
		},
	}
	err := executeWithAuthManager(context.Background(), atmosConfig, "some-identity", "some-integration")
	assert.ErrorIs(t, err, errUtils.ErrMutuallyExclusiveFlags)
}

func TestLoginCmd_HasPublicFlag(t *testing.T) {
	// Verify --public flag exists with a bool false default.
	publicFlag := loginCmd.Flags().Lookup("public")
	require.NotNil(t, publicFlag)
	assert.Equal(t, "bool", publicFlag.Value.Type())
	assert.Equal(t, "false", publicFlag.DefValue)
}

func TestLoginCmd_PublicExamplesInLong(t *testing.T) {
	// Verify --public usage and examples are documented in the long description.
	assert.Contains(t, loginCmd.Long, "--public")
	assert.Contains(t, loginCmd.Long, "atmos aws ecr login --public")
	assert.Contains(t, loginCmd.Long, "atmos aws ecr login --public --identity dev-admin")
}

func TestValidateLoginModes(t *testing.T) {
	tests := []struct {
		name            string
		public          bool
		integrationName string
		registries      []string
		wantErr         error
	}{
		{
			name:   "not public is always allowed",
			public: false,
		},
		{
			name:            "not public with integration arg is allowed",
			public:          false,
			integrationName: "dev/ecr",
		},
		{
			name:   "public alone (ambient) is allowed",
			public: true,
		},
		{
			name:            "public with integration arg is rejected",
			public:          true,
			integrationName: "ecr-public",
			wantErr:         errUtils.ErrMutuallyExclusiveFlags,
		},
		{
			name:       "public with registry is rejected",
			public:     true,
			registries: []string{"123456789012.dkr.ecr.us-east-2.amazonaws.com"},
			wantErr:    errUtils.ErrMutuallyExclusiveFlags,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLoginModes(tt.public, tt.integrationName, tt.registries)
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestExecutePublicLoginWithIdentity_SelectSentinelNoTTY(t *testing.T) {
	// __SELECT__ sentinel now triggers the interactive identity selector. With no
	// TTY (the test environment), it degrades to ErrIdentitySelectionRequiresTTY
	// before any auth occurs.
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{
			Realm: "test",
		},
	}
	err := executePublicLoginWithIdentity(context.Background(), atmosConfig, cfg.IdentityFlagSelectValue)
	assert.ErrorIs(t, err, errUtils.ErrIdentitySelectionRequiresTTY)
}

func TestResolveSelectedIdentity(t *testing.T) {
	tests := []struct {
		name         string
		identityName string
		setupMock    func(*authTypes.MockAuthManager)
		wantIdentity string
		wantErrIs    error
		wantExitCode int // -1 when no exit code is expected.
	}{
		{
			name:         "concrete name passes through without prompting",
			identityName: "dev-admin",
			setupMock: func(m *authTypes.MockAuthManager) {
				// Selector must NOT be invoked for an explicit identity.
				m.EXPECT().GetDefaultIdentity(gomock.Any()).Times(0)
			},
			wantIdentity: "dev-admin",
			wantExitCode: -1,
		},
		{
			name:         "sentinel resolves to the selected identity",
			identityName: cfg.IdentityFlagSelectValue,
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetDefaultIdentity(true).Return("picked-id", nil)
			},
			wantIdentity: "picked-id",
			wantExitCode: -1,
		},
		{
			name:         "user abort surfaces a SIGINT exit code",
			identityName: cfg.IdentityFlagSelectValue,
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetDefaultIdentity(true).Return("", errUtils.ErrUserAborted)
			},
			wantErrIs:    errUtils.ErrUserAborted,
			wantExitCode: errUtils.ExitCodeSIGINT,
		},
		{
			name:         "no TTY wraps ErrDefaultIdentity and the TTY error",
			identityName: cfg.IdentityFlagSelectValue,
			setupMock: func(m *authTypes.MockAuthManager) {
				m.EXPECT().GetDefaultIdentity(true).Return("", errUtils.ErrIdentitySelectionRequiresTTY)
			},
			wantErrIs:    errUtils.ErrIdentitySelectionRequiresTTY,
			wantExitCode: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockManager := authTypes.NewMockAuthManager(ctrl)
			tt.setupMock(mockManager)

			got, err := resolveSelectedIdentity(auth.AuthManager(mockManager), tt.identityName)

			if tt.wantErrIs != nil {
				assert.ErrorIs(t, err, tt.wantErrIs)
				if tt.wantExitCode >= 0 {
					assert.Equal(t, tt.wantExitCode, errUtils.GetExitCode(err))
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantIdentity, got)
		})
	}
}

// errUserAbortedWrapping documents that the SIGINT-coded abort still satisfies
// errors.Is for ErrUserAborted (guards against future wrapping regressions).
func TestResolveSelectedIdentity_AbortWrapsSentinel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := authTypes.NewMockAuthManager(ctrl)
	mockManager.EXPECT().GetDefaultIdentity(true).Return("", errUtils.ErrUserAborted)

	_, err := resolveSelectedIdentity(auth.AuthManager(mockManager), cfg.IdentityFlagSelectValue)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrUserAborted))
}
