package aws

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	atmosCredentials "github.com/cloudposse/atmos/pkg/auth/credentials"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/keyring"
)

// autoRealmTestRealm has the format of the realm Atmos derives from the atmos.yaml path when no
// realm is configured: 16 lowercase hexadecimal characters, never containing "_".
const autoRealmTestRealm = "3ed941021e53d074"

// TestUserIdentity_LegacyKeyring_AutoRealmMigratesWithoutPrompt reproduces regression A2 of the
// 1.235.0 regression test: in CI (no prompt), an aws/user identity whose name contains "_" failed
// with "older keyring key that was not adopted" after the upgrade, although under the realm
// derived from the atmos.yaml path the previous-layout key can only belong to this identity.
// The keys must be used and moved to the current key without asking.
func TestUserIdentity_LegacyKeyring_AutoRealmMigratesWithoutPrompt(t *testing.T) {
	f := newLegacyKeyringFixture(t, false)
	f.identity.SetRealm(autoRealmTestRealm)
	f.prompter.EXPECT().Available().Return(false).AnyTimes()
	f.prompter.EXPECT().ConfirmAdoption(gomock.Any()).Times(0)

	current := atmosCredentials.KeyringRealmPrefix + atmosCredentials.KeyringSeparator + autoRealmTestRealm +
		atmosCredentials.KeyringRealmAliasSeparator + legacyTestIdentity
	legacy := atmosCredentials.KeyringRealmPrefix + atmosCredentials.KeyringSeparator + autoRealmTestRealm +
		atmosCredentials.KeyringSeparator + legacyTestIdentity
	require.NoError(t, f.store.Store(legacyTestIdentity, legacyLongLivedKeys(), autoRealmTestRealm))
	data, err := f.raw.Get(current)
	require.NoError(t, err)
	require.NoError(t, f.raw.Delete(current))
	require.NoError(t, f.raw.Set(legacy, data))

	creds, err := f.identity.Authenticate(context.Background(), nil)
	require.NoError(t, err, "the legacy keys must be used without confirmation under the auto realm")

	session, ok := creds.(*types.AWSCredentials)
	require.True(t, ok)
	assert.Equal(t, "ASIASESSIONEXAMPLE", session.AccessKeyID)
	assert.Equal(t, []string{legacyTestAccessID}, f.sts.signedWith(), "STS must be called with the legacy long-lived keys")

	moved, err := f.raw.Get(current)
	require.NoError(t, err, "the legacy entry must be moved to the current key")
	assert.Equal(t, data, moved)
	_, err = f.raw.Get(legacy)
	require.ErrorIs(t, err, keyring.ErrNotFound, "the legacy key must be removed after the move")
}
