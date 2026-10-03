package user

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/credentials"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard so a rename of the identity fields used below fails the build.
var _ = schema.Identity{Kind: authTypes.ProviderKindAWSUser}

// newConfigureTestCmd builds a command with the same --identity flag shape as the auth parent
// command (a string flag with the select sentinel as NoOptDefVal) and parses args into it.
func newConfigureTestCmd(t *testing.T, args ...string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "configure", RunE: executeAuthUserConfigureCommand}
	cmd.Flags().StringP(cfg.IdentityFlagName, "i", "", "identity")
	cmd.Flags().Lookup(cfg.IdentityFlagName).NoOptDefVal = cfg.IdentityFlagSelectValue
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.ParseFlags(args))
	return cmd
}

// stubPrompts replaces the terminal decision and the interactive helpers for one test.
func stubPrompts(t *testing.T, interactiveOK bool, selector func([]string) (string, error)) {
	t.Helper()
	origAvail, origSelect := interactiveAvailable, selectIdentityFunc
	t.Cleanup(func() { interactiveAvailable, selectIdentityFunc = origAvail, origSelect })
	interactiveAvailable = func() bool { return interactiveOK }
	if selector == nil {
		selector = func([]string) (string, error) {
			t.Fatal("identity selector must not be shown")
			return "", nil
		}
	}
	selectIdentityFunc = selector
}

func TestResolveIdentityToConfigure(t *testing.T) {
	identities := map[string]schema.Identity{
		"ft-user":  {Kind: "aws/user"},
		"ft-user2": {Kind: "aws/user"},
		"ft-role":  {Kind: "aws/assume-role"},
	}
	selectable := []string{"ft-user", "ft-user2"}

	t.Run("explicit identity skips the selector even without a terminal", func(t *testing.T) {
		stubPrompts(t, false, nil)
		got, err := resolveIdentityToConfigure(newConfigureTestCmd(t, "--identity=ft-user2"), viper.New(), selectable, identities)
		require.NoError(t, err)
		assert.Equal(t, "ft-user2", got)
	})

	t.Run("identity from the environment skips the selector", func(t *testing.T) {
		stubPrompts(t, true, nil)
		v := viper.New()
		v.Set(cfg.IdentityFlagName, "ft-user")
		got, err := resolveIdentityToConfigure(newConfigureTestCmd(t), v, selectable, identities)
		require.NoError(t, err)
		assert.Equal(t, "ft-user", got)
	})

	t.Run("flag wins over the environment", func(t *testing.T) {
		stubPrompts(t, false, nil)
		v := viper.New()
		v.Set(cfg.IdentityFlagName, "ft-user")
		got, err := resolveIdentityToConfigure(newConfigureTestCmd(t, "--identity=ft-user2"), v, selectable, identities)
		require.NoError(t, err)
		assert.Equal(t, "ft-user2", got)
	})

	t.Run("unknown identity lists the aws/user identities", func(t *testing.T) {
		stubPrompts(t, true, nil)
		_, err := resolveIdentityToConfigure(newConfigureTestCmd(t, "--identity=nope"), viper.New(), selectable, identities)
		require.ErrorIs(t, err, errUtils.ErrIdentityNotFound)
		hints := cockroach.GetAllHints(err)
		require.NotEmpty(t, hints)
		assert.Contains(t, hints[0], "ft-user, ft-user2")
	})

	t.Run("non aws/user identity is rejected with a hint", func(t *testing.T) {
		stubPrompts(t, true, nil)
		_, err := resolveIdentityToConfigure(newConfigureTestCmd(t, "--identity=ft-role"), viper.New(), selectable, identities)
		require.ErrorIs(t, err, errUtils.ErrInvalidIdentityKind)
		hints := cockroach.GetAllHints(err)
		require.NotEmpty(t, hints)
		assert.Contains(t, hints[0], "ft-user, ft-user2")
		assert.Contains(t, hints[len(hints)-1], "--identity=ft-user")
	})

	t.Run("--identity=false is rejected instead of reported as not found", func(t *testing.T) {
		stubPrompts(t, true, nil)
		_, err := resolveIdentityToConfigure(newConfigureTestCmd(t, "--identity=false"), viper.New(), selectable, identities)
		require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
		require.NotErrorIs(t, err, errUtils.ErrIdentityNotFound)
		hints := cockroach.GetAllHints(err)
		require.NotEmpty(t, hints)
		assert.Contains(t, hints[0], "--identity=ft-user")
	})

	t.Run("false-like ATMOS_IDENTITY is rejected too", func(t *testing.T) {
		stubPrompts(t, true, nil)
		v := viper.New()
		v.Set(cfg.IdentityFlagName, "off")
		_, err := resolveIdentityToConfigure(newConfigureTestCmd(t), v, selectable, identities)
		require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
	})

	t.Run("hints stay safe when no aws/user identity exists", func(t *testing.T) {
		stubPrompts(t, false, nil)
		_, err := resolveIdentityToConfigure(newConfigureTestCmd(t, "--identity=nope"), viper.New(), nil, identities)
		require.ErrorIs(t, err, errUtils.ErrIdentityNotFound)
		_, err = resolveIdentityToConfigure(newConfigureTestCmd(t), viper.New(), nil, identities)
		require.ErrorIs(t, err, errUtils.ErrIdentitySelectionRequiresTTY)
		assert.Contains(t, cockroach.GetAllHints(err)[0], "--identity=IDENTITY")
	})

	t.Run("no identity without a terminal fails fast with a hint", func(t *testing.T) {
		stubPrompts(t, false, nil)
		_, err := resolveIdentityToConfigure(newConfigureTestCmd(t), viper.New(), selectable, identities)
		require.ErrorIs(t, err, errUtils.ErrIdentitySelectionRequiresTTY)
		require.ErrorIs(t, err, errUtils.ErrTTYRequired)
		hints := cockroach.GetAllHints(err)
		require.NotEmpty(t, hints)
		assert.Contains(t, hints[0], "atmos auth user configure --identity=ft-user")
	})

	t.Run("no identity with a terminal shows the selector", func(t *testing.T) {
		var offered []string
		stubPrompts(t, true, func(options []string) (string, error) {
			offered = options
			return "ft-user2", nil
		})
		got, err := resolveIdentityToConfigure(newConfigureTestCmd(t), viper.New(), selectable, identities)
		require.NoError(t, err)
		assert.Equal(t, "ft-user2", got)
		assert.Equal(t, selectable, offered)
	})

	t.Run("identity flag without a value shows the selector", func(t *testing.T) {
		stubPrompts(t, true, func([]string) (string, error) { return "ft-user", nil })
		got, err := resolveIdentityToConfigure(newConfigureTestCmd(t, "--identity"), viper.New(), selectable, identities)
		require.NoError(t, err)
		assert.Equal(t, "ft-user", got)
	})
}

func TestPromptAndSaveCredentials_FailsFastWithoutTerminal(t *testing.T) {
	stubPrompts(t, false, nil)
	origPrompt := promptForCredentialsFunc
	t.Cleanup(func() { promptForCredentialsFunc = origPrompt })
	promptForCredentialsFunc = func(awsUserIdentityInfo) (*authTypes.AWSCredentials, error) {
		t.Fatal("credential form must not be shown")
		return nil, nil
	}

	err := promptAndSaveCredentials(awsUserIdentityInfo{}, "ft-user", "realm", credentials.NewCredentialStoreWithConfig(&schema.AuthConfig{Keyring: schema.KeyringConfig{Type: "memory"}}))
	require.ErrorIs(t, err, errUtils.ErrAuthPromptUnavailable)
	require.ErrorIs(t, err, errUtils.ErrTTYRequired)
	hints := cockroach.GetAllHints(err)
	require.NotEmpty(t, hints)
	assert.Contains(t, hints[0], "--identity=ft-user")
}

// TestExecuteAuthUserConfigure_UsesConfiguredKeyring is the regression test for configure
// ignoring auth.keyring: the credentials must land in the configured file keyring (and be
// readable through the same store the auth manager builds), not in the system keychain.
func TestExecuteAuthUserConfigure_UsesConfiguredKeyring(t *testing.T) {
	root := t.TempDir()
	keyringDir := filepath.Join(root, "keyring")
	atmosYAML := "auth:\n" +
		"  realm: ft-configure-test\n" +
		"  keyring:\n" +
		"    type: file\n" +
		"    spec:\n" +
		"      path: " + filepath.ToSlash(keyringDir) + "\n" +
		"      password_env: FT_CONFIGURE_KEYRING_PASSWORD\n" +
		"  identities:\n" +
		"    ft-configure-user:\n" +
		"      kind: aws/user\n" +
		"    ft-configure-other:\n" +
		"      kind: aws/user\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte(atmosYAML), 0o600))

	t.Chdir(root)
	t.Setenv("FT_CONFIGURE_KEYRING_PASSWORD", "test-password")
	t.Setenv("ATMOS_KEYRING_TYPE", "")
	t.Setenv("ATMOS_KEYRING_FILE_PATH", "")
	t.Setenv("ATMOS_XDG_CONFIG_HOME", filepath.Join(root, "xdg"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "xdg"))

	stubPrompts(t, true, nil)
	origPrompt := promptForCredentialsFunc
	t.Cleanup(func() { promptForCredentialsFunc = origPrompt })
	promptForCredentialsFunc = func(awsUserIdentityInfo) (*authTypes.AWSCredentials, error) {
		return &authTypes.AWSCredentials{AccessKeyID: "AKIAFAKEFAKEFAKEFAKE", SecretAccessKey: "fake-secret"}, nil
	}

	cmd := newConfigureTestCmd(t, "--identity=ft-configure-user")
	require.NoError(t, executeAuthUserConfigureCommand(cmd, nil))

	entries, err := os.ReadDir(keyringDir)
	require.NoError(t, err, "configure must write to the keyring directory from auth.keyring")
	require.NotEmpty(t, entries)

	// Read it back through the store the auth manager builds from the same config.
	store := credentials.NewCredentialStoreWithConfig(&schema.AuthConfig{Keyring: schema.KeyringConfig{
		Type: "file",
		Spec: map[string]any{"path": keyringDir, "password_env": "FT_CONFIGURE_KEYRING_PASSWORD"},
	}})
	got, err := store.Retrieve("ft-configure-user", "ft-configure-test")
	require.NoError(t, err)
	creds, ok := got.(*authTypes.AWSCredentials)
	require.True(t, ok)
	assert.Equal(t, "AKIAFAKEFAKEFAKEFAKE", creds.AccessKeyID)
}
