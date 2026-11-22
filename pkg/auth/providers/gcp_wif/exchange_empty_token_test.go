package gcp_wif

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

// TestExchangeToken_EmptyAccessToken verifies a 200 STS response without an
// access_token is an authentication failure, not a "successful" empty credential.
func TestExchangeToken_EmptyAccessToken(t *testing.T) {
	for name, body := range map[string]string{
		"empty object":        `{}`,
		"blank access_token":  `{"access_token": "", "expires_in": 3600, "token_type": "Bearer"}`,
		"whitespace token":    `{"access_token": "   ", "expires_in": 3600, "token_type": "Bearer"}`,
		"JSON null body":      `null`,
		"missing token field": `{"expires_in": 3600, "token_type": "Bearer"}`,
	} {
		t.Run(name, func(t *testing.T) {
			ln, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Skipf("Skipping: unable to bind local listener: %v", err)
			}
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(body))
			}))
			server.Listener = ln
			server.StartTLS()
			defer server.Close()

			p := &Provider{
				spec: &types.GCPWorkloadIdentityFederationProviderSpec{
					ProjectNumber:              "123456",
					WorkloadIdentityPoolID:     "pool",
					WorkloadIdentityProviderID: "provider",
				},
				stsURL: server.URL,
			}
			p.WithHTTPClient(server.Client())

			token, err := p.exchangeToken(context.Background(), "test-oidc-token")
			require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
			assert.Nil(t, token)
			assert.Contains(t, err.Error(), "access_token")
		})
	}
}
