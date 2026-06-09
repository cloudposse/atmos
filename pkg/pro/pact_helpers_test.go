//go:build pact

package pro

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/stretchr/testify/require"
)

// pactDir returns the absolute path to the pacts/ output directory at the repo root.
// Uses the source file path embedded at compile time so the path is always correct
// regardless of which directory go test is invoked from.
func pactDir() string {
	_, filename, _, _ := runtime.Caller(0)
	// filename is .../pkg/pro/pact_helpers_test.go — go up two levels to reach repo root.
	return filepath.Join(filepath.Dir(filename), "..", "..", "pacts")
}

// newHTTPMockProvider creates a V2 HTTP pact mock provider for standard (non-TLS) interactions.
func newHTTPMockProvider(t *testing.T) *consumer.V2HTTPMockProvider {
	t.Helper()

	provider, err := consumer.NewV2Pact(consumer.MockHTTPProviderConfig{
		Consumer: "atmos",
		Provider: "AtmosPro",
		Host:     "127.0.0.1",
		PactDir:  pactDir(),
	})
	require.NoError(t, err, "failed to create pact HTTP mock provider")

	return provider
}

// newTLSMockProvider creates a V2 TLS pact mock provider for the GitHub OIDC interaction.
// The OIDC client enforces https:// via buildOIDCRequestURL, so a TLS mock is required.
// Uses "127.0.0.1" to bind reliably on all platforms; the test client uses InsecureSkipVerify
// because the pact TLS certificate has no IP SAN for 127.0.0.1.
func newTLSMockProvider(t *testing.T) *consumer.V2HTTPMockProvider {
	t.Helper()

	provider, err := consumer.NewV2Pact(consumer.MockHTTPProviderConfig{
		Consumer: "atmos",
		Provider: "AtmosPro",
		Host:     "127.0.0.1",
		TLS:      true,
		PactDir:  pactDir(),
	})
	require.NoError(t, err, "failed to create pact TLS mock provider")

	return provider
}

// newPactClient creates an AtmosProAPIClient pointed at the pact HTTP mock server.
// pact-go always populates MockServerConfig.TLSConfig even for non-TLS providers,
// so the scheme is always http:// for the standard mock; TLS tests handle their
// own client construction directly.
func newPactClient(config consumer.MockServerConfig) *AtmosProAPIClient {
	return &AtmosProAPIClient{
		BaseURL:         fmt.Sprintf("http://%s:%d", config.Host, config.Port),
		BaseAPIEndpoint: "api/v1",
		APIToken:        "test-token",
		HTTPClient:      &http.Client{Timeout: 5 * time.Second},
	}
}

// tlsHTTPClient returns an *http.Client for connecting to the pact TLS mock server.
// InsecureSkipVerify is required because the pact mock certificate has no IP SAN for
// 127.0.0.1; this is safe for a local-only test mock.
func tlsHTTPClient(_ *tls.Config) *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // local test mock only.
		},
	}
}
