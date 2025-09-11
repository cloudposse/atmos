package types

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type stubStore struct {
	creds ICredentials
	err   error
}

func (s *stubStore) Store(alias string, creds ICredentials) error { return nil }
func (s *stubStore) Retrieve(alias string) (ICredentials, error)  { return s.creds, s.err }
func (s *stubStore) Delete(alias string) error                    { return nil }
func (s *stubStore) List() ([]string, error)                      { return nil, errors.New("not implemented") }
func (s *stubStore) IsExpired(alias string) (bool, error)         { return false, nil }

func TestWhoami_Rehydrate_NilReceiver(t *testing.T) {
	var w *WhoamiInfo
	// Should be a no-op and not panic.
	assert.NoError(t, w.Rehydrate(nil))
}

func TestWhoami_Rehydrate_NoRefOrAlreadyPresent(t *testing.T) {
	// No ref.
	w := &WhoamiInfo{}
	assert.NoError(t, w.Rehydrate(&stubStore{}))
	assert.Nil(t, w.Credentials)

	// Already populated.
	w = &WhoamiInfo{Credentials: &AWSCredentials{AccessKeyID: "x"}}
	assert.NoError(t, w.Rehydrate(&stubStore{}))
}

func TestWhoami_Rehydrate_NilStore(t *testing.T) {
	w := &WhoamiInfo{CredentialsRef: "alias"}
	// Nil store is tolerated.
	assert.NoError(t, w.Rehydrate(nil))
	assert.Nil(t, w.Credentials)
}

func TestWhoami_Rehydrate_Success(t *testing.T) {
	w := &WhoamiInfo{CredentialsRef: "alias"}
	creds := &AWSCredentials{AccessKeyID: "AKIA", Expiration: time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339)}
	store := &stubStore{creds: creds}

	err := w.Rehydrate(store)
	assert.NoError(t, err)
	if assert.NotNil(t, w.Credentials) {
		ac, ok := w.Credentials.(*AWSCredentials)
		assert.True(t, ok)
		assert.Equal(t, "AKIA", ac.AccessKeyID)
	}
}

func TestWhoami_Rehydrate_Error(t *testing.T) {
	w := &WhoamiInfo{CredentialsRef: "alias"}
	store := &stubStore{err: errors.New("boom")}
	err := w.Rehydrate(store)
	assert.Error(t, err)
}
