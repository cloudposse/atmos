package keystore

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// signAndVerify signs a message through the ssh.Signer wrapper and verifies it with the key's
// OpenSSH public key.
func signAndVerify(t *testing.T, key Key) {
	t.Helper()
	signer, err := ssh.NewSignerFromSigner(key)
	require.NoError(t, err)
	msg := []byte("keystore test message")
	sig, err := signer.Sign(rand.Reader, msg)
	require.NoError(t, err)
	require.NoError(t, key.SSHPublicKey().Verify(msg, sig))
	assert.Equal(t, key.SSHPublicKey().Marshal(), signer.PublicKey().Marshal())
}

func TestFileStore_CreateLoadSign(t *testing.T) {
	ctx := context.Background()
	dir := filepath.Join(t.TempDir(), "keys")
	store := NewFileStore(dir)

	created, err := store.Create(ctx, "ops", nil)
	require.NoError(t, err)
	defer created.Close()
	assert.Equal(t, "ops", created.Name())
	assert.Empty(t, created.Requirements())
	assert.Equal(t, "ssh-ed25519", created.SSHPublicKey().Type())
	signAndVerify(t, created)

	loaded, err := store.Load(ctx, "ops")
	require.NoError(t, err)
	defer loaded.Close()
	assert.Equal(t, created.SSHPublicKey().Marshal(), loaded.SSHPublicKey().Marshal())
	signAndVerify(t, loaded)

	// Raw digest-style signing is not what ed25519 does; it signs the message with Hash(0).
	sig, err := loaded.Sign(rand.Reader, []byte("m"), crypto.Hash(0))
	require.NoError(t, err)
	assert.NotEmpty(t, sig)
}

func TestFileStore_Permissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX file modes are not enforced on Windows.")
	}
	dir := filepath.Join(t.TempDir(), "keys")
	_, err := NewFileStore(dir).Create(context.Background(), "ops", nil)
	require.NoError(t, err)

	st, err := os.Stat(dir)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o700), st.Mode().Perm())
	for _, f := range []string{"ops.key", "ops.json"} {
		st, err := os.Stat(filepath.Join(dir, f))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), st.Mode().Perm(), f)
	}
}

func TestFileStore_RejectsUnlocked(t *testing.T) {
	dir := t.TempDir()
	store := NewFileStore(dir)
	assert.False(t, store.Supports(Unlocked))

	tests := []struct {
		name string
		reqs []Requirement
	}{
		{"unlocked", []Requirement{Unlocked}},
		{"duplicated", []Requirement{Unlocked, Unlocked}},
		{"unknown", []Requirement{"biometry"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := store.Create(context.Background(), "k", tt.reqs)
			require.ErrorIs(t, err, ErrUnsupportedRequirement)
			assert.Nil(t, key)
		})
	}
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "a rejected create must leave nothing on disk")
}

func TestFileStore_Errors(t *testing.T) {
	ctx := context.Background()
	store := NewFileStore(t.TempDir())
	_, err := store.Create(ctx, "dup", nil)
	require.NoError(t, err)

	tests := []struct {
		name string
		do   func() error
		want error
	}{
		{"create twice", func() error { _, err := store.Create(ctx, "dup", nil); return err }, ErrKeyExists},
		{"load missing", func() error { _, err := store.Load(ctx, "none"); return err }, ErrKeyNotFound},
		{"delete missing", func() error { return store.Delete(ctx, "none") }, ErrKeyNotFound},
		{"empty name", func() error { _, err := store.Create(ctx, "", nil); return err }, ErrInvalidName},
		{"traversal", func() error { _, err := store.Load(ctx, "../etc/passwd"); return err }, ErrInvalidName},
		{"separator", func() error { return store.Delete(ctx, "a/b") }, ErrInvalidName},
		{"leading dot", func() error { _, err := store.Create(ctx, ".hidden", nil); return err }, ErrInvalidName},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.ErrorIs(t, tt.do(), tt.want)
		})
	}

	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = store.Create(cancelled, "late", nil)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestFileStore_ListAndDelete(t *testing.T) {
	ctx := context.Background()
	store := NewFileStore(filepath.Join(t.TempDir(), "absent"))

	infos, err := store.List(ctx)
	require.NoError(t, err)
	assert.Empty(t, infos)

	var pubs [2][]byte
	for i, name := range []string{"zeta", "alpha"} {
		key, err := store.Create(ctx, name, nil)
		require.NoError(t, err)
		pubs[i] = key.SSHPublicKey().Marshal()
		require.NoError(t, key.Close())
	}
	infos, err = store.List(ctx)
	require.NoError(t, err)
	require.Len(t, infos, 2)
	assert.Equal(t, "alpha", infos[0].Name)
	assert.Equal(t, "zeta", infos[1].Name)
	assert.Equal(t, pubs[1], infos[0].PublicKey.Marshal())
	assert.Equal(t, pubs[0], infos[1].PublicKey.Marshal())
	assert.Equal(t, KindFile, infos[0].Store)
	assert.False(t, infos[0].Created.IsZero())

	require.NoError(t, store.Delete(ctx, "alpha"))
	_, err = store.Load(ctx, "alpha")
	require.ErrorIs(t, err, ErrKeyNotFound)
	infos, err = store.List(ctx)
	require.NoError(t, err)
	require.Len(t, infos, 1)
	assert.Equal(t, "zeta", infos[0].Name)
}

func TestFileStore_CorruptStorage(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store := NewFileStore(dir)
	a, err := store.Create(ctx, "a", nil)
	require.NoError(t, err)
	b, err := store.Create(ctx, "b", nil)
	require.NoError(t, err)
	_ = a.Close()
	_ = b.Close()

	t.Run("missing metadata", func(t *testing.T) {
		require.NoError(t, os.Remove(filepath.Join(dir, "a.json")))
		_, err := store.Load(ctx, "a")
		assert.ErrorIs(t, err, ErrInvalidKey)
	})
	t.Run("metadata for a different key", func(t *testing.T) {
		other, err := os.ReadFile(filepath.Join(dir, "b.json"))
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "a.json"), other, 0o600))
		_, err = store.Load(ctx, "a")
		assert.ErrorIs(t, err, ErrInvalidKey)
	})
	t.Run("garbage private key", func(t *testing.T) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "b.key"), []byte("not a key"), 0o600))
		_, err := store.Load(ctx, "b")
		assert.ErrorIs(t, err, ErrInvalidKey)
	})
}

func TestRequirements(t *testing.T) {
	assert.Equal(t, []Requirement{Unlocked}, Normalize([]Requirement{Unlocked, Unlocked}))
	assert.Equal(t, []Requirement{"a", "b"}, Normalize([]Requirement{"b", "a", "b"}))
	assert.Empty(t, Normalize(nil))

	tests := []struct {
		name       string
		have, want []Requirement
		err        bool
	}{
		{"both empty", nil, nil, false},
		{"same", []Requirement{Unlocked}, []Requirement{Unlocked}, false},
		{"order and duplicates", []Requirement{"b", "a"}, []Requirement{"a", "b", "a"}, false},
		{"added", nil, []Requirement{Unlocked}, true},
		{"dropped", []Requirement{Unlocked}, nil, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckRequirements(tt.have, tt.want)
			if tt.err {
				assert.ErrorIs(t, err, ErrRequirementMismatch)
				return
			}
			assert.NoError(t, err)
		})
	}
}

func TestRegistry(t *testing.T) {
	assert.Contains(t, Kinds(), KindFile)
	assert.Contains(t, Kinds(), KindSecureEnclave)

	store, err := New(KindFile, t.TempDir())
	require.NoError(t, err)
	assert.Equal(t, KindFile, store.Kind())

	_, err = New("tpm-does-not-exist", t.TempDir())
	assert.ErrorIs(t, err, ErrUnknownKind)

	const custom = "registry-test-kind"
	Register(custom, func(dir string) (Store, error) { return NewFileStore(dir), nil })
	got, err := New(custom, t.TempDir())
	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Panics(t, func() { Register(custom, func(string) (Store, error) { return nil, nil }) })
	assert.Panics(t, func() { Register("", func(string) (Store, error) { return nil, nil }) })
	assert.Panics(t, func() { Register("registry-nil", nil) })
}

func TestOSStatus(t *testing.T) {
	locked := &OSStatusError{Op: "SecKeyCreateSignature", Code: -25308, Domain: "NSOSStatusErrorDomain", Message: "denied"}
	wrapped := errors.Join(errors.New("sign"), locked)

	assert.True(t, IsInteractionNotAllowed(locked))
	assert.True(t, IsInteractionNotAllowed(wrapped))
	code, ok := OSStatus(wrapped)
	assert.True(t, ok)
	assert.Equal(t, CodeInteractionNotAllowed, code)
	assert.Contains(t, locked.Error(), "-25308")

	assert.False(t, IsInteractionNotAllowed(&OSStatusError{Code: -25293}))
	assert.False(t, IsInteractionNotAllowed(errors.New("other")))
	assert.False(t, IsInteractionNotAllowed(nil))
	_, ok = OSStatus(nil)
	assert.False(t, ok)
}

func TestCheckSHA256Digest(t *testing.T) {
	good := sha256.Sum256([]byte("x"))
	assert.NoError(t, checkSHA256Digest(good[:], crypto.SHA256))
	assert.NoError(t, checkSHA256Digest(good[:], nil))
	assert.ErrorIs(t, checkSHA256Digest(good[:], crypto.SHA512), ErrInvalidDigest)
	assert.ErrorIs(t, checkSHA256Digest(good[:5], crypto.SHA256), ErrInvalidDigest)
}
