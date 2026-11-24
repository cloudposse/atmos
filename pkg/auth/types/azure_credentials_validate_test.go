package types

import (
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

const fakeSubscriptionResponse = `{"id":"/subscriptions/synthetic-subscription","subscriptionId":"synthetic-subscription","displayName":"Synthetic"}`

// fakeARM starts a local TLS stand-in for Azure Resource Manager that answers every request
// with status and body, and returns client options that send the validation call to it.
func fakeARM(t *testing.T, status int, body string) (*arm.ClientOptions, *atomic.Int32) {
	t.Helper()
	calls := &atomic.Int32{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return armOptionsFor(server.URL, server.Client()), calls
}

// armOptionsFor returns client options that target endpoint through transport.
func armOptionsFor(endpoint string, transport policy.Transporter) *arm.ClientOptions {
	return &arm.ClientOptions{ClientOptions: azcore.ClientOptions{
		Cloud: cloud.Configuration{
			ActiveDirectoryAuthorityHost: "https://login.example.invalid/",
			Services: map[cloud.ServiceName]cloud.ServiceConfiguration{
				cloud.ResourceManager: {Endpoint: endpoint, Audience: "https://management.example.invalid"},
			},
		},
		Transport: transport,
		// No retries, so the unreachable and server-fault cases answer at once.
		Retry: policy.RetryOptions{MaxRetries: -1},
	}}
}

func validAzureCredentials() *AzureCredentials {
	return &AzureCredentials{
		AccessToken:    "synthetic-access",
		TenantID:       "synthetic-tenant",
		SubscriptionID: "synthetic-subscription",
		Expiration:     time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}
}

// TestAzureCredentials_ValidateUnreachableARM reproduces the 1.235.0 upgrade regression where
// `atmos auth whoami` exited 1 for valid Azure credentials when Azure Resource Manager could not
// be reached. Like AWS, an unreachable endpoint means the credentials could not be validated, not
// that they are invalid.
func TestAzureCredentials_ValidateUnreachableARM(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	endpoint := "https://" + listener.Addr().String()
	require.NoError(t, listener.Close())

	creds := validAzureCredentials()
	creds.validationClientOptions = armOptionsFor(endpoint, nil)

	info, err := creds.Validate(t.Context())
	require.Nil(t, info)
	require.ErrorIs(t, err, errUtils.ErrCredentialValidationUnavailable)
	assert.NotErrorIs(t, err, errUtils.ErrAuthenticationFailed)
}

// TestAzureCredentials_ValidateWithoutSubscription verifies that credentials without a
// subscription cannot be checked against ARM, which is reported as an unavailable validation
// rather than as invalid credentials.
func TestAzureCredentials_ValidateWithoutSubscription(t *testing.T) {
	options, calls := fakeARM(t, http.StatusOK, fakeSubscriptionResponse)
	creds := validAzureCredentials()
	creds.SubscriptionID = ""
	creds.validationClientOptions = options

	_, err := creds.Validate(t.Context())
	require.ErrorIs(t, err, errUtils.ErrCredentialValidationUnavailable)
	assert.NotErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
	assert.Contains(t, err.Error(), "subscription")
	assert.Zero(t, calls.Load(), "no ARM call is made without a subscription")
}

// TestAzureCredentials_ValidateExpiredSkipsNetwork verifies expired credentials fail with
// ErrExpiredCredentials without contacting ARM.
func TestAzureCredentials_ValidateExpiredSkipsNetwork(t *testing.T) {
	options, calls := fakeARM(t, http.StatusOK, fakeSubscriptionResponse)
	creds := validAzureCredentials()
	creds.Expiration = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	creds.validationClientOptions = options

	_, err := creds.Validate(t.Context())
	require.ErrorIs(t, err, errUtils.ErrExpiredCredentials)
	assert.Zero(t, calls.Load())
}

// TestAzureCredentials_ValidateClassifiesARMAnswers verifies that only an answer saying the token
// itself was rejected marks the credentials invalid. Other answers (no access to the subscription,
// an unknown subscription, a server fault) leave them unvalidated.
func TestAzureCredentials_ValidateClassifiesARMAnswers(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		rejected bool
	}{
		{name: "401 invalid token", status: http.StatusUnauthorized, body: `{"error":{"code":"InvalidAuthenticationToken","message":"bad"}}`, rejected: true},
		{name: "401 expired token", status: http.StatusUnauthorized, body: `{"error":{"code":"ExpiredAuthenticationToken","message":"expired"}}`, rejected: true},
		{name: "403 expired token code", status: http.StatusForbidden, body: `{"error":{"code":"ExpiredAuthenticationToken","message":"expired"}}`, rejected: true},
		{name: "403 no access to the subscription", status: http.StatusForbidden, body: `{"error":{"code":"AuthorizationFailed","message":"no access"}}`},
		{name: "404 unknown subscription", status: http.StatusNotFound, body: `{"error":{"code":"SubscriptionNotFound","message":"missing"}}`},
		{name: "500 server fault", status: http.StatusInternalServerError, body: `{"error":{"code":"InternalServerError","message":"boom"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options, _ := fakeARM(t, tt.status, tt.body)
			creds := validAzureCredentials()
			creds.validationClientOptions = options

			info, err := creds.Validate(t.Context())
			require.Nil(t, info)
			if tt.rejected {
				require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
				assert.NotErrorIs(t, err, errUtils.ErrCredentialValidationUnavailable)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrCredentialValidationUnavailable)
			assert.NotErrorIs(t, err, errUtils.ErrAuthenticationFailed)
		})
	}
}

// TestAzureCredentials_ValidateSucceeds verifies a successful ARM answer populates the validation info.
func TestAzureCredentials_ValidateSucceeds(t *testing.T) {
	options, calls := fakeARM(t, http.StatusOK, fakeSubscriptionResponse)
	creds := validAzureCredentials()
	creds.validationClientOptions = options

	info, err := creds.Validate(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "/subscriptions/synthetic-subscription", info.Principal)
	assert.Equal(t, "synthetic-tenant", info.Account)
	require.NotNil(t, info.Expiration)
	assert.Equal(t, int32(1), calls.Load())
}

// TestAzureCredentials_ValidationTargetsTheCredentialCloud verifies that credentials minted in a
// sovereign cloud are validated against that cloud's Resource Manager, not the public one, whose
// answer would wrongly reject the token.
func TestAzureCredentials_ValidationTargetsTheCredentialCloud(t *testing.T) {
	tests := []struct {
		cloudEnvironment string
		want             cloud.Configuration
	}{
		{cloudEnvironment: "", want: cloud.AzurePublic},
		{cloudEnvironment: "public", want: cloud.AzurePublic},
		{cloudEnvironment: "usgovernment", want: cloud.AzureGovernment},
		{cloudEnvironment: "china", want: cloud.AzureChina},
	}
	for _, tt := range tests {
		t.Run("cloud="+tt.cloudEnvironment, func(t *testing.T) {
			creds := &AzureCredentials{CloudEnvironment: tt.cloudEnvironment}
			options := creds.armValidationOptions()
			assert.Equal(t, tt.want.Services[cloud.ResourceManager].Endpoint, options.Cloud.Services[cloud.ResourceManager].Endpoint)
			assert.Equal(t, azureValidationMaxRetries, options.Retry.MaxRetries)
		})
	}

	t.Run("injected options win", func(t *testing.T) {
		injected := armOptionsFor("https://arm.example.invalid", nil)
		creds := &AzureCredentials{CloudEnvironment: "china", validationClientOptions: injected}
		assert.Same(t, injected, creds.armValidationOptions())
	})
}

// TestAzureCredentials_ValidateClientSetupFailure verifies a Resource Manager client that cannot
// be built (no Resource Manager endpoint for the cloud) leaves the credentials unvalidated.
func TestAzureCredentials_ValidateClientSetupFailure(t *testing.T) {
	creds := validAzureCredentials()
	// A non-zero cloud without a Resource Manager service (a zero cloud would default to the
	// public cloud and reach the network).
	creds.validationClientOptions = &arm.ClientOptions{ClientOptions: azcore.ClientOptions{
		Cloud:     cloud.Configuration{ActiveDirectoryAuthorityHost: "https://login.example.invalid/"},
		Transport: failingTransport{t: t},
	}}

	_, err := creds.Validate(t.Context())
	require.ErrorIs(t, err, errUtils.ErrCredentialValidationUnavailable)
	assert.NotErrorIs(t, err, errUtils.ErrAuthenticationFailed)
}

// failingTransport fails the test if a request is ever sent.
type failingTransport struct{ t *testing.T }

func (f failingTransport) Do(req *http.Request) (*http.Response, error) {
	f.t.Errorf("unexpected request to %s", req.URL)
	return nil, errors.New("no network in this test")
}

// TestAzureCredentials_ValidateKeeps401WhenBodyIsTruncated verifies a 401 whose body cannot be
// read is still a rejection. The SDK returns the body read error instead of a ResponseError in
// that case, so the status must come from the captured raw response.
func TestAzureCredentials_ValidateKeeps401WhenBodyIsTruncated(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Promise more bytes than are sent, then drop the connection.
		w.Header().Set("Content-Length", "4096")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"Invalid`))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		if hijacker, ok := w.(http.Hijacker); ok {
			if conn, _, err := hijacker.Hijack(); err == nil {
				_ = conn.Close()
			}
		}
	}))
	t.Cleanup(server.Close)
	creds := validAzureCredentials()
	creds.validationClientOptions = armOptionsFor(server.URL, server.Client())

	_, err := creds.Validate(t.Context())
	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.NotErrorIs(t, err, errUtils.ErrCredentialValidationUnavailable)
}
