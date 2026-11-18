//go:build !windows

package agent

import (
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/cloudposse/atmos/plugins/ssh/keystore"
)

// fakeStore serves one in-memory ed25519 key whose Sign fails with signErr.
type fakeStore struct {
	priv    ed25519.PrivateKey
	pub     ssh.PublicKey
	signErr error

	mu     sync.Mutex
	opens  int
	closes int
}

// newLockedStore returns a store whose key refuses to sign as the Secure Enclave does when the
// screen is locked.
func newLockedStore(t *testing.T) *fakeStore {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	sshPub, err := ssh.NewPublicKey(pub)
	require.NoError(t, err)
	return &fakeStore{
		priv: priv, pub: sshPub,
		signErr: &keystore.OSStatusError{
			Op: "SecKeyCreateSignature", Code: keystore.CodeInteractionNotAllowed,
			Domain: "NSOSStatusErrorDomain", Message: "screen is locked",
		},
	}
}

func (s *fakeStore) opened() int      { s.mu.Lock(); defer s.mu.Unlock(); return s.opens }
func (s *fakeStore) closedCount() int { s.mu.Lock(); defer s.mu.Unlock(); return s.closes }

func (s *fakeStore) Kind() string                         { return "fake" }
func (s *fakeStore) Supports(keystore.Requirement) bool   { return false }
func (s *fakeStore) Delete(context.Context, string) error { return keystore.ErrKeyNotFound }
func (s *fakeStore) List(context.Context) ([]keystore.Info, error) {
	return nil, nil
}

func (s *fakeStore) Create(context.Context, string, []keystore.Requirement) (keystore.Key, error) {
	return nil, keystore.ErrUnsupportedRequirement
}

func (s *fakeStore) Load(_ context.Context, name string) (keystore.Key, error) {
	s.mu.Lock()
	s.opens++
	s.mu.Unlock()
	return &fakeKey{store: s, name: name}, nil
}

type fakeKey struct {
	store *fakeStore
	name  string
}

var _ keystore.Key = (*fakeKey)(nil)

func (k *fakeKey) Public() crypto.PublicKey { return k.store.priv.Public() }
func (k *fakeKey) Sign(r io.Reader, msg []byte, o crypto.SignerOpts) ([]byte, error) {
	if k.store.signErr != nil {
		return nil, k.store.signErr
	}
	return k.store.priv.Sign(r, msg, o)
}

func (k *fakeKey) Close() error {
	k.store.mu.Lock()
	defer k.store.mu.Unlock()
	k.store.closes++
	return nil
}
func (k *fakeKey) Name() string                         { return k.name }
func (k *fakeKey) SSHPublicKey() ssh.PublicKey          { return k.store.pub }
func (k *fakeKey) Requirements() []keystore.Requirement { return nil }
func (k *fakeKey) Info() keystore.Info {
	return keystore.Info{Name: k.name, Store: "fake", Created: time.Now().UTC(), PublicKey: k.store.pub}
}
