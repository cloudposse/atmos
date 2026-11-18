// Package authoritytest holds the conformance suite every authority must pass, a fake GitHub
// server, and an in-memory fake authority for other packages' tests.
package authoritytest

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/cloudposse/atmos/plugins/ssh/authority"
)

// Environment is one isolated authority under test, together with the hooks the suite needs to
// look behind it.
type Environment struct {
	// Authority is the implementation under test.
	Authority authority.Authority
	// Credentials hold every permission the authority needs.
	Credentials map[string]string
	// Invalid holds credentials the authority must reject. Nil skips the check.
	Invalid map[string]string
	// Restricted returns credentials that lack the permission for use. Nil skips the check.
	Restricted func(use authority.Use) map[string]string
	// Permission names the permission that Restricted credentials lack for use. The error must
	// contain it.
	Permission func(use authority.Use) string
	// Seed registers key on the authority for use before the test enrolls it, and returns the ID
	// the authority gave it. Nil skips the adoption check.
	Seed func(t *testing.T, key ssh.PublicKey, use authority.Use) string
	// Enrollments counts the entries on the authority for key and use. Nil skips the checks that
	// need to look behind the authority.
	Enrollments func(key ssh.PublicKey, use authority.Use) int
	// Log returns everything the authority sent, as text. Nil skips the log check.
	Log func() string
	// Secrets are the strings, such as tokens, that must not appear in a receipt, an error, or
	// the log.
	Secrets []string
}

// NewKey returns a fresh Ed25519 public key.
func NewKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	key, err := ssh.NewPublicKey(pub)
	require.NoError(t, err)
	return key
}

// Fingerprints returns the fingerprints of keys as a set, for Params.Existing.
func Fingerprints(keys ...ssh.PublicKey) map[string]bool {
	set := make(map[string]bool, len(keys))
	for _, key := range keys {
		set[authority.Fingerprint(key)] = true
	}
	return set
}
