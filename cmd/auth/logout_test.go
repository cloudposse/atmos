package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/realm"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestBuildKeychainDeletionMessage covers the pure formatter for the keychain
// confirmation prompt.
func TestBuildKeychainDeletionMessage(t *testing.T) {
	got := buildKeychainDeletionMessage("prod-admin")
	// The identity name must appear in the prompt.
	assert.Contains(t, got, "prod-admin")
	// Key bullets the user is asked to confirm must be present.
	assert.Contains(t, got, "Access keys")
	assert.Contains(t, got, "Service account")
	assert.Contains(t, got, "Provider credentials")
	assert.Contains(t, got, "Session data")
}

// TestConfirmKeychainDeletion_ForceShortCircuit asserts that --force bypasses
// the prompt entirely and confirms.
func TestConfirmKeychainDeletion_ForceShortCircuit(t *testing.T) {
	confirmed, err := confirmKeychainDeletion("prod-admin", true /*force*/, false /*isTTY*/)
	require.NoError(t, err)
	assert.True(t, confirmed, "--force must short-circuit confirmation regardless of TTY state")
}

// TestConfirmKeychainDeletion_NonTTYWithoutForceErrors asserts that running
// without a TTY and without --force surfaces ErrKeychainDeletionRequiresConfirmation.
func TestConfirmKeychainDeletion_NonTTYWithoutForceErrors(t *testing.T) {
	confirmed, err := confirmKeychainDeletion("prod-admin", false /*force*/, false /*isTTY*/)
	require.Error(t, err)
	assert.False(t, confirmed)
	assert.ErrorIs(t, err, errUtils.ErrKeychainDeletionRequiresConfirmation)
}

// TestDetectExternalCredentials covers the read-only detection of provider env
// vars that may continue to authorise the caller after `atmos auth logout`.
func TestDetectExternalCredentials(t *testing.T) {
	t.Run("no env vars set returns empty", func(t *testing.T) {
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
		t.Setenv("AZURE_CERTIFICATE_PATH", "")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "")

		got := detectExternalCredentials()
		assert.Empty(t, got)
	})

	t.Run("all three providers detected", func(t *testing.T) {
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/tmp/gcp.json")
		t.Setenv("AZURE_CERTIFICATE_PATH", "/tmp/azure.pem")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/tmp/aws-creds")

		got := detectExternalCredentials()
		require.Len(t, got, 3)
		// Each warning should include both the env var name and the value to
		// help the user track down where the creds are coming from.
		assert.Contains(t, got[0], "GOOGLE_APPLICATION_CREDENTIALS")
		assert.Contains(t, got[0], "/tmp/gcp.json")
		assert.Contains(t, got[1], "AZURE_CERTIFICATE_PATH")
		assert.Contains(t, got[1], "/tmp/azure.pem")
		assert.Contains(t, got[2], "AWS_SHARED_CREDENTIALS_FILE")
		assert.Contains(t, got[2], "/tmp/aws-creds")
	})

	t.Run("only AWS env var set", func(t *testing.T) {
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
		t.Setenv("AZURE_CERTIFICATE_PATH", "")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/aws/creds")

		got := detectExternalCredentials()
		require.Len(t, got, 1)
		assert.Contains(t, got[0], "AWS_SHARED_CREDENTIALS_FILE")
	})
}

// TestBuildLogoutOptions covers the pure construction of the interactive
// logout selection list. The output must include every identity, every
// provider, and a final "all" option in that order.
func TestBuildLogoutOptions(t *testing.T) {
	t.Run("empty config still includes the All option", func(t *testing.T) {
		got := buildLogoutOptions(nil, nil)
		require.Len(t, got, 1)
		assert.Equal(t, "all", got[0].typ)
	})

	t.Run("includes one option per identity, one per provider, plus All", func(t *testing.T) {
		identities := map[string]schema.Identity{
			"id-a": {Kind: "aws/permission-set"},
			"id-b": {Kind: "aws/assume-role"},
		}
		providers := map[string]schema.Provider{
			"sso-east": {Kind: "aws/iam-identity-center"},
		}

		got := buildLogoutOptions(identities, providers)
		// 2 identities + 1 provider + 1 "all" = 4 options.
		require.Len(t, got, 4)

		// Last option must always be "all".
		assert.Equal(t, "all", got[len(got)-1].typ,
			"the All option must be last so it appears at the bottom of the picker")

		// Collect the typ → target pairs (order of map iteration is undefined).
		seen := map[string]string{}
		for _, opt := range got {
			seen[opt.typ+":"+opt.target] = opt.label
		}
		assert.Contains(t, seen, "identity:id-a")
		assert.Contains(t, seen, "identity:id-b")
		assert.Contains(t, seen, "provider:sso-east")
		assert.Contains(t, seen, "all:")
	})

	t.Run("provider option label calls out cascade", func(t *testing.T) {
		got := buildLogoutOptions(nil, map[string]schema.Provider{
			"sso": {Kind: "aws/iam-identity-center"},
		})
		require.Len(t, got, 2)
		// First entry is the provider; "all" is last.
		assert.Equal(t, "provider", got[0].typ)
		assert.Contains(t, got[0].label, "removes all identities",
			"provider logout label must warn the user that all identities are affected")
	})
}

// TestExecuteLogoutOption_InvalidType returns ErrInvalidLogoutOption for an
// option type the dispatcher doesn't recognise.
func TestExecuteLogoutOption_InvalidType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := authTypes.NewMockAuthManager(ctrl)

	err := executeLogoutOption(context.Background(), m, logoutOption{typ: "bogus"}, false, false, false)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidLogoutOption)
}

// TestPerformIdentityLogout_NotFound covers the "identity not in config" error
// branch via a mocked AuthManager.
func TestPerformIdentityLogout_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := authTypes.NewMockAuthManager(ctrl)

	// Identity "missing" is not present in the configured identities map.
	m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
		"prod-admin": {Kind: "aws/permission-set"},
	})

	err := performIdentityLogout(context.Background(), m, "missing", false, false, false)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIdentityNotInConfig)
}

// TestPerformIdentityLogout_DryRun covers the dry-run path. No Logout call
// must be made; the function must complete successfully.
func TestPerformIdentityLogout_DryRun(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := authTypes.NewMockAuthManager(ctrl)

	m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
		"prod-admin": {Kind: "aws/permission-set"},
	})
	m.EXPECT().GetProviderForIdentity("prod-admin").Return("aws-sso")
	m.EXPECT().GetFilesDisplayPath("aws-sso").Return("/atmos/realm/creds")
	// Logout must NOT be called in dry-run mode.
	m.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	err := performIdentityLogout(context.Background(), m, "prod-admin", true /*dryRun*/, true /*deleteKeychain*/, false)
	require.NoError(t, err)
}

// TestPerformProviderLogout_NotFound covers the "provider not in config"
// error branch.
func TestPerformProviderLogout_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := authTypes.NewMockAuthManager(ctrl)

	m.EXPECT().GetProviders().Return(map[string]schema.Provider{
		"aws-sso": {Kind: "aws/iam-identity-center"},
	})

	err := performProviderLogout(context.Background(), m, "missing", false, false, false)
	require.Error(t, err)
	// Could be ErrProviderNotInConfig or similar; just verify error surfaces.
	assert.Contains(t, err.Error(), "missing",
		"the missing provider name must appear in the error message")
}

// TestPerformLogoutAll_DryRun covers the dry-run path. LogoutAll must not be
// called.
func TestPerformLogoutAll_DryRun(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := authTypes.NewMockAuthManager(ctrl)

	m.EXPECT().GetProviders().Return(map[string]schema.Provider{
		"aws-sso": {Kind: "aws/iam-identity-center"},
	})
	m.EXPECT().GetFilesDisplayPath("aws-sso").Return("/atmos/realm/creds")
	// LogoutAll must NOT be called in dry-run mode.
	m.EXPECT().LogoutAll(gomock.Any(), gomock.Any()).Times(0)

	err := performLogoutAll(context.Background(), m, true /*dryRun*/, true /*deleteKeychain*/, false)
	require.NoError(t, err)
}

// TestPerformIdentityLogout_Success covers the happy path with no keychain
// deletion (so no confirmation prompt is invoked).
func TestPerformIdentityLogout_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := authTypes.NewMockAuthManager(ctrl)

	m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
		"prod-admin": {Kind: "aws/permission-set"},
	})
	m.EXPECT().GetProviderForIdentity("prod-admin").Return("aws-sso")
	m.EXPECT().GetRealm().Return(realmInfoMatcher())
	m.EXPECT().Logout(gomock.Any(), "prod-admin", false).Return(nil)

	err := performIdentityLogout(context.Background(), m, "prod-admin", false /*dryRun*/, false /*deleteKeychain*/, false)
	require.NoError(t, err)
}

// TestPerformIdentityLogout_PartialLogout covers the ErrPartialLogout branch:
// the function should not surface an error but should print a warning.
func TestPerformIdentityLogout_PartialLogout(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := authTypes.NewMockAuthManager(ctrl)

	m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
		"prod-admin": {Kind: "aws/permission-set"},
	})
	m.EXPECT().GetProviderForIdentity("prod-admin").Return("aws-sso")
	m.EXPECT().GetRealm().Return(realmInfoMatcher())
	m.EXPECT().Logout(gomock.Any(), "prod-admin", false).Return(errUtils.ErrPartialLogout)

	err := performIdentityLogout(context.Background(), m, "prod-admin", false, false, false)
	// Partial logout is treated as success — the function prints a warning
	// but returns nil so the user sees a meaningful summary instead of an error.
	require.NoError(t, err,
		"ErrPartialLogout must be downgraded to success-with-warning")
}

// TestPerformIdentityLogout_LogoutError covers the generic-error path: the
// function must surface the error to the caller (not swallow it).
func TestPerformIdentityLogout_LogoutError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := authTypes.NewMockAuthManager(ctrl)

	boom := errors.New("keyring backend unavailable")
	m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
		"prod-admin": {Kind: "aws/permission-set"},
	})
	m.EXPECT().GetProviderForIdentity("prod-admin").Return("aws-sso")
	m.EXPECT().GetRealm().Return(realmInfoMatcher())
	m.EXPECT().Logout(gomock.Any(), "prod-admin", false).Return(boom)

	err := performIdentityLogout(context.Background(), m, "prod-admin", false, false, false)
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}

// TestPerformProviderLogout_Success covers the happy path.
func TestPerformProviderLogout_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := authTypes.NewMockAuthManager(ctrl)

	m.EXPECT().GetProviders().Return(map[string]schema.Provider{
		"aws-sso": {Kind: "aws/iam-identity-center"},
	})
	m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
		"prod-admin": {Kind: "aws/permission-set"},
		"dev-admin":  {Kind: "aws/permission-set"},
	})
	m.EXPECT().GetProviderForIdentity("prod-admin").Return("aws-sso")
	m.EXPECT().GetProviderForIdentity("dev-admin").Return("aws-sso")
	m.EXPECT().GetRealm().Return(realmInfoMatcher())
	m.EXPECT().LogoutProvider(gomock.Any(), "aws-sso", false).Return(nil)

	err := performProviderLogout(context.Background(), m, "aws-sso", false, false, false)
	require.NoError(t, err)
}

// TestPerformProviderLogout_DryRun covers the dry-run path. LogoutProvider must
// not be called.
func TestPerformProviderLogout_DryRun(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := authTypes.NewMockAuthManager(ctrl)

	m.EXPECT().GetProviders().Return(map[string]schema.Provider{
		"aws-sso": {Kind: "aws/iam-identity-center"},
	})
	m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
		"prod-admin": {Kind: "aws/permission-set"},
	})
	m.EXPECT().GetProviderForIdentity("prod-admin").Return("aws-sso")
	m.EXPECT().GetFilesDisplayPath("aws-sso").Return("/atmos/realm/creds")
	m.EXPECT().LogoutProvider(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	err := performProviderLogout(context.Background(), m, "aws-sso", true /*dryRun*/, false, false)
	require.NoError(t, err)
}

// TestExecuteLogoutOption_DispatchAll covers the "all" branch of the
// dispatcher.
func TestExecuteLogoutOption_DispatchAll(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := authTypes.NewMockAuthManager(ctrl)

	// performLogoutAll dry-run path: short-circuits with no LogoutAll call.
	m.EXPECT().GetProviders().Return(map[string]schema.Provider{})
	m.EXPECT().LogoutAll(gomock.Any(), gomock.Any()).Times(0)

	err := executeLogoutOption(context.Background(), m, logoutOption{typ: "all"}, true, false, false)
	require.NoError(t, err)
}

// TestExecuteLogoutOption_DispatchIdentity covers the "identity" branch.
func TestExecuteLogoutOption_DispatchIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := authTypes.NewMockAuthManager(ctrl)

	// performIdentityLogout dry-run path: needs GetFilesDisplayPath for
	// the "Would remove ... files:" message but does not invoke Logout.
	m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
		"prod-admin": {Kind: "aws/permission-set"},
	})
	m.EXPECT().GetProviderForIdentity("prod-admin").Return("aws-sso")
	m.EXPECT().GetFilesDisplayPath("aws-sso").Return("/atmos/creds")
	m.EXPECT().Logout(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	err := executeLogoutOption(context.Background(), m, logoutOption{typ: "identity", target: "prod-admin"}, true, false, false)
	require.NoError(t, err)
}

// TestExecuteLogoutOption_DispatchProvider covers the "provider" branch.
func TestExecuteLogoutOption_DispatchProvider(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := authTypes.NewMockAuthManager(ctrl)

	m.EXPECT().GetProviders().Return(map[string]schema.Provider{
		"aws-sso": {Kind: "aws/iam-identity-center"},
	})
	m.EXPECT().GetIdentities().Return(map[string]schema.Identity{})
	m.EXPECT().GetFilesDisplayPath("aws-sso").Return("/atmos/creds")
	m.EXPECT().LogoutProvider(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	err := executeLogoutOption(context.Background(), m, logoutOption{typ: "provider", target: "aws-sso"}, true, false, false)
	require.NoError(t, err)
}

// TestPerformInteractiveLogout_NoIdentities covers the early-return branch
// when the auth config has no identities at all.
func TestPerformInteractiveLogout_NoIdentities(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := authTypes.NewMockAuthManager(ctrl)

	m.EXPECT().GetIdentities().Return(map[string]schema.Identity{})
	m.EXPECT().GetProviders().Return(map[string]schema.Provider{})
	// LogoutAll / Logout / huh form must NOT be invoked.

	err := performInteractiveLogout(context.Background(), m, false, false, false)
	require.NoError(t, err,
		"empty-identities branch must short-circuit cleanly without an error")
}

// TestPerformLogoutAllRealms_NoRealms covers the early-return branch where
// the XDG config directory exists but contains no realm subdirectories.
func TestPerformLogoutAllRealms_NoRealms(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpHome)

	atmosCfg := &schema.AtmosConfiguration{}
	err := performLogoutAllRealms(context.Background(), atmosCfg, false, false, false)
	require.NoError(t, err,
		"no-realms branch must return success cleanly")
}

// TestPerformLogoutAllRealms_RealRemove covers the non-dry-run, no-keychain
// path: realm directories must be physically removed.
func TestPerformLogoutAllRealms_RealRemove(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpHome)

	realmAWSDir := filepath.Join(tmpHome, "atmos", "realm-1", "aws")
	require.NoError(t, os.MkdirAll(realmAWSDir, 0o755))

	atmosCfg := &schema.AtmosConfiguration{}
	err := performLogoutAllRealms(context.Background(), atmosCfg, false /*dryRun*/, false /*deleteKeychain*/, false)
	require.NoError(t, err)

	// The realm directory must be gone.
	_, err = os.Stat(filepath.Join(tmpHome, "atmos", "realm-1"))
	assert.True(t, os.IsNotExist(err),
		"non-dry-run logout-all-realms must remove discovered realm directories")
}

// TestPerformLogoutAllRealms_DryRun covers the dry-run branch with discovered
// realms. The realm directories must NOT be removed.
func TestPerformLogoutAllRealms_DryRun(t *testing.T) {
	tmpHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpHome)

	// Create realm directories matching the discoverRealms convention:
	// XDG_CONFIG_HOME/atmos/<realm>/<provider>/.
	require.NoError(t, os.MkdirAll(filepath.Join(tmpHome, "atmos", "realm-1", "aws"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(tmpHome, "atmos", "realm-2", "azure"), 0o755))

	atmosCfg := &schema.AtmosConfiguration{}
	err := performLogoutAllRealms(context.Background(), atmosCfg, true /*dryRun*/, false, false)
	require.NoError(t, err)

	// Dry-run must not remove the realm directories.
	_, err = os.Stat(filepath.Join(tmpHome, "atmos", "realm-1", "aws"))
	require.NoError(t, err, "dry-run must NOT delete realm directories")
	_, err = os.Stat(filepath.Join(tmpHome, "atmos", "realm-2", "azure"))
	require.NoError(t, err)
}

// TestPerformLogoutAll_Success covers the happy path with no keychain
// deletion (so no confirmation prompt is invoked).
func TestPerformLogoutAll_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	m := authTypes.NewMockAuthManager(ctrl)

	m.EXPECT().LogoutAll(gomock.Any(), false).Return(nil)
	m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
		"prod-admin": {Kind: "aws/permission-set"},
		"dev-admin":  {Kind: "aws/permission-set"},
	})
	m.EXPECT().GetRealm().Return(realmInfoMatcher())

	err := performLogoutAll(context.Background(), m, false /*dryRun*/, false /*deleteKeychain*/, false)
	require.NoError(t, err)
}

// realmInfoMatcher returns a stub RealmInfo for use in mock returns.
func realmInfoMatcher() realm.RealmInfo {
	return realm.RealmInfo{Value: "test-realm", Source: "test"}
}

// TestExecuteAuthLogoutCommand_SmokeNoConfig exercises the logout orchestrator
// from a directory without an atmos.yaml. Contract: no panic.
func TestExecuteAuthLogoutCommand_SmokeNoConfig(t *testing.T) {
	tmp := t.TempDir()
	t.Chdir(tmp)

	cmd := authLogoutCmd
	resetAuthCmdFlags(t, cmd)
	cmd.SetContext(context.Background())

	assert.NotPanics(t, func() {
		_ = executeAuthLogoutCommand(cmd, nil)
	})
}

// TestExecuteAuthLogoutCommand_WithMockAuthDryRun exercises logout
// end-to-end against the mock auth fixture in --dry-run mode (no actual
// credential removal). Drives interactive/identity/provider dispatch.
func TestExecuteAuthLogoutCommand_WithMockAuthDryRun(t *testing.T) {
	setupMockAuthFixture(t)

	cmd := authLogoutCmd
	resetAuthCmdFlags(t, cmd)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.ParseFlags([]string{"--dry-run", "mock-identity"}))

	err := executeAuthLogoutCommand(cmd, []string{"mock-identity"})
	assert.NoError(t, err,
		"dry-run logout of a configured identity must succeed")
}

// TestDisplayExternalCredentialWarnings smoke-covers the I/O wrapper around
// detectExternalCredentials. Two branches: warnings present and not.
func TestDisplayExternalCredentialWarnings(t *testing.T) {
	t.Run("no env vars: silent no-op", func(t *testing.T) {
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "")
		t.Setenv("AZURE_CERTIFICATE_PATH", "")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "")
		assert.NotPanics(t, func() {
			displayExternalCredentialWarnings()
		})
	})

	t.Run("env vars present: emits warning section", func(t *testing.T) {
		t.Setenv("GOOGLE_APPLICATION_CREDENTIALS", "/tmp/gcp.json")
		t.Setenv("AZURE_CERTIFICATE_PATH", "")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/tmp/aws-creds")
		assert.NotPanics(t, func() {
			displayExternalCredentialWarnings()
		})
	})
}

// TestDisplayBrowserWarning smoke-covers the deferred-warning helper. The
// cache state is environment-dependent — both `cache hit, skip` and `cache
// miss, write` paths are exercised together by simply ensuring the function
// is non-panicking.
func TestDisplayBrowserWarning(t *testing.T) {
	assert.NotPanics(t, func() {
		displayBrowserWarning()
	})

	// Calling it again should also not panic — the second call hits the
	// "already shown" cache branch.
	assert.NotPanics(t, func() {
		displayBrowserWarning()
	})
}

// TestDiscoverRealms covers the realm-discovery helper used by the multi-realm
// logout flow.
func TestDiscoverRealms(t *testing.T) {
	t.Run("missing base dir returns nil without error", func(t *testing.T) {
		// Deliberately reference a path that does not exist.
		got, err := discoverRealms(filepath.Join(t.TempDir(), "does-not-exist"))
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	t.Run("dirs without provider subdirs are not realms", func(t *testing.T) {
		base := t.TempDir()
		// Create a top-level dir that has NO aws/azure subdir — must not be
		// reported as a realm.
		require.NoError(t, os.MkdirAll(filepath.Join(base, "not-a-realm", "random"), 0o755))

		got, err := discoverRealms(base)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("dirs with aws subdir are reported as realms", func(t *testing.T) {
		base := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(base, "realm-1", "aws"), 0o755))
		require.NoError(t, os.MkdirAll(filepath.Join(base, "realm-2", "azure"), 0o755))
		// File at top-level must be ignored.
		require.NoError(t, os.WriteFile(filepath.Join(base, "stray-file"), []byte("x"), 0o644))

		got, err := discoverRealms(base)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"realm-1", "realm-2"}, got,
			"realms are directories containing an aws/ or azure/ provider subdir")
	})
}
