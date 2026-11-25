// Package fingerprint produces opaque keys for process-local caches containing auth scopes.
package fingerprint

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"sync"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Keep the secret only in memory. Cache keys must not be reusable across processes
// or act as unkeyed digests of low-entropy credentials.
var processSecret = sync.OnceValue(rand.Text)

// Key fingerprints a serialized cache identity, including any authentication material.
// The result is stable within this process only; never persist it or use it to verify passwords.
func Key(identity []byte) string {
	defer perf.Track(nil, "fingerprint.Key")()

	return keyedDigest([]byte(processSecret()), identity)
}

func keyedDigest(secret, identity []byte) string {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(identity)
	return hex.EncodeToString(mac.Sum(nil))
}
