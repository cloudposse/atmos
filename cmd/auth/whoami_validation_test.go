package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	authTypes "github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Compile-time sentinel for the schema field the endpoint tests configure.
var _ = schema.Identity{Spec: map[string]interface{}{"endpoint_url": ""}}

const whoamiFakeSTSResponse = `<GetCallerIdentityResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <GetCallerIdentityResult>
    <Arn>arn:aws:iam::111111111111:user/devuser</Arn>
    <UserId>AIDAEXAMPLE</UserId>
    <Account>111111111111</Account>
  </GetCallerIdentityResult>
  <ResponseMetadata><RequestId>req-1</RequestId></ResponseMetadata>
</GetCallerIdentityResponse>`

const whoamiFakeSTSRejection = `<ErrorResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <Error><Type>Sender</Type><Code>ExpiredToken</Code><Message>The security token included in the request is expired</Message></Error>
  <RequestId>req-2</RequestId>
</ErrorResponse>`

// whoamiFakeSTS starts a local STS stand-in answering with status and body.
func whoamiFakeSTS(t *testing.T, status int, body string) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server.URL
}

// unreachableLocalEndpoint returns a loopback URL with nothing listening on it.
func unreachableLocalEndpoint(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := listener.Addr().String()
	require.NoError(t, listener.Close())
	return "http://" + addr
}

// captureWhoamiStreams points the data and UI channels at in-memory buffers.
func captureWhoamiStreams(t *testing.T) (stdout, stderr *bytes.Buffer) {
	t.Helper()
	stdout, stderr = &bytes.Buffer{}, &bytes.Buffer{}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(consoleLinkTestStreams{output: stdout, error: stderr}))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)
	t.Cleanup(func() {
		restore, restoreErr := iolib.NewContext()
		require.NoError(t, restoreErr)
		data.InitWriter(restore)
		ui.InitFormatter(restore)
	})
	return stdout, stderr
}

// awsWhoami builds a WhoamiInfo holding AWS credentials that expire in validFor and
// validate against endpoint.
func awsWhoami(validFor time.Duration, endpoint string) *authTypes.WhoamiInfo {
	creds := &authTypes.AWSCredentials{
		AccessKeyID:     "AKIASYNTHETIC0000000",
		SecretAccessKey: "synthetic-secret",
		SessionToken:    "synthetic-session-token",
		Region:          "us-east-1",
		Expiration:      time.Now().Add(validFor).UTC().Format(time.RFC3339),
	}
	creds.SetValidationEndpoint(endpoint)
	return &authTypes.WhoamiInfo{Identity: "devuser", Provider: "aws-user", Region: "us-east-1", Credentials: creds}
}

// TestReportWhoami_UnreachableSTSKeepsExitZero is the regression for whoami failing
// valid, unexpired credentials when STS cannot be reached: it exits 0, keeps the JSON
// on stdout valid, and warns on stderr that validation could not be performed.
func TestReportWhoami_UnreachableSTSKeepsExitZero(t *testing.T) {
	for _, format := range []string{"json", ""} {
		t.Run("format="+format, func(t *testing.T) {
			stdout, stderr := captureWhoamiStreams(t)

			err := reportWhoami(context.Background(), awsWhoami(time.Hour, unreachableLocalEndpoint(t)), format)
			require.NoError(t, err, "valid unexpired credentials must not fail when STS is unreachable")

			assert.Contains(t, plainWhoamiText(stderr), "Could not validate the credentials")
			assert.NotContains(t, plainWhoamiText(stderr), "may be expired or invalid")
			if format == "json" {
				var doc map[string]any
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), "stdout must stay valid JSON")
				assert.Equal(t, "devuser", doc["identity"])
			}
		})
	}
}

// TestReportWhoami_UnreachableSTSWithExpiredCredentialsFails verifies the negative path:
// expired credentials still fail even when STS cannot be reached.
func TestReportWhoami_UnreachableSTSWithExpiredCredentialsFails(t *testing.T) {
	stdout, _ := captureWhoamiStreams(t)

	err := reportWhoami(context.Background(), awsWhoami(-time.Hour, unreachableLocalEndpoint(t)), "json")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrExpiredCredentials)
	assert.Equal(t, 1, errUtils.GetExitCode(err))
	var doc map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), "stdout must stay valid JSON")
}

// TestReportWhoami_RejectedCredentialsFail verifies that credentials STS rejects fail
// with exit 1 even though their expiration time is in the future.
func TestReportWhoami_RejectedCredentialsFail(t *testing.T) {
	stdout, stderr := captureWhoamiStreams(t)

	err := reportWhoami(context.Background(), awsWhoami(time.Hour, whoamiFakeSTS(t, http.StatusForbidden, whoamiFakeSTSRejection)), "json")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrExpiredCredentials)
	assert.Equal(t, 1, errUtils.GetExitCode(err))
	assert.Contains(t, plainWhoamiText(stderr), "may be expired or invalid")
	var doc map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc), "stdout must stay valid JSON")
}

// TestReportWhoami_ValidatesAgainstConfiguredEndpoint verifies that successful
// validation against the configured endpoint populates the principal and account.
func TestReportWhoami_ValidatesAgainstConfiguredEndpoint(t *testing.T) {
	stdout, stderr := captureWhoamiStreams(t)

	err := reportWhoami(context.Background(), awsWhoami(time.Hour, whoamiFakeSTS(t, http.StatusOK, whoamiFakeSTSResponse)), "json")
	require.NoError(t, err)
	assert.NotContains(t, plainWhoamiText(stderr), "Could not validate")
	var doc map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &doc))
	assert.Equal(t, "arn:aws:iam::111111111111:user/devuser", doc["principal"])
	assert.Equal(t, "111111111111", doc["account"])
}

// TestConfigureWhoamiValidation verifies that validation is pointed at the endpoint the
// identity authenticates against: the identity's spec.endpoint_url, else the provider's.
func TestConfigureWhoamiValidation(t *testing.T) {
	stsURL := whoamiFakeSTS(t, http.StatusOK, whoamiFakeSTSResponse)

	tests := []struct {
		name       string
		identities map[string]schema.Identity
		providers  map[string]schema.Provider
		wantValid  bool
	}{
		{
			name:       "identity spec.endpoint_url",
			identities: map[string]schema.Identity{"devuser": {Kind: "aws/user", Spec: map[string]interface{}{"endpoint_url": stsURL}}},
			wantValid:  true,
		},
		{
			name:       "provider spec.endpoint_url when the identity has none",
			identities: map[string]schema.Identity{"devuser": {Kind: "aws/assume-role"}},
			providers:  map[string]schema.Provider{"aws-user": {Kind: "aws/iam-identity-center", Spec: map[string]interface{}{"endpoint_url": stsURL}}},
			wantValid:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			m := authTypes.NewMockAuthManager(ctrl)
			m.EXPECT().GetIdentities().Return(tt.identities)
			m.EXPECT().GetProviders().Return(tt.providers)

			// Start from an unreachable endpoint so only the configured one can succeed.
			whoami := awsWhoami(time.Hour, unreachableLocalEndpoint(t))
			configureWhoamiValidation(m, whoami)

			check := validateCredentials(context.Background(), whoami)
			assert.Equal(t, tt.wantValid, check.valid)
			assert.NoError(t, check.unverified)
			assert.Equal(t, "111111111111", whoami.Account)
		})
	}

	t.Run("no endpoint configured leaves the default endpoint", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		m := authTypes.NewMockAuthManager(ctrl)
		m.EXPECT().GetIdentities().Return(map[string]schema.Identity{"devuser": {Kind: "aws/user"}})
		m.EXPECT().GetProviders().Return(nil)

		unreachable := unreachableLocalEndpoint(t)
		whoami := awsWhoami(time.Hour, unreachable)
		configureWhoamiValidation(m, whoami)

		check := validateCredentials(context.Background(), whoami)
		assert.True(t, check.valid, "unexpired credentials stay valid when validation is unavailable")
		require.ErrorIs(t, check.unverified, errUtils.ErrCredentialValidationUnavailable)
		assert.Contains(t, check.unverified.Error(), unreachable, "the previously set endpoint must be kept")
	})

	t.Run("credentials without endpoint support are left alone", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		m := authTypes.NewMockAuthManager(ctrl)
		// No GetIdentities/GetProviders expectations: they must not be consulted.
		creds := authTypes.NewMockICredentials(ctrl)
		configureWhoamiValidation(m, &authTypes.WhoamiInfo{Identity: "devuser", Credentials: creds})
		configureWhoamiValidation(nil, &authTypes.WhoamiInfo{Identity: "devuser"})
	})
}

// plainWhoamiText returns the buffer with ANSI styling removed and whitespace
// collapsed, so assertions do not depend on color or terminal word wrapping.
func plainWhoamiText(buf *bytes.Buffer) string {
	return strings.Join(strings.Fields(ansi.Strip(buf.String())), " ")
}
