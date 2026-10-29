package fingerprint

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKeyedDigest(t *testing.T) {
	// RFC 4231, test case 1, checks that both the secret and input reach HMAC-SHA256.
	secret := bytes.Repeat([]byte{0x0b}, 20)
	identity := []byte("Hi There")
	digest := keyedDigest(secret, identity)
	require.Equal(t, "b0344c61d8db38535ca8afceaf0bf12b881dc200c9833da726e9376c2e32cff7", digest)
	require.NotEqual(t, digest, keyedDigest([]byte("another process secret"), identity))
	require.NotEqual(t, digest, keyedDigest(secret, []byte("another auth scope")))
}

func TestKeyStableAcrossConcurrentCallers(t *testing.T) {
	const callers = 32
	results := make(chan string, callers)
	for range callers {
		go func() { results <- Key([]byte("same auth scope")) }()
	}
	want := <-results
	for range callers - 1 {
		require.Equal(t, want, <-results)
	}
	require.NotEqual(t, want, Key([]byte("rotated auth scope")))
}
