//go:build pact

package pro

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/pact-foundation/pact-go/v2/consumer"
	"github.com/pact-foundation/pact-go/v2/matchers"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestPact_Exceptions exercises the SDK-backed production transport using a fixed
// credential source. Neither contract requires live GitHub credentials.
func TestPact_Exceptions(t *testing.T) {
	envelope, err := os.ReadFile("testdata/exception.envelope")
	require.NoError(t, err)
	for _, tc := range []struct {
		name, state, token string
		status             int
	}{
		{"an enriched CLI exception", "repository 123 accepts exception envelopes", "pact-accepted-oidc-token", 200},
		{"an exception with rejected authentication", "repository 123 rejects exception authentication", "pact-rejected-oidc-token", 401},
	} {
		t.Run(tc.name, func(t *testing.T) {
			provider := newHTTPMockProvider(t)
			err := provider.AddInteraction().Given(tc.state).UponReceiving(tc.name).
				WithRequest("POST", "/api/v1/events/123/exceptions", func(b *consumer.V2RequestBuilder) {
					b.Header("X-Sentry-Auth", matchers.S("Sentry sentry_version=7, sentry_client=atmos-cli/test, sentry_key="+tc.token)).
						Body("application/x-sentry-envelope", envelope)
				}).WillRespondWith(tc.status).
				ExecuteTest(t, func(config consumer.MockServerConfig) error {
					transport, err := NewExceptionTransport(&schema.ProSettings{BaseURL: fmt.Sprintf("http://%s:%d", config.Host, config.Port)}, "123", ExceptionTransportOptions{Token: func(context.Context) (string, error) { return tc.token, nil }})
					if err != nil {
						return err
					}
					defer transport.Close()
					transport.Configure(sentry.ClientOptions{})
					transport.SendEvent(exceptionTestEvent())
					require.True(t, transport.Flush(time.Second))
					return nil
				})
			require.NoError(t, err)
		})
	}
}
