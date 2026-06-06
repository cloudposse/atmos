package credentials

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/99designs/keyring"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	zkeyring "github.com/zalando/go-keyring"

	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// This file is the STORAGE CONTRACT GATE for extracting the keyring backends into a generic
// pkg/keyring package (see docs/prd/secrets-management.md). It pins the exact on-disk /
// in-keychain layout — keyring key strings, the zalando service/account arguments, and the
// credential-envelope JSON shape — so the refactor is provably non-breaking: these tests are
// written against the CURRENT implementation and MUST keep passing byte-for-byte afterwards.
// If a change here is required, existing users' stored credentials would be silently lost.

// newTestFileStore builds a file-backed keyring store rooted in a temp dir, mirroring the
// setup used across keyring_file_test.go.
func newTestFileStore(t *testing.T) *fileKeyringStore {
	t.Helper()

	tempDir := t.TempDir()
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	authConfig := &schema.AuthConfig{
		Keyring: schema.KeyringConfig{
			Type: "file",
			Spec: map[string]interface{}{
				"path": tempDir,
			},
		},
	}

	store, err := newFileKeyringStore(authConfig)
	require.NoError(t, err)
	return store
}

// TestBuildKeyringKey_ExactFormat pins the realm-scoped key string and the empty-realm
// backward-compatibility fallback. These exact strings are the storage addresses for every
// credential, so any drift orphans existing entries.
func TestBuildKeyringKey_ExactFormat(t *testing.T) {
	tests := []struct {
		name  string
		alias string
		realm string
		want  string
	}{
		{name: "empty realm falls back to atmos_alias", alias: "a", realm: "", want: "atmos_a"},
		{name: "realm-scoped key", alias: "a", realm: "r", want: "atmos_r_a"},
		{name: "alias containing underscores is preserved verbatim", alias: "a_b", realm: "r", want: "atmos_r_a_b"},
		{name: "realm and alias with underscores", alias: "x_y", realm: "p_q", want: "atmos_p_q_x_y"},
		{name: "empty realm with underscore alias", alias: "_leading", realm: "", want: "atmos__leading"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, buildKeyringKey(tt.alias, tt.realm))
		})
	}

	// The constants that compose every key are themselves part of the contract.
	assert.Equal(t, "atmos", KeyringRealmPrefix)
	assert.Equal(t, "_", KeyringSeparator)
	assert.Equal(t, "atmos-auth", KeyringUser)

	// A realm must produce a different key than the empty-realm fallback (isolation guarantee).
	assert.NotEqual(t, buildKeyringKey("a", ""), buildKeyringKey("a", "r"))
}

// TestSystemKeyringStore_ExactServiceAndAccount pins the zalando go-keyring call arguments:
// the "service" is the realm-scoped composed key and the "account" is the KeyringUser constant
// ("atmos-auth"). The stored value is the credential envelope JSON. (keyring.MockInit() is
// installed by the package init in store_test.go.)
func TestSystemKeyringStore_ExactServiceAndAccount(t *testing.T) {
	store, err := newSystemKeyringStore()
	require.NoError(t, err)

	alias := "contract-aws"
	realm := "contract-realm"
	creds := &types.AWSCredentials{
		AccessKeyID:     "AKIA_CONTRACT",
		SecretAccessKey: "SECRET_CONTRACT",
		SessionToken:    "TOKEN_CONTRACT",
		Region:          "us-east-1",
	}
	require.NoError(t, store.Store(alias, creds, realm))

	composedKey := buildKeyringKey(alias, realm)
	require.Equal(t, "atmos_contract-realm_contract-aws", composedKey)

	// The entry must be readable at exactly (service=composedKey, account="atmos-auth").
	raw, err := zkeyring.Get(composedKey, KeyringUser)
	require.NoError(t, err, "stored entry must live at service=composedKey, account=KeyringUser")

	// And the stored bytes are the credential envelope wrapping the type's own JSON.
	awsJSON, err := json.Marshal(creds)
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"aws","data":`+string(awsJSON)+`}`, raw)

	// The account is part of the address: the same service under a different account is absent.
	_, err = zkeyring.Get(composedKey, "not-atmos-auth")
	assert.ErrorIs(t, err, zkeyring.ErrNotFound, "account must be part of the storage address")

	// Clean up the mock entry.
	_ = zkeyring.Delete(composedKey, KeyringUser)
}

// TestCredentialEnvelope_JSONSchema pins the exact stored byte layout for the file backend:
// `{"type":"<kind>","data":<type-json>}`. The wrapper shape is the keyring layer's contract;
// the inner `data` is the credential type's own JSON.
func TestCredentialEnvelope_JSONSchema(t *testing.T) {
	store := newTestFileStore(t)

	alias := "envelope-aws"
	realm := "envelope-realm"
	creds := &types.AWSCredentials{
		AccessKeyID:     "AKIA_ENVELOPE",
		SecretAccessKey: "SECRET_ENVELOPE",
		Region:          "eu-west-1",
	}
	require.NoError(t, store.Store(alias, creds, realm))

	// Read the raw stored item directly from the underlying ring (bypassing Retrieve).
	item, err := store.ring.Get(buildKeyringKey(alias, realm))
	require.NoError(t, err)

	awsJSON, err := json.Marshal(creds)
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"aws","data":`+string(awsJSON)+`}`, string(item.Data))

	// The envelope must have exactly the two contract fields.
	var env struct {
		Type string          `json:"type"`
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(item.Data, &env))
	assert.Equal(t, "aws", env.Type)
	assert.NotEmpty(t, env.Data)
}

// TestFileKeyring_RealmIsolation verifies the realm scoping actually isolates: the same alias
// in two realms stores and retrieves distinct values, and neither leaks into the other.
func TestFileKeyring_RealmIsolation(t *testing.T) {
	store := newTestFileStore(t)

	alias := "shared-alias"
	require.NoError(t, store.Store(alias, &types.OIDCCredentials{Token: "realm-a-token", Provider: "a"}, "realm-a"))
	require.NoError(t, store.Store(alias, &types.OIDCCredentials{Token: "realm-b-token", Provider: "b"}, "realm-b"))

	// Keys for the two realms must differ.
	assert.NotEqual(t, buildKeyringKey(alias, "realm-a"), buildKeyringKey(alias, "realm-b"))

	gotA, err := store.Retrieve(alias, "realm-a")
	require.NoError(t, err)
	gotB, err := store.Retrieve(alias, "realm-b")
	require.NoError(t, err)

	a, ok := gotA.(*types.OIDCCredentials)
	require.True(t, ok)
	b, ok := gotB.(*types.OIDCCredentials)
	require.True(t, ok)

	assert.Equal(t, "realm-a-token", a.Token)
	assert.Equal(t, "realm-b-token", b.Token)
}

// TestFileKeyring_StoreRetrieve_GCP fills a coverage gap: the file backend only exercised GCP
// via the shared suite. A direct round-trip pins the GCP envelope kind ("gcp") and all fields.
func TestFileKeyring_StoreRetrieve_GCP(t *testing.T) {
	store := newTestFileStore(t)

	alias := "gcp-direct"
	realm := "gcp-realm"
	creds := &types.GCPCredentials{
		AccessToken:         "ya29.CONTRACT",
		TokenExpiry:         time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC),
		ProjectID:           "my-project",
		ServiceAccountEmail: "sa@my-project.iam.gserviceaccount.com",
		Scopes:              []string{"https://www.googleapis.com/auth/cloud-platform"},
	}
	require.NoError(t, store.Store(alias, creds, realm))

	// The stored envelope kind is "gcp".
	item, err := store.ring.Get(buildKeyringKey(alias, realm))
	require.NoError(t, err)
	gcpJSON, err := json.Marshal(creds)
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"gcp","data":`+string(gcpJSON)+`}`, string(item.Data))

	got, err := store.Retrieve(alias, realm)
	require.NoError(t, err)
	out, ok := got.(*types.GCPCredentials)
	require.True(t, ok)
	assert.Equal(t, creds.AccessToken, out.AccessToken)
	assert.Equal(t, creds.TokenExpiry, out.TokenExpiry)
	assert.Equal(t, creds.ProjectID, out.ProjectID)
	assert.Equal(t, creds.ServiceAccountEmail, out.ServiceAccountEmail)
	assert.Equal(t, creds.Scopes, out.Scopes)
}

// TestKeyringMockInit_Sanity confirms the in-memory zalando mock the package init installs is
// actually active, so the system-backend contract tests above are meaningful.
func TestKeyringMockInit_Sanity(t *testing.T) {
	const service = "atmos-mockinit-sanity"
	require.NoError(t, zkeyring.Set(service, KeyringUser, "value"))

	got, err := zkeyring.Get(service, KeyringUser)
	require.NoError(t, err)
	assert.Equal(t, "value", got)

	_ = zkeyring.Delete(service, KeyringUser)
}

// Compile-time guard: a rename of the credential type structs used by the contract tests must
// break the build (parallels the schema guard in store_test.go).
var (
	_ = types.AWSCredentials{}
	_ = types.GCPCredentials{}
	_ = types.OIDCCredentials{}
	_ = keyring.Item{}
)
