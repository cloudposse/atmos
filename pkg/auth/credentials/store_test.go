package credentials

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/zalando/go-keyring"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

// Ensure the keyring uses an in-memory mock backend for tests.
func init() {
	keyring.MockInit()
}

func TestNewCredentialStore(t *testing.T) {
	s := NewCredentialStore()
	assert.NotNil(t, s)
}

func TestStoreRetrieve_AWS(t *testing.T) {
	s := NewCredentialStore()
	alias := "aws-1"
	exp := time.Now().UTC().Add(1 * time.Hour).Format(time.RFC3339)
	in := &types.AWSCredentials{AccessKeyID: "AKIA", SecretAccessKey: "SECRET", SessionToken: "TOKEN", Region: "us-east-1", Expiration: exp}
	assert.NoError(t, s.Store(alias, in))

	got, err := s.Retrieve(alias)
	assert.NoError(t, err)
	out, ok := got.(*types.AWSCredentials)
	if assert.True(t, ok) {
		assert.Equal(t, in.AccessKeyID, out.AccessKeyID)
		assert.Equal(t, in.Region, out.Region)
		assert.Equal(t, in.Expiration, out.Expiration)
	}
}

func TestStoreRetrieve_OIDC(t *testing.T) {
	s := NewCredentialStore()
	alias := "oidc-1"
	in := &types.OIDCCredentials{Token: "hdr.payload.", Provider: "github", Audience: "sts"}
	assert.NoError(t, s.Store(alias, in))

	got, err := s.Retrieve(alias)
	assert.NoError(t, err)
	out, ok := got.(*types.OIDCCredentials)
	if assert.True(t, ok) {
		assert.Equal(t, in.Token, out.Token)
		assert.Equal(t, in.Provider, out.Provider)
		assert.Equal(t, in.Audience, out.Audience)
	}
}

// fakeCreds implements types.ICredentials but is not a supported concrete type.
type fakeCreds struct{}

func (f *fakeCreds) IsExpired() bool                        { return false }
func (f *fakeCreds) GetExpiration() (*time.Time, error)     { return nil, nil }
func (f *fakeCreds) BuildWhoamiInfo(info *types.WhoamiInfo) {}

func TestStore_UnsupportedType(t *testing.T) {
	s := NewCredentialStore()
	err := s.Store("alias", &fakeCreds{})
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrCredentialStore))
}

func TestDelete_Flow(t *testing.T) {
	s := NewCredentialStore()
	alias := "to-delete"
	// Delete non-existent -> success (treated as already deleted).
	assert.NoError(t, s.Delete(alias))

	// Store then delete -> ok.
	assert.NoError(t, s.Store(alias, &types.OIDCCredentials{Token: "hdr.payload."}))
	assert.NoError(t, s.Delete(alias))
	// Retrieve after delete -> error.
	_, err := s.Retrieve(alias)
	assert.Error(t, err)

	// Delete again -> success (idempotent).
	assert.NoError(t, s.Delete(alias))
}

func TestList_NotSupported(t *testing.T) {
	s := NewCredentialStore()
	_, err := s.List()
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrCredentialStore))
}

// TestDefaultStore_Suite runs the shared test suite against the default credential store.
func TestDefaultStore_Suite(t *testing.T) {
	factory := func(t *testing.T) types.CredentialStore {
		return NewCredentialStore()
	}

	RunCredentialStoreTests(t, factory)
}
