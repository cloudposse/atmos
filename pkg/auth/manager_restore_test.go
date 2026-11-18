package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// restoringIdentity records the credential the manager hands it on the passive path.
type restoringIdentity struct {
	stubIdentity
	restored types.ICredentials
	calls    int
	err      error
}

func (r *restoringIdentity) RestoreCredentials(_ context.Context, creds types.ICredentials) error {
	r.calls++
	r.restored = creds
	return r.err
}

func TestManager_GetCachedCredentials_RestoresStoredCredential(t *testing.T) {
	validTime := time.Now().Add(time.Hour)
	expiredTime := time.Now().Add(-time.Hour)

	newManager := func(id types.Identity, creds *testCreds) *manager {
		store := &testStore{data: map[string]any{"dev": creds}, expired: map[string]bool{}}
		return &manager{
			config:          &schema.AuthConfig{Identities: map[string]schema.Identity{"dev": {Kind: "plugin/kind"}}},
			identities:      map[string]types.Identity{"dev": id},
			credentialStore: store,
		}
	}

	t.Run("a valid stored credential is handed to the identity", func(t *testing.T) {
		id := &restoringIdentity{stubIdentity: stubIdentity{provider: "p"}}
		stored := &testCreds{exp: &validTime}

		_, err := newManager(id, stored).GetCachedCredentials(context.Background(), "dev")
		require.NoError(t, err)
		assert.Equal(t, 1, id.calls)
		assert.Same(t, stored, id.restored)
	})

	t.Run("an expired stored credential is not restored", func(t *testing.T) {
		id := &restoringIdentity{stubIdentity: stubIdentity{provider: "p"}}

		_, err := newManager(id, &testCreds{exp: &expiredTime}).GetCachedCredentials(context.Background(), "dev")
		require.ErrorIs(t, err, errUtils.ErrExpiredCredentials)
		assert.Zero(t, id.calls)
	})

	t.Run("a credential the identity refuses is not a usable cache hit", func(t *testing.T) {
		id := &restoringIdentity{stubIdentity: stubIdentity{provider: "p"}, err: errors.New("refused")}

		_, err := newManager(id, &testCreds{exp: &validTime}).GetCachedCredentials(context.Background(), "dev")
		require.ErrorIs(t, err, errUtils.ErrNoCredentialsFound)
	})

	t.Run("identities without restore support are unaffected", func(t *testing.T) {
		_, err := newManager(stubIdentity{provider: "p"}, &testCreds{exp: &validTime}).GetCachedCredentials(context.Background(), "dev")
		require.NoError(t, err)
	})
}
