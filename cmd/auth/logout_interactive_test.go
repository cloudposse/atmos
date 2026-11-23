package auth

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// forceInteractive pins the cmd/auth interactivity check for the test.
func forceInteractive(t *testing.T, interactive bool) {
	t.Helper()
	orig := isInteractiveFn
	t.Cleanup(func() { isInteractiveFn = orig })
	isInteractiveFn = func() bool { return interactive }
}

// stdinAtEOF replaces os.Stdin with a pipe that is already closed for writing,
// the state of a CI job or `atmos ... < /dev/null`. A selection prompt that is
// shown anyway reads EOF immediately and, before the fix, picked its first option.
func stdinAtEOF(t *testing.T) {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	require.NoError(t, w.Close())
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = orig
		_ = r.Close()
	})
}

// TestPerformInteractiveLogout_WithoutTTYFailsInsteadOfPicking verifies that a
// plain `atmos auth logout` without a terminal fails with a TTY-required error
// and never logs anything out. Before the fix the selection form ran anyway and
// logged out whichever option came first.
func TestPerformInteractiveLogout_WithoutTTYFailsInsteadOfPicking(t *testing.T) {
	forceInteractive(t, false)
	stdinAtEOF(t)

	ctrl := gomock.NewController(t)
	m := authTypes.NewMockAuthManager(ctrl)
	m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
		"prod-admin": {Kind: "aws/permission-set"},
		"dev-admin":  {Kind: "aws/permission-set"},
	}).AnyTimes()
	m.EXPECT().GetProviders().Return(map[string]schema.Provider{
		"sso": {Kind: "aws/iam-identity-center"},
	}).AnyTimes()
	// No Logout, LogoutProvider or LogoutAll expectations: any call fails the test.

	err := performInteractiveLogout(context.Background(), m, false, false, false)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTTYRequired)
	assert.ErrorIs(t, err, errUtils.ErrLogoutSelectionRequiresTTY)
}

// TestPerformInteractiveLogout_NoIdentitiesSkipsTTYCheck verifies the negative
// path: with nothing configured there is nothing to choose, so the command
// still succeeds without a terminal.
func TestPerformInteractiveLogout_NoIdentitiesSkipsTTYCheck(t *testing.T) {
	forceInteractive(t, false)

	ctrl := gomock.NewController(t)
	m := authTypes.NewMockAuthManager(ctrl)
	m.EXPECT().GetIdentities().Return(map[string]schema.Identity{})
	m.EXPECT().GetProviders().Return(map[string]schema.Provider{})

	require.NoError(t, performInteractiveLogout(context.Background(), m, false, false, false))
}

// TestPerformSelectIdentityLogout_WithoutTTYFails verifies that a bare
// `--identity` uses the identity selector, which fails with the same
// TTY-required error other bare `--identity` commands return.
func TestPerformSelectIdentityLogout_WithoutTTYFails(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := authTypes.NewMockAuthManager(ctrl)
	m.EXPECT().GetDefaultIdentity(true).Return("", errUtils.ErrIdentitySelectionRequiresTTY)
	// No Logout expectation: any call fails the test.

	err := performSelectIdentityLogout(context.Background(), m, false, false, false)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIdentitySelectionRequiresTTY)
	assert.ErrorIs(t, err, errUtils.ErrTTYRequired)
}

// TestPerformSelectIdentityLogout_SelectedIdentityIsLoggedOut verifies the
// interactive path: the identity chosen in the selector is the one logged out.
func TestPerformSelectIdentityLogout_SelectedIdentityIsLoggedOut(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := authTypes.NewMockAuthManager(ctrl)
	m.EXPECT().GetDefaultIdentity(true).Return("dev-admin", nil)
	m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
		"dev-admin":  {Kind: "aws/permission-set"},
		"prod-admin": {Kind: "aws/permission-set"},
	})
	m.EXPECT().GetProviderForIdentity("dev-admin").Return("aws-sso")
	m.EXPECT().GetFilesDisplayPath("aws-sso").Return("/atmos/creds")

	// Dry run: the chosen identity is previewed and nothing is removed.
	require.NoError(t, performSelectIdentityLogout(context.Background(), m, true, false, false))
}

// TestBuildLogoutOptions_SortedOrder verifies the logout menu lists identities,
// then providers, each sorted by name, then the All option, so the order is the
// same on every run (Go map iteration is random).
func TestBuildLogoutOptions_SortedOrder(t *testing.T) {
	identities := map[string]schema.Identity{"zeta": {}, "alpha": {}, "mid": {}}
	providers := map[string]schema.Provider{"sso-b": {}, "sso-a": {}}

	for range 5 {
		got := buildLogoutOptions(identities, providers)
		require.Len(t, got, 6)
		targets := make([]string, 0, len(got))
		for _, opt := range got {
			targets = append(targets, opt.typ+":"+opt.target)
		}
		assert.Equal(t, []string{
			"identity:alpha", "identity:mid", "identity:zeta",
			"provider:sso-a", "provider:sso-b",
			"all:",
		}, targets)
	}
}

// restorePersistentIdentityFlag restores the inherited --identity flag after the
// test. resetAuthCmdFlags snapshots only the command's own flags, and Cobra merges
// the parent's persistent --identity flag into them during ParseFlags.
func restorePersistentIdentityFlag(t *testing.T) {
	t.Helper()
	flag := authCmd.PersistentFlags().Lookup(IdentityFlagName)
	require.NotNil(t, flag)
	value, changed := flag.Value.String(), flag.Changed
	t.Cleanup(func() {
		_ = flag.Value.Set(value)
		flag.Changed = changed
	})
}

// writeTwoIdentityFixture writes an atmos.yaml with two mock identities and no
// default, isolates XDG and keyring state, and chdirs into it.
func writeTwoIdentityFixture(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("ATMOS_KEYRING_TYPE", "memory")
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("XDG_DATA_HOME", tmp)
	t.Setenv("XDG_CACHE_HOME", tmp)

	atmosYaml := `base_path: "./"
stacks:
  base_path: stacks
  included_paths: ["**/*"]
  name_pattern: "{stage}"
auth:
  providers:
    mock-provider:
      kind: mock/aws
  identities:
    mock-identity:
      kind: mock/aws
      via:
        provider: mock-provider
    mock-identity-2:
      kind: mock/aws
      via:
        provider: mock-provider
logs:
  level: Info
`
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "atmos.yaml"), []byte(atmosYaml), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "stacks"), 0o755))
	t.Chdir(tmp)
	viper.Reset()
	t.Cleanup(viper.Reset)
}

// TestExecuteAuthLogoutCommand_BareIdentityWithoutTTYFails is the end-to-end
// regression for a bare `atmos auth logout --identity` in CI with stdin at EOF:
// it must fail with the identity-selection TTY error instead of logging out a
// randomly ordered first identity and exiting 0.
func TestExecuteAuthLogoutCommand_BareIdentityWithoutTTYFails(t *testing.T) {
	writeTwoIdentityFixture(t)
	t.Setenv("CI", "true")
	forceInteractive(t, false)
	stdinAtEOF(t)

	cmd := authLogoutCmd
	resetAuthCmdFlags(t, cmd)
	restorePersistentIdentityFlag(t)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.ParseFlags([]string{"--identity"}))
	require.Equal(t, IdentityFlagSelectValue, GetIdentityFromFlags(cmd), "precondition: a bare --identity requests selection")

	err := executeAuthLogoutCommand(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrIdentitySelectionRequiresTTY)
}

// TestExecuteAuthLogoutCommand_PlainLogoutWithoutTTYFails covers a plain
// `atmos auth logout` (no selector) without a terminal end to end.
func TestExecuteAuthLogoutCommand_PlainLogoutWithoutTTYFails(t *testing.T) {
	writeTwoIdentityFixture(t)
	t.Setenv("CI", "true")
	forceInteractive(t, false)
	stdinAtEOF(t)

	cmd := authLogoutCmd
	resetAuthCmdFlags(t, cmd)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.ParseFlags(nil))

	err := executeAuthLogoutCommand(cmd, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrTTYRequired)
}

// TestPerformIdentityLogout_CaseInsensitiveIdentity is the regression for `auth logout`
// failing with "identity not found" for a differently-cased identity name: the name
// resolves to the configured identity, which is the one logged out.
func TestPerformIdentityLogout_CaseInsensitiveIdentity(t *testing.T) {
	ctrl := gomock.NewController(t)
	m := authTypes.NewMockAuthManager(ctrl)
	m.EXPECT().GetIdentities().Return(map[string]schema.Identity{
		"prod-admin": {Kind: "aws/permission-set"},
		"dev-admin":  {Kind: "aws/permission-set"},
	})
	m.EXPECT().GetProviderForIdentity("prod-admin").Return("aws-sso")
	m.EXPECT().GetRealm().Return(realmInfoMatcher())
	m.EXPECT().Logout(gomock.Any(), "prod-admin", false).Return(nil)

	require.NoError(t, performIdentityLogout(context.Background(), m, "Prod-Admin", false, false, false))
}

// TestCanonicalIdentityKey covers exact, case-insensitive, and unknown names.
func TestCanonicalIdentityKey(t *testing.T) {
	identities := map[string]schema.Identity{"prod-admin": {}, "dev-admin": {}}
	assert.Equal(t, "prod-admin", canonicalIdentityKey(identities, "prod-admin"))
	assert.Equal(t, "prod-admin", canonicalIdentityKey(identities, "PROD-Admin"))
	assert.Equal(t, "Unknown", canonicalIdentityKey(identities, "Unknown"), "an unknown name is returned unchanged")
	assert.Equal(t, "", canonicalIdentityKey(nil, ""))
}
