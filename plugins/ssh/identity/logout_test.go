package identity_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/ssh/authority"
	"github.com/cloudposse/atmos/plugins/ssh/authority/authoritytest"
	"github.com/cloudposse/atmos/plugins/ssh/identity"
	"github.com/cloudposse/atmos/plugins/ssh/keystore"
)

func (r *rig) logout(name string, spec map[string]any) error {
	r.t.Helper()
	return r.svc.Logout(context.Background(), name, spec)
}

func TestLogoutWithALiveKeyLeavesItEnrolledAndRemovesTheAgentEntry(t *testing.T) {
	r := newRig(t)
	r.mustAuthenticate("ops", baseSpec(nil))
	pub := r.pub("ops")
	require.Len(t, r.agentConfig().Identities, 1)
	require.FileExists(t, r.gitConfigPath("ops"))

	require.NoError(t, r.logout("ops", baseSpec(nil)))

	assert.Equal(t, 1, r.fake.Count(pub, authority.UseSigning), "a live key stays enrolled")
	assert.Len(t, r.receipts(), 1)
	assert.Empty(t, r.agentConfig().Identities, "the socket goes away with the agent entry")
	assert.NoFileExists(t, r.gitConfigPath("ops"))
	assert.Equal(t, 0, r.fake.CallCount("revoke"))
	st := r.status("ops")
	assert.True(t, st.Success)
	assert.Equal(t, "logout", st.Operation)
}

func TestLogoutRevokesTheEnrollmentOfAKeyThatIsGone(t *testing.T) {
	r := newRig(t)
	r.mustAuthenticate("ops", baseSpec(map[string]any{"allow": []any{"signing", "authentication"}}))
	pub := r.pub("ops")
	require.NoError(t, r.store().Delete(t.Context(), "ops"))

	require.NoError(t, r.logout("ops", baseSpec(map[string]any{"allow": []any{"signing", "authentication"}})))

	assert.Zero(t, r.fake.Count(pub, authority.UseSigning))
	assert.Zero(t, r.fake.Count(pub, authority.UseAuthentication))
	assert.Empty(t, r.receipts(), "the receipt goes once the enrollment is withdrawn")
	assert.Equal(t, []string{"ops"}, r.status("ops").Revoked)
	assert.Empty(t, r.agentConfig().Identities)
}

func TestLogoutRevokesOnlyReceiptsWhoseKeyIsGone(t *testing.T) {
	r := newRig(t)
	r.mustAuthenticate("gone", baseSpec(nil))
	r.mustAuthenticate("kept", baseSpec(nil))
	gone, kept := r.pub("gone"), r.pub("kept")
	require.NoError(t, r.store().Delete(t.Context(), "gone"))

	require.NoError(t, r.logout("kept", baseSpec(nil)))

	assert.Zero(t, r.fake.Count(gone, authority.UseSigning), "the missing key's enrollment was withdrawn")
	assert.Equal(t, 1, r.fake.Count(kept, authority.UseSigning), "the live key stays")
	receipts := r.receipts()
	require.Len(t, receipts, 1)
	assert.Equal(t, "kept", receipts[0].KeyName)
}

func TestLogoutUsesTheTokenTheIdentitySaved(t *testing.T) {
	r := newRig(t)
	r.mustAuthenticate("ops", baseSpec(nil))
	require.NoError(t, r.store().Delete(t.Context(), "ops"))
	require.Equal(t, authoritytest.FakeToken, string(r.state.Data[identity.StateTokenKey]))

	require.NoError(t, r.logout("ops", baseSpec(nil)))

	assert.Equal(t, 1, r.fake.CallCount("revoke"))
}

func TestLogoutWithoutASavedTokenReportsWhatItCouldNotRevoke(t *testing.T) {
	r := newRig(t)
	r.mustAuthenticate("ops", baseSpec(nil))
	require.NoError(t, r.store().Delete(t.Context(), "ops"))
	r.state.Data = map[string][]byte{}

	err := r.logout("ops", baseSpec(nil))

	require.Error(t, err)
	assert.Len(t, r.receipts(), 1, "the receipt stays so a later login or logout can retry")
	assert.Empty(t, r.agentConfig().Identities, "the agent entry is removed anyway")
	assert.False(t, r.status("ops").Success)
	assert.NotEmpty(t, r.status("ops").Error)
}

func TestLogoutDoesNotRevokeWhenTheKeyStoresCannotAllBeListed(t *testing.T) {
	r := newRig(t)
	r.mustAuthenticate("ops", baseSpec(nil))
	require.NoError(t, r.store().Delete(t.Context(), "ops"))
	opener := r.deps.OpenStore
	r.deps.OpenStore = func(kind, dir string) (keystore.Store, error) {
		if kind == "secure-enclave" {
			return nil, os.ErrPermission
		}
		return opener(kind, dir)
	}
	r.use()

	require.NoError(t, r.logout("ops", baseSpec(nil)))

	assert.Zero(t, r.fake.CallCount("revoke"), "an unknown set of keys revokes nothing")
	assert.Len(t, r.receipts(), 1)
	assert.NotEmpty(t, r.status("ops").Warnings)
}

func TestLogoutWithAnInvalidSpecStillRemovesTheAgentEntry(t *testing.T) {
	r := newRig(t)
	r.mustAuthenticate("ops", baseSpec(nil))

	require.NoError(t, r.logout("ops", map[string]any{"nonsense": true}))

	assert.Empty(t, r.agentConfig().Identities)
	assert.Contains(t, r.status("ops").Warnings[0], "enrollments were not checked")
}

func TestLogoutOfAnIdentityThatNeverLoggedInIsHarmless(t *testing.T) {
	r := newRig(t)

	require.NoError(t, r.logout("ops", baseSpec(nil)))

	assert.Zero(t, r.fake.CallCount("revoke"))
}
