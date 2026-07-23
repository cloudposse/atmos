package acr

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	azureCloud "github.com/cloudposse/atmos/pkg/auth/cloud/azure"
	"github.com/cloudposse/atmos/pkg/auth/cloud/docker"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCreateAuthManager_Success(t *testing.T) {
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
	_, err := createAuthManager(nil, "")
	require.Error(t, err)
}

func TestLoginCmd_Help(t *testing.T) {
	assert.Equal(t, "login [integration]", loginCmd.Use)
	assert.Equal(t, "Login to Azure Container Registry", loginCmd.Short)
	assert.Contains(t, loginCmd.Long, "Azure Container Registry")
}

func TestLoginCmd_HasRegistryFlag(t *testing.T) {
	registryFlag := loginCmd.Flags().Lookup("registry")
	require.NotNil(t, registryFlag)
	assert.Equal(t, "r", registryFlag.Shorthand)
}

func TestLoginCmd_HasIdentityFlag(t *testing.T) {
	identityFlag := loginCmd.Flags().Lookup("identity")
	require.NotNil(t, identityFlag)
	assert.Equal(t, "i", identityFlag.Shorthand)
}

func TestLoginCmd_NoPublicFlag(t *testing.T) {
	// ACR has no ECR-Public equivalent — the flag should not exist.
	publicFlag := loginCmd.Flags().Lookup("public")
	assert.Nil(t, publicFlag)
}

func TestLoginCmd_TooManyArgs(t *testing.T) {
	err := loginCmd.Args(loginCmd, []string{"arg1", "arg2"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "at most 1 arg")
}

func TestLoginCmd_MaxOneArg(t *testing.T) {
	assert.Nil(t, loginCmd.Args(loginCmd, []string{}))
	assert.Nil(t, loginCmd.Args(loginCmd, []string{"arg1"}))
	assert.NotNil(t, loginCmd.Args(loginCmd, []string{"arg1", "arg2"}))
}

func TestLoginCmd_NoArgsError(t *testing.T) {
	assert.NotNil(t, errUtils.ErrACRLoginNoArgs)
}

func TestLoginCmd_FlagDefaults(t *testing.T) {
	registryFlag := loginCmd.Flags().Lookup("registry")
	require.NotNil(t, registryFlag)
	assert.Equal(t, "[]", registryFlag.DefValue)
	assert.Equal(t, "stringArray", registryFlag.Value.Type())
}

func TestLoginCmd_CommandStructure(t *testing.T) {
	assert.NotNil(t, loginCmd.Args)
	assert.NotNil(t, loginCmd.RunE)
	assert.False(t, loginCmd.FParseErrWhitelist.UnknownFlags)
}

func TestLoginCmd_ParentIsAcrCmd(t *testing.T) {
	require.NotNil(t, loginCmd.Parent())
	assert.Equal(t, "acr", loginCmd.Parent().Name())
}

func TestLoginCmd_LongDescription(t *testing.T) {
	assert.Contains(t, loginCmd.Long, "named integration")
	assert.Contains(t, loginCmd.Long, "--identity")
	assert.Contains(t, loginCmd.Long, "--registry")
	assert.Contains(t, loginCmd.Long, "Examples:")
}

func TestLoginCmd_ExamplesInLong(t *testing.T) {
	assert.Contains(t, loginCmd.Long, "atmos azure acr login dev/acr")
	assert.Contains(t, loginCmd.Long, "atmos azure acr login --identity dev-admin")
	assert.Contains(t, loginCmd.Long, "atmos azure acr login --registry")
}

func TestLoginCmd_HelpArgIsNotTreatedAsIntegration(t *testing.T) {
	err := executeLoginCommand(loginCmd, []string{"help"})
	assert.NoError(t, err)
}

func TestExecuteWithAuthManager_NoArgs(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{Realm: "test"},
	}
	err := executeWithAuthManager(context.Background(), atmosConfig, "", "")
	assert.ErrorIs(t, err, errUtils.ErrACRLoginNoArgs)
}

func TestExecuteWithAuthManager_SelectSentinelNoTTY(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{Realm: "test"},
	}
	err := executeWithAuthManager(context.Background(), atmosConfig, cfg.IdentityFlagSelectValue, "")
	assert.ErrorIs(t, err, errUtils.ErrIdentitySelectionRequiresTTY)
}

func TestExecuteWithAuthManager_MutuallyExclusiveFlags(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{Realm: "test"},
	}
	err := executeWithAuthManager(context.Background(), atmosConfig, "some-identity", "some-integration")
	assert.ErrorIs(t, err, errUtils.ErrMutuallyExclusiveFlags)
}

func TestResolveSelectedIdentity(t *testing.T) {
	tests := []struct {
		name         string
		identityName string
		setupMock    func(*authTypes.MockAuthManager)
		wantIdentity string
		wantErrIs    error
		wantExitCode int
	}{
		{
			name:         "concrete name passes through without prompting",
			identityName: "dev-admin",
			setupMock: func(m *authTypes.MockAuthManager) {
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

func TestResolveSelectedIdentity_AbortWrapsSentinel(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockManager := authTypes.NewMockAuthManager(ctrl)
	mockManager.EXPECT().GetDefaultIdentity(true).Return("", errUtils.ErrUserAborted)

	_, err := resolveSelectedIdentity(auth.AuthManager(mockManager), cfg.IdentityFlagSelectValue)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrUserAborted))
}

// stubAzureSeams replaces the package-level Azure/auth-manager seams with the
// given stubs and restores the originals when the test ends. Pass nil to
// leave a seam at its real implementation.
func stubAzureSeams(
	t *testing.T,
	loadCreds func(ctx context.Context) (*authTypes.AzureCredentials, error),
	token func(ctx context.Context, creds authTypes.ICredentials, loginServer string) (*azureCloud.ACRAuthResult, error),
) {
	t.Helper()

	origLoad := loadDefaultAzureCredentials
	origToken := getAuthorizationToken
	t.Cleanup(func() {
		loadDefaultAzureCredentials = origLoad
		getAuthorizationToken = origToken
	})

	if loadCreds != nil {
		loadDefaultAzureCredentials = loadCreds
	}
	if token != nil {
		getAuthorizationToken = token
	}
}

// stubCreateAuthManager replaces the createAuthManager seam with one that
// returns the given manager, restoring the original when the test ends.
func stubCreateAuthManager(t *testing.T, mgr auth.AuthManager) {
	t.Helper()

	orig := createAuthManager
	t.Cleanup(func() { createAuthManager = orig })
	createAuthManager = func(_ *schema.AuthConfig, _ string) (auth.AuthManager, error) {
		return mgr, nil
	}
}

func validACRAuthResult() *azureCloud.ACRAuthResult {
	return &azureCloud.ACRAuthResult{
		Username:  "00000000-0000-0000-0000-000000000000",
		Password:  "test-refresh-token",
		Registry:  "myregistry.azurecr.io",
		ExpiresAt: time.Now().Add(3 * time.Hour),
	}
}

func TestExecuteExplicitRegistries(t *testing.T) {
	const reg1 = "myregistry.azurecr.io"
	const reg2 = "otherregistry.azurecr.io"

	okCreds := func(_ context.Context) (*authTypes.AzureCredentials, error) {
		return &authTypes.AzureCredentials{TenantID: "tenant-123"}, nil
	}

	t.Run("multi-registry success", func(t *testing.T) {
		dockerDir := t.TempDir()
		t.Setenv("DOCKER_CONFIG", dockerDir)
		stubAzureSeams(
			t, okCreds,
			func(_ context.Context, _ authTypes.ICredentials, loginServer string) (*azureCloud.ACRAuthResult, error) {
				return &azureCloud.ACRAuthResult{
					Username:  "00000000-0000-0000-0000-000000000000",
					Password:  "token-" + loginServer,
					Registry:  loginServer,
					ExpiresAt: time.Now().Add(3 * time.Hour),
				}, nil
			},
		)

		err := executeExplicitRegistries(context.Background(), []string{reg1, reg2})
		require.NoError(t, err)

		mgr, err := docker.NewConfigManager()
		require.NoError(t, err)
		registries, err := mgr.GetAuthenticatedRegistries()
		require.NoError(t, err)
		assert.Contains(t, registries, reg1)
		assert.Contains(t, registries, reg2)
	})

	t.Run("credentials load error", func(t *testing.T) {
		stubAzureSeams(
			t,
			func(_ context.Context) (*authTypes.AzureCredentials, error) {
				return nil, fmt.Errorf("no credentials")
			},
			nil,
		)

		err := executeExplicitRegistries(context.Background(), []string{reg1})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no credentials")
	})

	t.Run("invalid registry URL", func(t *testing.T) {
		stubAzureSeams(t, okCreds, nil)

		err := executeExplicitRegistries(context.Background(), []string{"not-a-registry"})
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrACRInvalidRegistry)
	})

	t.Run("auth token error", func(t *testing.T) {
		dockerDir := t.TempDir()
		t.Setenv("DOCKER_CONFIG", dockerDir)
		stubAzureSeams(
			t, okCreds,
			func(_ context.Context, _ authTypes.ICredentials, _ string) (*azureCloud.ACRAuthResult, error) {
				return nil, fmt.Errorf("access denied")
			},
		)

		err := executeExplicitRegistries(context.Background(), []string{reg1})
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrACRLoginFailed)
	})
}

func TestExecuteWithAuthManager_NamedIntegration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockManager := authTypes.NewMockAuthManager(ctrl)
	mockManager.EXPECT().ExecuteIntegration(gomock.Any(), "dev/acr").Return(nil)
	stubCreateAuthManager(t, mockManager)

	atmosConfig := &schema.AtmosConfiguration{Auth: schema.AuthConfig{Realm: "test"}}
	err := executeWithAuthManager(context.Background(), atmosConfig, "", "dev/acr")
	require.NoError(t, err)
}

func TestExecuteWithAuthManager_IdentityIntegrations(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockManager := authTypes.NewMockAuthManager(ctrl)
	mockManager.EXPECT().GetDefaultIdentity(gomock.Any()).Times(0)
	mockManager.EXPECT().ExecuteIdentityIntegrations(gomock.Any(), "azure-dev").Return(nil)
	stubCreateAuthManager(t, mockManager)

	atmosConfig := &schema.AtmosConfiguration{Auth: schema.AuthConfig{Realm: "test"}}
	err := executeWithAuthManager(context.Background(), atmosConfig, "azure-dev", "")
	require.NoError(t, err)
}

// newTestLoginCmd builds a command with the same flags as loginCmd so dispatch
// tests can exercise executeLoginCommand without mutating the global command.
func newTestLoginCmd() *cobra.Command {
	c := &cobra.Command{Use: "login", RunE: executeLoginCommand}
	c.Flags().StringP("identity", "i", "", "")
	c.Flags().StringArrayP("registry", "r", nil, "")
	return c
}

func TestExecuteLoginCommand_Dispatch(t *testing.T) {
	const reg = "myregistry.azurecr.io"

	t.Run("explicit registry routes to executeExplicitRegistries", func(t *testing.T) {
		dockerDir := t.TempDir()
		t.Setenv("DOCKER_CONFIG", dockerDir)
		stubAzureSeams(
			t,
			func(_ context.Context) (*authTypes.AzureCredentials, error) {
				return &authTypes.AzureCredentials{TenantID: "tenant-123"}, nil
			},
			func(_ context.Context, _ authTypes.ICredentials, _ string) (*azureCloud.ACRAuthResult, error) {
				return validACRAuthResult(), nil
			},
		)

		c := newTestLoginCmd()
		require.NoError(t, c.Flags().Set("registry", reg))
		err := executeLoginCommand(c, nil)
		require.NoError(t, err)
	})
}
