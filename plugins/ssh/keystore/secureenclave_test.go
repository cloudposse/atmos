package keystore

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"errors"
	"io"
	"runtime"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeBackend is an in-memory stand-in for the Secure Enclave so the store logic runs everywhere.
type fakeBackend struct {
	generated [][]Requirement
	signErr   error
}

type fakeHandle struct {
	key     *ecdsa.PrivateKey
	signErr error
	once    sync.Once
	closed  bool
}

func (f *fakeBackend) generate(reqs []Requirement) ([]byte, handle, error) {
	f.generated = append(f.generated, reqs)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	blob, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, nil, err
	}
	return blob, &fakeHandle{key: key, signErr: f.signErr}, nil
}

func (f *fakeBackend) open(blob []byte) (handle, error) {
	key, err := x509.ParseECPrivateKey(blob)
	if err != nil {
		return nil, err
	}
	return &fakeHandle{key: key, signErr: f.signErr}, nil
}

func (h *fakeHandle) Public() crypto.PublicKey { return h.key.Public() }
func (h *fakeHandle) Close() error             { h.once.Do(func() { h.closed = true }); return nil }
func (h *fakeHandle) Sign(r io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if h.signErr != nil {
		return nil, h.signErr
	}
	if err := checkSHA256Digest(digest, opts); err != nil {
		return nil, err
	}
	return h.key.Sign(r, digest, opts)
}

func useFakeBackend(t *testing.T, fb *fakeBackend) {
	t.Helper()
	seBackendMu.Lock()
	prev := seBackendFn
	seBackendFn = fb
	seBackendMu.Unlock()
	t.Cleanup(func() {
		seBackendMu.Lock()
		seBackendFn = prev
		seBackendMu.Unlock()
	})
}

func TestSecureEnclaveStore_WithFakeBackend(t *testing.T) {
	ctx := context.Background()
	fb := &fakeBackend{}
	useFakeBackend(t, fb)
	store := NewSecureEnclaveStore(t.TempDir())

	assert.Equal(t, KindSecureEnclave, store.Kind())
	assert.True(t, store.Supports(Unlocked))
	assert.False(t, store.Supports("biometry"))

	tests := []struct {
		name string
		reqs []Requirement
		want []Requirement
	}{
		{"plain", nil, []Requirement{}},
		{"unlocked", []Requirement{Unlocked}, []Requirement{Unlocked}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := store.Create(ctx, tt.name+"-key", tt.reqs)
			require.NoError(t, err)
			defer key.Close()
			assert.Equal(t, "ecdsa-sha2-nistp256", key.SSHPublicKey().Type())
			assert.ElementsMatch(t, tt.want, key.Requirements())
			signAndVerify(t, key)

			// The sidecar, not the blob, tells how the key was created.
			loaded, err := store.Load(ctx, tt.name+"-key")
			require.NoError(t, err)
			defer loaded.Close()
			assert.ElementsMatch(t, tt.want, loaded.Requirements())
			assert.NoError(t, CheckRequirements(loaded.Requirements(), tt.reqs))
			signAndVerify(t, loaded)
		})
	}
	require.Len(t, fb.generated, 2)
	assert.Empty(t, fb.generated[0])
	assert.Equal(t, []Requirement{Unlocked}, fb.generated[1])

	infos, err := store.List(ctx)
	require.NoError(t, err)
	require.Len(t, infos, 2)
	assert.Equal(t, "plain-key", infos[0].Name)
	assert.Equal(t, []Requirement{Unlocked}, infos[1].Requirements)

	require.ErrorIs(t, func() error { _, err := store.Create(ctx, "unlocked-key", nil); return err }(), ErrKeyExists)
	require.ErrorIs(t, func() error { _, err := store.Create(ctx, "bad", []Requirement{"biometry"}); return err }(), ErrUnsupportedRequirement)
	require.NoError(t, store.Delete(ctx, "unlocked-key"))
	require.ErrorIs(t, store.Delete(ctx, "unlocked-key"), ErrKeyNotFound)
}

func TestSecureEnclaveStore_LockedErrorSurfaces(t *testing.T) {
	locked := &OSStatusError{Op: "SecKeyCreateSignature", Code: CodeInteractionNotAllowed}
	useFakeBackend(t, &fakeBackend{})
	store := NewSecureEnclaveStore(t.TempDir())
	key, err := store.Create(context.Background(), "k", []Requirement{Unlocked})
	require.NoError(t, err)
	defer key.Close()
	key.(*storedKey).handle.(*fakeHandle).signErr = locked

	digest := sha256.Sum256([]byte("x"))
	_, err = key.Sign(rand.Reader, digest[:], crypto.SHA256)
	require.Error(t, err)
	assert.True(t, IsInteractionNotAllowed(err))
}

func TestSecureEnclaveStore_DetectsSwappedMetadata(t *testing.T) {
	ctx := context.Background()
	useFakeBackend(t, &fakeBackend{})
	dir := t.TempDir()
	store := NewSecureEnclaveStore(dir)
	a, err := store.Create(ctx, "a", nil)
	require.NoError(t, err)
	b, err := store.Create(ctx, "b", nil)
	require.NoError(t, err)
	_ = a.Close()
	_ = b.Close()

	// Point a's metadata at b's public key by copying b's blob over a's.
	blobB, err := readFile(t, dir, "b.key")
	require.NoError(t, err)
	require.NoError(t, writeFile(t, dir, "a.key", blobB))
	_, err = store.Load(ctx, "a")
	assert.ErrorIs(t, err, ErrInvalidKey)
}

func TestSecureEnclaveStore_UnsupportedWithoutBackend(t *testing.T) {
	if runtime.GOOS == "darwin" {
		t.Skip("macOS has a real backend; this covers the stub behaviour.")
	}
	ctx := context.Background()
	store := NewSecureEnclaveStore(t.TempDir())
	assert.False(t, store.Supports(Unlocked))

	_, err := store.Create(ctx, "k", nil)
	assert.ErrorIs(t, err, ErrUnsupportedPlatform)
	_, err = store.Load(ctx, "k")
	assert.ErrorIs(t, err, ErrUnsupportedPlatform)
	assert.ErrorIs(t, store.Delete(ctx, "k"), ErrUnsupportedPlatform)
	_, err = store.List(ctx)
	assert.ErrorIs(t, err, ErrUnsupportedPlatform)
	assert.False(t, errors.Is(err, ErrKeyNotFound))
}
