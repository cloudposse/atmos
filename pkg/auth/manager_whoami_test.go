package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/realm"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ambientStubIdentity mimics the generic `ambient` identity contract for
// manager-level tests: Authenticate returns nil credentials, Environment
// returns a small, deterministic map, and GetProviderName returns
// "ambient". The identity is defined in the ambient package, but we want
// to exercise the manager's buildWhoamiInfo without importing ambient
// into the auth package (import cycle via types).
type ambientStubIdentity struct {
	env map[string]string
}

func (a ambientStubIdentity) Kind() string                     { return "ambient" }
func (a ambientStubIdentity) GetProviderName() (string, error) { return "ambient", nil }
func (a ambientStubIdentity) Authenticate(_ context.Context, _ types.ICredentials) (types.ICredentials, error) {
	// (nil, nil) is the bug's epicenter: the generic ambient kind does
	// not manage credentials by design. The regression reproduced in
	// TestManager_Authenticate_Ambient_Standalone depends on this
	// exact return.
	return nil, nil
}
func (a ambientStubIdentity) Validate() error { return nil }
func (a ambientStubIdentity) Environment() (map[string]string, error) {
	if a.env == nil {
		return map[string]string{}, nil
	}
	return a.env, nil
}
func (a ambientStubIdentity) Paths() ([]types.Path, error) { return []types.Path{}, nil }
func (a ambientStubIdentity) PostAuthenticate(_ context.Context, _ *types.PostAuthenticateParams) error {
	return nil
}
func (a ambientStubIdentity) Logout(_ context.Context) error  { return nil }
func (a ambientStubIdentity) CredentialsExist() (bool, error) { return true, nil }
func (a ambientStubIdentity) LoadCredentials(_ context.Context) (types.ICredentials, error) {
	return nil, nil
}

func (a ambientStubIdentity) PrepareEnvironment(_ context.Context, environ map[string]string) (map[string]string, error) {
	return environ, nil
}
func (a ambientStubIdentity) SetRealm(_ string) {}

// TestManager_buildWhoamiInfo_NilCredentials reproduces the SIGSEGV
// reported when authenticating a standalone generic `ambient` identity.
// The generic ambient kind returns (nil, nil) from Authenticate by
// design, and manager.Authenticate forwards those nil credentials to
// buildWhoamiInfo. Before the fix, buildWhoamiInfo dereferenced the
// nil interface via creds.BuildWhoamiInfo(info).
func TestManager_buildWhoamiInfo_NilCredentials(t *testing.T) {
	store := &testStore{data: map[string]any{}}
	ident := ambientStubIdentity{env: map[string]string{"AWS_REGION": "us-east-1"}}
	m := &manager{
		credentialStore: store,
		identities:      map[string]types.Identity{"passthrough": ident},
		realm:           realm.RealmInfo{Value: "test-realm", Source: realm.SourceConfig},
	}

	// Must not panic on nil credentials.
	require.NotPanics(t, func() {
		info := m.buildWhoamiInfo("passthrough", nil)
		require.NotNil(t, info)

		assert.Equal(t, "passthrough", info.Identity)
		assert.Equal(t, "ambient", info.Provider)
		assert.Equal(t, "test-realm", info.Realm)
		assert.Equal(t, realm.SourceConfig, info.RealmSource)

		// Environment from the identity should still be populated —
		// callers (e.g. `atmos auth whoami`) rely on this even when
		// Atmos is a pure passthrough.
		assert.Equal(t, "us-east-1", info.Environment["AWS_REGION"])

		// No credentials, no keystore cache, no reference handle.
		assert.Nil(t, info.Credentials)
		assert.Empty(t, info.CredentialsRef)
		assert.Empty(t, store.data, "nothing should be written to the credential store for nil-creds identities")
	})
}

// TestManager_Authenticate_Ambient_Standalone exercises the full
// Authenticate() entry point with a real manager and a registered
// ambient identity. Before the fix, this path panicked at
// buildWhoamiInfo; after the fix it returns a populated WhoamiInfo.
func TestManager_Authenticate_Ambient_Standalone(t *testing.T) {
	cfg := &schema.AuthConfig{
		Realm: "test-realm",
		Identities: map[string]schema.Identity{
			"passthrough": {Kind: "ambient"},
		},
	}

	authManager, err := NewAuthManager(cfg, &testStore{data: map[string]any{}}, dummyValidator{}, nil, "")
	require.NoError(t, err)

	require.NotPanics(t, func() {
		info, err := authManager.Authenticate(context.Background(), "passthrough")
		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "passthrough", info.Identity)
		assert.Equal(t, "ambient", info.Provider)
		assert.Nil(t, info.Credentials, "generic ambient identity does not manage credentials")
	})
}
