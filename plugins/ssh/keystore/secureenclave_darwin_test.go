//go:build darwin

package keystore

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

const (
	hardwareEnv = "ATMOS_SSH_TEST_SECURE_ENCLAVE"
	// pocBlob is a Secure Enclave blob made by CryptoKit in the proof of concept. It is only read.
	pocBlob = "/Users/erik/se-agent-poc/keys/afu.key"
)

func requireHardware(t *testing.T) {
	t.Helper()
	if os.Getenv(hardwareEnv) != "1" {
		t.Skipf("set %s=1 to run against the Secure Enclave", hardwareEnv)
	}
}

func TestSecureEnclave_Hardware(t *testing.T) {
	requireHardware(t)
	ctx := context.Background()
	store := NewSecureEnclaveStore(t.TempDir())
	require.True(t, store.Supports(Unlocked))

	tests := []struct {
		name string
		reqs []Requirement
		want []Requirement
	}{
		{"none", nil, []Requirement{}},
		{"unlocked", []Requirement{Unlocked}, []Requirement{Unlocked}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := store.Create(ctx, tt.name, tt.reqs)
			require.NoError(t, err)
			defer key.Close()
			assert.Equal(t, "ecdsa-sha2-nistp256", key.SSHPublicKey().Type())
			assert.ElementsMatch(t, tt.want, key.Requirements())
			signAndVerify(t, key)

			loaded, err := store.Load(ctx, tt.name)
			require.NoError(t, err)
			defer loaded.Close()
			assert.Equal(t, key.SSHPublicKey().Marshal(), loaded.SSHPublicKey().Marshal())
			assert.ElementsMatch(t, tt.want, loaded.Requirements())
			signAndVerify(t, loaded)
		})
	}

	infos, err := store.List(ctx)
	require.NoError(t, err)
	require.Len(t, infos, 2)
	require.NoError(t, store.Delete(ctx, "none"))
	_, err = store.Load(ctx, "none")
	assert.ErrorIs(t, err, ErrKeyNotFound)

	t.Run("closed key refuses to sign", func(t *testing.T) {
		key, err := store.Load(ctx, "unlocked")
		require.NoError(t, err)
		require.NoError(t, key.Close())
		require.NoError(t, key.Close())
		digest := sha256.Sum256([]byte("x"))
		_, err = key.Sign(rand.Reader, digest[:], crypto.SHA256)
		assert.ErrorIs(t, err, ErrKeyClosed)
	})
}

// TestSecureEnclave_LoadsCryptoKitBlob proves keys made by the CryptoKit proof of concept load and
// sign through the C API. The blob is read, never modified.
func TestSecureEnclave_LoadsCryptoKitBlob(t *testing.T) {
	requireHardware(t)
	blob, err := os.ReadFile(pocBlob)
	if os.IsNotExist(err) {
		t.Skipf("no proof-of-concept blob at %s", pocBlob)
	}
	require.NoError(t, err)

	api, err := loadAPI()
	require.NoError(t, err)
	key, err := openSEKey(api, blob)
	require.NoError(t, err)
	defer key.Close()

	pub, ok := key.Public().(*ecdsa.PublicKey)
	require.True(t, ok)
	sshPub, err := ssh.NewPublicKey(pub)
	require.NoError(t, err)

	signer, err := ssh.NewSignerFromSigner(key)
	require.NoError(t, err)
	msg := []byte("cryptokit compatibility")
	sig, err := signer.Sign(rand.Reader, msg)
	require.NoError(t, err)
	require.NoError(t, sshPub.Verify(msg, sig))

	// A second open of the same blob yields the same key.
	again, err := openSEKey(api, blob)
	require.NoError(t, err)
	defer again.Close()
	assert.True(t, pub.Equal(again.Public()))
}

func TestSecureEnclave_BadBlob(t *testing.T) {
	requireHardware(t)
	api, err := loadAPI()
	require.NoError(t, err)
	_, err = openSEKey(api, []byte("not a Secure Enclave blob"))
	require.Error(t, err)
	_, isStatus := OSStatus(err)
	assert.True(t, isStatus, "failure should carry an OSStatus: %v", err)
}
