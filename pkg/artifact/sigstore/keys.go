package sigstore

import (
	"crypto"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/signature"
)

// maxKeyFileSize bounds one public-key file.
const maxKeyFileSize = 64 << 10

// resolvedKey is one trusted public key together with the identifiers a bundle may carry for it.
type resolvedKey struct {
	fingerprint string
	hints       []string
	key         crypto.PublicKey
}

// keySet is the trusted keys of one trust set.
type keySet struct {
	keys []resolvedKey
}

// trustedKeys expands the keys into the verifier's key list; one key is registered under each of
// its hints.
func (k *keySet) trustedKeys() []signature.TrustedKey {
	var out []signature.TrustedKey
	for index := range k.keys {
		for _, hint := range k.keys[index].hints {
			out = append(out, signature.TrustedKey{Hint: hint, PublicKey: k.keys[index].key})
		}
	}
	return out
}

// fingerprintFor maps a bundle's key hint back to the fingerprint of the key that matched.
func (k *keySet) fingerprintFor(hint string) string {
	for index := range k.keys {
		for _, candidate := range k.keys[index].hints {
			if candidate == hint {
				return k.keys[index].fingerprint
			}
		}
	}
	return hint
}

// newResolvedKey derives the fingerprint and the hint variants from a key's PKIX DER bytes.
func newResolvedKey(der []byte, key crypto.PublicKey, extraHints ...string) resolvedKey {
	sum := sha256.Sum256(der)
	fingerprint := hex.EncodeToString(sum[:])
	hints := []string{
		fingerprint,
		base64.StdEncoding.EncodeToString(sum[:]),
		base64.URLEncoding.EncodeToString(sum[:]),
		base64.RawURLEncoding.EncodeToString(sum[:]),
	}
	hints = append(hints, extraHints...)
	return resolvedKey{fingerprint: fingerprint, hints: hints, key: key}
}

// programmaticKey builds a key registered through WithTrustedKey.
func programmaticKey(hint string, key crypto.PublicKey) (resolvedKey, error) {
	der, err := x509.MarshalPKIXPublicKey(key)
	if err != nil {
		return resolvedKey{}, fmt.Errorf("%w: trusted key %q: %w", errUtils.ErrArtifactConfigInvalid, hint, err)
	}
	return newResolvedKey(der, key, hint), nil
}

// parsePublicKeyFile decodes the first PEM "PUBLIC KEY" block of a key file.
func parsePublicKeyFile(path string, raw []byte) (resolvedKey, error) {
	if len(raw) == 0 || len(raw) > maxKeyFileSize {
		return resolvedKey{}, fmt.Errorf("%w: trusted key file %q must be 1 to %d bytes", errUtils.ErrArtifactConfigInvalid, path, maxKeyFileSize)
	}
	block, _ := pem.Decode(raw)
	if block == nil || block.Type != "PUBLIC KEY" {
		return resolvedKey{}, fmt.Errorf("%w: trusted key file %q is not a PEM PUBLIC KEY", errUtils.ErrArtifactConfigInvalid, path)
	}
	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return resolvedKey{}, fmt.Errorf("%w: trusted key file %q: %w", errUtils.ErrArtifactConfigInvalid, path, err)
	}
	return newResolvedKey(block.Bytes, key), nil
}
