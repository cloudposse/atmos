package adfs

import (
	"crypto/tls"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/cfg"
)

// Regression: upstream left MinVersion unset (code-scanning missing-ssl-minversion). ADFS keeps
// TLS renegotiation, which only exists up to TLS 1.2, so the floor is TLS 1.2, not TLS 1.3.
func TestNewSetsTLSFloorAndKeepsRenegotiation(t *testing.T) {
	for _, skipVerify := range []bool{false, true} {
		client, err := New(&cfg.IDPAccount{URL: "https://adfs.example.com", SkipVerify: skipVerify})
		require.NoError(t, err)

		transport, ok := client.client.Transport.(*http.Transport)
		require.True(t, ok, "ADFS client must use an *http.Transport")
		require.NotNil(t, transport.TLSClientConfig)
		assert.Equal(t, uint16(tls.VersionTLS12), transport.TLSClientConfig.MinVersion)
		assert.Equal(t, tls.RenegotiateFreelyAsClient, transport.TLSClientConfig.Renegotiation)
		assert.Equal(t, skipVerify, transport.TLSClientConfig.InsecureSkipVerify)
	}
}
