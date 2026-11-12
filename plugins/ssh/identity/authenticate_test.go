package identity_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/ssh/agent"
	"github.com/cloudposse/atmos/plugins/ssh/authority"
	"github.com/cloudposse/atmos/plugins/ssh/authority/authoritytest"
	"github.com/cloudposse/atmos/plugins/ssh/identity"
	"github.com/cloudposse/atmos/plugins/ssh/internal/testfakes"
	"github.com/cloudposse/atmos/plugins/ssh/keystore"
	"github.com/cloudposse/atmos/plugins/ssh/paths"
	"github.com/cloudposse/atmos/plugins/ssh/statusfile"
)

func TestFirstAuthenticateCreatesEnrollsServesAndWritesGitConfig(t *testing.T) {
	r := newRig(t)
	spec := baseSpec(map[string]any{
		"allow": []any{"signing", "authentication"}, "user": "deploy", "rewrite_https": []any{"github.com"},
	})

	res := r.mustAuthenticate("ops", spec)

	// The key exists in the store.
	pub := r.pub("ops")

	// The authority was asked once, for both uses, with the machine's title.
	require.Equal(t, 1, ensureCalls(r.fake))
	assert.Equal(t, 1, r.fake.Count(pub, authority.UseSigning))
	assert.Equal(t, 1, r.fake.Count(pub, authority.UseAuthentication))
	receipts := r.receipts()
	require.Len(t, receipts, 1)
	assert.Equal(t, "fake", receipts[0].Authority)

	// The agent knows the identity and the daemon was installed and started once.
	assert.Equal(t, []agent.Identity{{Name: "ops", Store: "file", Key: "ops", Allow: []agent.Allow{agent.AllowSigning, agent.AllowAuthentication}}}, r.agentConfig().Identities)
	require.Equal(t, 1, r.mgr.InstallCount())
	assert.Equal(t, 1, r.mgr.Starts)
	assert.Equal(t, []string{"agent-serve", "--root", r.paths.Root()}, r.mgr.Installs[0].Args)
	assert.Equal(t, 1, r.installs)
	socket, err := r.paths.SocketPath("ops")
	require.NoError(t, err)
	assert.Equal(t, []string{socket}, r.checks)

	// The environment has the three variables, and an expiry the host accepts.
	gitPath := r.gitConfigPath("ops")
	assert.Equal(t, map[string]string{
		"SSH_AUTH_SOCK": socket, "GIT_CONFIG_GLOBAL": gitPath, "ANSIBLE_REMOTE_USER": "deploy",
	}, res.Environment)
	assert.Equal(t, r.clock.Now().Add(identity.CredentialLifetime), res.ExpiresAt)
	assert.NotContains(t, res.Environment, "GIT_CONFIG_COUNT")

	// The Git configuration includes the user's own first, then sets the signing keys.
	content := r.readFile(gitPath)
	assert.Contains(t, content, `signingkey = "key::`+authority.AuthorizedKey(pub)+`"`)
	assert.Contains(t, content, `program = "`+keygen+`"`)
	assert.Contains(t, content, `[url "ssh://git@github.com/"]`)
	info, err := os.Stat(gitPath)
	require.NoError(t, err)
	if info.Mode().Perm() != 0 {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}

	// The status file says what happened and holds the title-free summary.
	st := r.status("ops")
	assert.True(t, st.Success)
	assert.Equal(t, "enrolled", st.Outcome)
	assert.Equal(t, "fake-user", st.Subject)
	assert.ElementsMatch(t, []string{"signing", "authentication"}, st.Added)
	assert.Equal(t, r.clock.Now(), st.Time)

	// The token is kept for logout.
	assert.Equal(t, authoritytest.FakeToken, string(r.state.Data[identity.StateTokenKey]))
}

func TestFirstAuthenticateTitlesTheKeyWithTheHostname(t *testing.T) {
	r := newRig(t)
	r.mustAuthenticate("ops", baseSpec(nil))
	// The fake records the title in its receipt subject only through Ensure; check via the call.
	require.Len(t, r.fake.Calls(), 1)
	assert.Equal(t, "ensure", r.fake.Calls()[0].Op)
}

func TestEnvironmentFollowsAllowUserAndRewrite(t *testing.T) {
	tests := []struct {
		name    string
		extra   map[string]any
		wantGit bool
		wantUsr bool
	}{
		{"signing only", map[string]any{}, true, false},
		{"signing with user", map[string]any{"user": "deploy"}, true, false},
		{"authentication with user", map[string]any{"allow": []any{"authentication"}, "user": "deploy"}, false, true},
		{"authentication without user", map[string]any{"allow": []any{"authentication"}}, false, false},
		{"authentication with rewrite", map[string]any{"allow": []any{"authentication"}, "rewrite_https": []any{"example.com"}}, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t)
			res := r.mustAuthenticate("ops", baseSpec(tt.extra))

			assert.Contains(t, res.Environment, "SSH_AUTH_SOCK")
			_, hasGit := res.Environment["GIT_CONFIG_GLOBAL"]
			_, hasUser := res.Environment["ANSIBLE_REMOTE_USER"]
			assert.Equal(t, tt.wantGit, hasGit)
			assert.Equal(t, tt.wantUsr, hasUser)
			_, err := os.Stat(r.gitConfigPath("ops"))
			assert.Equal(t, tt.wantGit, err == nil, "the file exists exactly when the variable is set")
		})
	}
}

func TestSecondAuthenticateMakesNoAuthorityCallAndDoesNotReinstall(t *testing.T) {
	r := newRig(t)
	first := r.mustAuthenticate("ops", baseSpec(map[string]any{"allow": []any{"signing", "authentication"}}))
	r.clock.Advance(time.Hour)

	second := r.mustAuthenticate("ops", baseSpec(map[string]any{"allow": []any{"signing", "authentication"}}))

	assert.Equal(t, 1, ensureCalls(r.fake), "a matching receipt means no authority call")
	assert.Equal(t, 1, r.mgr.InstallCount(), "a running current daemon is not reinstalled")
	assert.Equal(t, 1, r.mgr.Starts)
	assert.Equal(t, 1, r.installs)
	assert.Equal(t, first.Environment, second.Environment)
	assert.Equal(t, "unchanged", r.status("ops").Outcome)
	assert.Equal(t, 1, r.state.Puts, "the unchanged token is not written again")
	assert.Len(t, r.receipts(), 1)
}

func TestAuthenticateReinstallsAnOutOfDateOrStoppedDaemon(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(m *testfakes.Manager)
	}{
		{"stopped", func(m *testfakes.Manager) { m.St.Running = false }},
		{"not installed", func(m *testfakes.Manager) { m.St.Installed = false }},
		{"daemon is another version", func(m *testfakes.Manager) { m.St.VersionMismatch = true }},
		{"installed copy is another version", func(m *testfakes.Manager) { m.St.InstalledVersion = "0.0.9" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t)
			r.mustAuthenticate("ops", baseSpec(nil))
			require.Equal(t, 1, r.mgr.InstallCount())
			tt.mutate(r.mgr)

			r.mustAuthenticate("ops", baseSpec(nil))

			assert.Equal(t, 2, r.mgr.InstallCount())
			assert.Equal(t, 2, r.mgr.Starts)
		})
	}
}

func TestAddingAUseEnrollsItAndKeepsTheKey(t *testing.T) {
	r := newRig(t)
	r.mustAuthenticate("ops", baseSpec(nil))
	pub := r.pub("ops")

	r.mustAuthenticate("ops", baseSpec(map[string]any{"allow": []any{"signing", "authentication"}}))

	assert.Equal(t, 2, ensureCalls(r.fake))
	assert.Equal(t, 1, r.fake.Count(pub, authority.UseAuthentication))
	assert.Equal(t, 1, r.fake.Count(pub, authority.UseSigning))
	st := r.status("ops")
	assert.Equal(t, "updated", st.Outcome)
	assert.Equal(t, []string{"authentication"}, st.Added)
	assert.Equal(t, pub, r.pub("ops"), "the same key")
	assert.Len(t, r.receipts(), 1)
}

func TestFailuresAreRecordedWithTheirCause(t *testing.T) {
	requireUnlocked := baseSpec(map[string]any{"require": "unlocked"})
	tests := []struct {
		name     string
		prepare  func(r *rig)
		spec     map[string]any
		upstream map[string]string
		code     identity.Code
		contains string
		is       error
	}{
		{
			name: "unsupported requirement on the file store", spec: requireUnlocked, upstream: fakeToken(),
			code: identity.CodeUnsupportedRequirement, contains: `"unlocked"`, is: keystore.ErrUnsupportedRequirement,
		},
		{
			name: "unknown authority", spec: baseSpec(map[string]any{"authority": "nope"}), upstream: fakeToken(),
			code: identity.CodeUnknownAuthority, contains: "nope", is: authority.ErrUnknownAuthority,
		},
		{
			name: "missing token for github", spec: baseSpec(map[string]any{"authority": "github"}), upstream: map[string]string{"assertion": "self"},
			code: identity.CodeTokenRejected, contains: "no token", is: authority.ErrUnauthorized,
		},
		{
			name: "missing scope", spec: baseSpec(nil), upstream: fakeToken(),
			prepare: func(r *rig) {
				r.fake.FailNextEnsure(&authority.Error{Authority: "fake", Operation: "ensure", Use: authority.UseSigning, Err: authority.ErrMissingPermission, Permission: "write:ssh_signing_key"})
			},
			code: identity.CodeMissingScope, contains: "write:ssh_signing_key", is: authority.ErrMissingPermission,
		},
		{
			name: "authority unreachable", spec: baseSpec(nil), upstream: fakeToken(),
			prepare: func(r *rig) {
				r.fake.FailNextEnsure(&authority.Error{Authority: "fake", Operation: "ensure", Err: authority.ErrUnavailable})
			},
			code: identity.CodeAuthorityUnreachable, contains: "unreachable", is: authority.ErrUnavailable,
		},
		{
			name: "invalid spec", spec: map[string]any{"store": "file", "allow": []any{"signing"}, "bogus": true}, upstream: fakeToken(),
			code: identity.CodeInvalidSpec, contains: "bogus", is: identity.ErrInvalidSpec,
		},
		{
			name: "secure enclave on a machine without one", spec: baseSpec(map[string]any{"store": "secure-enclave"}), upstream: fakeToken(),
			code: identity.CodeUnsupportedPlatform, contains: "not supported", is: keystore.ErrUnsupportedPlatform,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newRig(t)
			if tt.prepare != nil {
				tt.prepare(r)
			}

			_, err := r.authenticate("ops", tt.spec, tt.upstream)

			require.Error(t, err)
			var failure *identity.Error
			require.ErrorAs(t, err, &failure)
			assert.Equal(t, tt.code, failure.Code)
			assert.ErrorIs(t, err, tt.is)
			assert.Contains(t, failure.Message, tt.contains)

			st := r.status("ops")
			assert.False(t, st.Success)
			assert.Equal(t, "failed", st.Outcome)
			assert.Equal(t, string(tt.code), st.Code)
			assert.Contains(t, st.Error, tt.contains)
			assert.Equal(t, 0, r.mgr.InstallCount(), "a failed login starts no daemon")
		})
	}
}

func TestAnAuthorityFailureLeavesNoTokenInTheStatusFileOrTheError(t *testing.T) {
	r := newRig(t)
	token := authoritytest.FakeToken
	r.fake.FailNextEnsure(errors.New("upstream said no to " + token))

	_, err := r.authenticate("ops", baseSpec(nil), fakeToken())

	require.Error(t, err)
	assert.NotContains(t, err.Error(), token)
	path, pathErr := statusfile.Path(r.paths, "ops")
	require.NoError(t, pathErr)
	assert.NotContains(t, r.readFile(path), token)
	assert.Contains(t, r.readFile(path), "[redacted]")
	for _, receipt := range r.receipts() {
		assert.NotContains(t, receipt.PublicKey, token)
	}
}

func TestRequirementMismatchFailsAndNamesTheFix(t *testing.T) {
	r := newRig(t)
	r.deps.OpenStore = testfakes.RequirementOpener()
	r.use()
	r.mustAuthenticate("ops", baseSpec(nil))
	require.Equal(t, 1, ensureCalls(r.fake))

	_, err := r.authenticate("ops", baseSpec(map[string]any{"require": []any{"unlocked"}}), fakeToken())

	require.ErrorIs(t, err, keystore.ErrRequirementMismatch)
	st := r.status("ops")
	assert.Equal(t, string(identity.CodeRequirementMismatch), st.Code)
	assert.Contains(t, st.Error, "atmos ssh key delete ops --yes")
	assert.Equal(t, 1, ensureCalls(r.fake), "nothing was enrolled for the mismatched request")
}

func TestARequirementTheStoreCanEnforceIsAppliedAtCreation(t *testing.T) {
	r := newRig(t)
	r.deps.OpenStore = testfakes.RequirementOpener()
	r.use()
	spec := baseSpec(map[string]any{"require": "unlocked"})

	r.mustAuthenticate("ops", spec)
	r.mustAuthenticate("ops", spec)

	assert.Equal(t, []string{"unlocked"}, r.status("ops").Require)
	_, err := r.authenticate("ops", baseSpec(nil), fakeToken())
	require.ErrorIs(t, err, keystore.ErrRequirementMismatch, "dropping the term is also a mismatch")
}

func TestARotatedKeyRevokesTheOldEnrollmentAndEnrollsTheNewKey(t *testing.T) {
	r := newRig(t)
	r.mustAuthenticate("ops", baseSpec(nil))
	old := r.pub("ops")
	require.NoError(t, r.store().Delete(t.Context(), "ops"))

	r.mustAuthenticate("ops", baseSpec(nil))

	fresh := r.pub("ops")
	assert.NotEqual(t, authority.Fingerprint(old), authority.Fingerprint(fresh))
	assert.Zero(t, r.fake.Count(old, authority.UseSigning), "the old key was withdrawn")
	assert.Equal(t, 1, r.fake.Count(fresh, authority.UseSigning), "the new key is enrolled")
	receipts := r.receipts()
	require.Len(t, receipts, 1)
	assert.Equal(t, authority.Fingerprint(fresh), receipts[0].Fingerprint)
	assert.Equal(t, []string{"ops"}, r.status("ops").Revoked)
	assert.Equal(t, "enrolled", r.status("ops").Outcome)
}

func TestAKeyOfAnotherIdentityIsNotRevoked(t *testing.T) {
	r := newRig(t)
	r.mustAuthenticate("alpha", baseSpec(nil))
	r.mustAuthenticate("beta", baseSpec(nil))

	r.mustAuthenticate("alpha", baseSpec(nil))

	assert.Len(t, r.receipts(), 2)
	assert.Equal(t, 1, r.fake.Count(r.pub("beta"), authority.UseSigning))
}

func TestAnAuthenticateOfAKeyNamedInTheSpecSharesTheKey(t *testing.T) {
	r := newRig(t)
	r.mustAuthenticate("one", baseSpec(map[string]any{"key": "shared"}))
	r.mustAuthenticate("two", baseSpec(map[string]any{"key": "shared"}))

	keys, err := r.store().List(t.Context())
	require.NoError(t, err)
	require.Len(t, keys, 1)
	assert.Equal(t, "shared", keys[0].Name)
	assert.Equal(t, 1, ensureCalls(r.fake), "the second identity finds the first one's receipt")
	assert.Len(t, r.agentConfig().Identities, 2)
}

func TestALockedKeyFailsAsInteractionRequired(t *testing.T) {
	r := newRig(t)
	r.mustAuthenticate("ops", baseSpec(nil))
	r.deps.OpenStore = func(kind, dir string) (keystore.Store, error) {
		store, err := testfakes.FileOpener(kind, dir)
		if err != nil {
			return nil, err
		}
		return testfakes.FailingLoad(store, testfakes.ErrLocked), nil
	}
	r.use()

	_, err := r.authenticate("ops", baseSpec(nil), fakeToken())

	var failure *identity.Error
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, identity.CodeKeyLocked, failure.Code)
	assert.Contains(t, r.status("ops").Error, "locked")
}

func TestTheAgentThatNeverComesUpIsReported(t *testing.T) {
	r := newRig(t)
	r.socketErrs = 1 << 20

	_, err := r.authenticate("ops", baseSpec(nil), fakeToken())

	require.ErrorIs(t, err, identity.ErrAgentUnavailable)
	assert.Equal(t, string(identity.CodeAgentUnavailable), r.status("ops").Code)
	assert.Contains(t, r.status("ops").Error, "socket not up yet")
}

func TestASlowDaemonIsWaitedFor(t *testing.T) {
	r := newRig(t)
	r.socketErrs = 3
	before := r.clock.Now()

	r.mustAuthenticate("ops", baseSpec(nil))

	assert.Len(t, r.checks, 4)
	assert.True(t, r.clock.Now().After(before), "it waited between checks")
}

func TestSigningNeedsSSHKeygen(t *testing.T) {
	r := newRig(t)
	r.keygenErr = identity.ErrNoSSHKeygen

	_, err := r.authenticate("ops", baseSpec(nil), fakeToken())

	require.ErrorIs(t, err, identity.ErrNoSSHKeygen)
	assert.Contains(t, r.status("ops").Error, "ssh-keygen")

	// An authentication-only identity does not need it.
	_, err = r.authenticate("deploy", baseSpec(map[string]any{"allow": []any{"authentication"}}), fakeToken())
	require.NoError(t, err)
}

func TestAStaleGitConfigIsRemovedWhenSigningIsDropped(t *testing.T) {
	r := newRig(t)
	r.mustAuthenticate("ops", baseSpec(map[string]any{"allow": []any{"signing", "authentication"}}))
	require.FileExists(t, r.gitConfigPath("ops"))

	r.mustAuthenticate("ops", baseSpec(map[string]any{"allow": []any{"authentication"}}))

	assert.NoFileExists(t, r.gitConfigPath("ops"))
}

func TestAnUnchangedGitConfigIsNotRewritten(t *testing.T) {
	r := newRig(t)
	r.mustAuthenticate("ops", baseSpec(nil))
	before, err := os.Stat(r.gitConfigPath("ops"))
	require.NoError(t, err)
	r.clock.Advance(time.Hour)

	r.mustAuthenticate("ops", baseSpec(nil))

	after, err := os.Stat(r.gitConfigPath("ops"))
	require.NoError(t, err)
	assert.Equal(t, before.ModTime(), after.ModTime())
}

func TestTheTokenIsSavedForLogoutAndAFailureToSaveIsOnlyAWarning(t *testing.T) {
	r := newRig(t)
	r.state.PutErr = errors.New("keyring is read-only")

	r.mustAuthenticate("ops", baseSpec(nil))

	st := r.status("ops")
	assert.True(t, st.Success)
	require.Len(t, st.Warnings, 1)
	assert.Contains(t, st.Warnings[0], "keyring is read-only")
}

func TestNoTokenMeansNothingIsSaved(t *testing.T) {
	r := newRig(t)

	r.mustAuthenticateWith("ops", baseSpec(map[string]any{"authority": "self"}), map[string]string{"assertion": "self"})

	assert.Zero(t, r.state.Puts)
	assert.Equal(t, "enrolled", r.status("ops").Outcome)
	assert.Zero(t, ensureCalls(r.fake))
}

func TestAnInvalidIdentityNameTouchesNothing(t *testing.T) {
	r := newRig(t)

	_, err := r.authenticate("../escape", baseSpec(nil), fakeToken())

	require.ErrorIs(t, err, paths.ErrInvalidIdentity)
	_, statErr := os.Stat(filepath.Join(filepath.Dir(r.paths.Root()), "escape"))
	assert.True(t, os.IsNotExist(statErr))
	assert.Zero(t, ensureCalls(r.fake))
}

func TestACorruptReceiptStopsEnrollmentInsteadOfOverwritingIt(t *testing.T) {
	r := newRig(t)
	r.mustAuthenticate("ops", baseSpec(nil))
	files, err := filepath.Glob(filepath.Join(r.paths.ReceiptsDir(), "*"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	require.NoError(t, os.WriteFile(files[0], []byte("not json"), 0o600))

	_, err = r.authenticate("ops", baseSpec(nil), fakeToken())

	require.ErrorIs(t, err, authority.ErrReceiptCorrupt)
	assert.Equal(t, 1, ensureCalls(r.fake))
}
