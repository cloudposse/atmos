package types

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

const (
	fakeSTSCallerIdentityXML = `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetCallerIdentityResult>
    <Arn>arn:aws:iam::123456789012:user/dev</Arn>
    <UserId>AIDAEXAMPLE</UserId>
    <Account>123456789012</Account>
  </GetCallerIdentityResult>
  <ResponseMetadata><RequestId>req-1</RequestId></ResponseMetadata>
</GetCallerIdentityResponse>`
	fakeSTSInvalidTokenXML = `<ErrorResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <Error><Type>Sender</Type><Code>InvalidClientTokenId</Code><Message>The security token included in the request is invalid.</Message></Error>
  <RequestId>req-2</RequestId>
</ErrorResponse>`
	fakeSTSInternalFailureXML = `<ErrorResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <Error><Type>Receiver</Type><Code>InternalFailure</Code><Message>internal</Message></Error>
  <RequestId>req-3</RequestId>
</ErrorResponse>`
)

// fakeSTS starts a local STS stand-in that answers every request with status and body,
// and counts the GetCallerIdentity calls it receives.
func fakeSTS(t *testing.T, status int, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err == nil && r.Form.Get("Action") == "GetCallerIdentity" {
			calls.Add(1)
		}
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server, &calls
}

// closedLocalEndpoint returns the URL of a loopback port nothing listens on, so a
// connection attempt is refused immediately without touching the network.
func closedLocalEndpoint(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())
	return "http://" + addr
}

func validAWSCredentials() *AWSCredentials {
	return &AWSCredentials{
		AccessKeyID:     "AKIASYNTHETIC0000000",
		SecretAccessKey: "synthetic-secret",
		SessionToken:    "synthetic-session-token",
		Region:          "us-east-1",
		Expiration:      time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}
}

// TestAWSCredentials_ValidateUsesConfiguredEndpoint verifies that validation calls the
// identity's configured endpoint (spec.endpoint_url) instead of public STS.
func TestAWSCredentials_ValidateUsesConfiguredEndpoint(t *testing.T) {
	server, calls := fakeSTS(t, http.StatusOK, fakeSTSCallerIdentityXML)
	creds := validAWSCredentials()
	creds.SetValidationEndpoint(server.URL)

	info, err := creds.Validate(context.Background())
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.Equal(t, "arn:aws:iam::123456789012:user/dev", info.Principal)
	assert.Equal(t, "123456789012", info.Account)
	assert.NotNil(t, info.Expiration)
	assert.Equal(t, int32(1), calls.Load(), "the configured endpoint must receive the GetCallerIdentity call")
}

// TestAWSCredentials_ValidateRejectedCredentials verifies that an STS rejection is a
// definitive authentication failure, not an unavailable validation.
func TestAWSCredentials_ValidateRejectedCredentials(t *testing.T) {
	server, _ := fakeSTS(t, http.StatusForbidden, fakeSTSInvalidTokenXML)
	creds := validAWSCredentials()
	creds.SetValidationEndpoint(server.URL)

	info, err := creds.Validate(context.Background())
	require.Nil(t, info)
	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.NotErrorIs(t, err, errUtils.ErrCredentialValidationUnavailable)
}

// TestAWSCredentials_ValidateUnreachableEndpoint verifies that a transport failure is
// reported as an unavailable validation rather than as rejected credentials.
func TestAWSCredentials_ValidateUnreachableEndpoint(t *testing.T) {
	creds := validAWSCredentials()
	creds.SetValidationEndpoint(closedLocalEndpoint(t))

	info, err := creds.Validate(context.Background())
	require.Nil(t, info)
	require.ErrorIs(t, err, errUtils.ErrCredentialValidationUnavailable)
	assert.NotErrorIs(t, err, errUtils.ErrAuthenticationFailed)
}

// TestAWSCredentials_ValidateServerFault verifies that a server-side STS failure is not
// treated as a rejection of the credentials.
func TestAWSCredentials_ValidateServerFault(t *testing.T) {
	server, _ := fakeSTS(t, http.StatusInternalServerError, fakeSTSInternalFailureXML)
	creds := validAWSCredentials()
	creds.SetValidationEndpoint(server.URL)

	_, err := creds.Validate(context.Background())
	require.ErrorIs(t, err, errUtils.ErrCredentialValidationUnavailable)
	assert.NotErrorIs(t, err, errUtils.ErrAuthenticationFailed)
}

// TestAWSCredentials_ValidateExpiredSkipsNetwork verifies that expired credentials fail
// definitively without contacting the endpoint.
func TestAWSCredentials_ValidateExpiredSkipsNetwork(t *testing.T) {
	server, calls := fakeSTS(t, http.StatusOK, fakeSTSCallerIdentityXML)
	creds := validAWSCredentials()
	creds.Expiration = time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	creds.SetValidationEndpoint(server.URL)

	info, err := creds.Validate(context.Background())
	require.Nil(t, info)
	require.ErrorIs(t, err, errUtils.ErrExpiredCredentials)
	assert.NotErrorIs(t, err, errUtils.ErrCredentialValidationUnavailable)
	assert.Zero(t, calls.Load(), "expired credentials must not be sent to STS")
}

// TestAWSCredentials_ValidationEndpointIsNotSerialized verifies that the runtime
// endpoint never reaches the keyring or JSON output.
func TestAWSCredentials_ValidationEndpointIsNotSerialized(t *testing.T) {
	creds := validAWSCredentials()
	creds.SetValidationEndpoint("http://127.0.0.1:4566")

	raw, err := json.Marshal(creds)
	require.NoError(t, err)
	out := string(raw)
	assert.NotContains(t, out, "4566")
	assert.NotContains(t, strings.ToLower(out), "endpoint")
	assert.Contains(t, out, `"region":"us-east-1"`, "regular fields are still serialized")
}

// TestAWSCredentials_ValidateIncompleteSignature verifies that an HTTP 400
// IncompleteSignature answer is a credential rejection, not an unavailable validation.
func TestAWSCredentials_ValidateIncompleteSignature(t *testing.T) {
	body := `<ErrorResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <Error><Type>Sender</Type><Code>IncompleteSignature</Code><Message>bad signature</Message></Error>
  <RequestId>req-4</RequestId>
</ErrorResponse>`
	server, _ := fakeSTS(t, http.StatusBadRequest, body)
	creds := validAWSCredentials()
	creds.SetValidationEndpoint(server.URL)

	_, err := creds.Validate(context.Background())
	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.NotErrorIs(t, err, errUtils.ErrCredentialValidationUnavailable)
}

// TestAWSCredentials_ValidateIncompleteCredentials verifies that a missing secret is
// reported as a local failure without contacting STS.
func TestAWSCredentials_ValidateIncompleteCredentials(t *testing.T) {
	server, calls := fakeSTS(t, http.StatusOK, fakeSTSCallerIdentityXML)
	creds := validAWSCredentials()
	creds.SecretAccessKey = ""
	creds.SetValidationEndpoint(server.URL)

	_, err := creds.Validate(context.Background())
	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.NotErrorIs(t, err, errUtils.ErrCredentialValidationUnavailable)
	assert.NotContains(t, err.Error(), "could not be reached")
	assert.Equal(t, int32(0), calls.Load())
}
