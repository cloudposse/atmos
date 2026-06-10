package credentials

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

// assertProRoundTrip stores and retrieves Atmos Pro credentials and asserts all fields survive.
func assertProRoundTrip(t *testing.T, s types.CredentialStore, alias string) {
	t.Helper()
	in := &types.ProCredentials{Token: "hdr.payload.", BaseURL: "https://pro", Endpoint: "api/v1", WorkspaceID: "ws-1", Provider: "atmos-pro"}
	assert.NoError(t, s.Store(alias, in, "realmA"))

	got, err := s.Retrieve(alias, "realmA")
	assert.NoError(t, err)
	out, ok := got.(*types.ProCredentials)
	if assert.True(t, ok) {
		assert.Equal(t, in.Token, out.Token)
		assert.Equal(t, in.BaseURL, out.BaseURL)
		assert.Equal(t, in.Endpoint, out.Endpoint)
		assert.Equal(t, in.WorkspaceID, out.WorkspaceID)
		assert.Equal(t, in.Provider, out.Provider)
	}
}

func TestStoreRetrieve_Pro_FileStore(t *testing.T) {
	t.Setenv("ATMOS_KEYRING_TYPE", "file")
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password")
	t.Setenv("ATMOS_XDG_DATA_HOME", t.TempDir())

	assertProRoundTrip(t, NewCredentialStore(), "atmos-pro-file")
}

func TestStoreRetrieve_Pro_SystemStore(t *testing.T) {
	// The system keyring uses the in-memory mock backend (keyring.MockInit in package init).
	t.Setenv("ATMOS_KEYRING_TYPE", "system")

	assertProRoundTrip(t, NewCredentialStore(), "atmos-pro-system")
}
