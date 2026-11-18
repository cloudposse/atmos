package okta

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsLoopbackURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{url: "https://localhost:15310/certifier", want: true},
		{url: "https://LOCALHOST:15310/certifier", want: true},
		{url: "https://127.0.0.1:15310/certifier", want: true},
		{url: "https://127.12.0.1/certifier", want: true},
		{url: "https://[::1]:15310/certifier", want: true},
		{url: "https://duo.example.com/certifier", want: false},
		{url: "https://localhost.example.com/certifier", want: false},
		{url: "https://10.0.0.1/certifier", want: false},
		{url: "", want: false},
		{url: "://bad", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			assert.Equal(t, tt.want, isLoopbackURL(tt.url))
		})
	}
}

// Regression: upstream skipped certificate verification for any certifier_url the Duo page named,
// and left MinVersion unset. Verification may only be skipped for the local (loopback) certifier.
func TestCertifierTLSConfig(t *testing.T) {
	local := certifierTLSConfig("https://127.0.0.1:15310/certifier")
	assert.True(t, local.InsecureSkipVerify, "the local certifier uses a self-signed certificate")
	assert.Equal(t, uint16(tls.VersionTLS12), local.MinVersion)

	remote := certifierTLSConfig("https://certifier.attacker.example/certifier")
	assert.False(t, remote.InsecureSkipVerify, "a non-loopback certifier must be verified")
	assert.Equal(t, uint16(tls.VersionTLS12), remote.MinVersion)
}
