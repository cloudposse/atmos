package aws

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	pkghttp "github.com/cloudposse/atmos/pkg/http"
	atmosio "github.com/cloudposse/atmos/pkg/io"
)

const (
	consoleTestAccessKeyID  = "ASIAQ7XK3MZP2R000004"
	consoleTestSecretKey    = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYzq00000004"
	consoleTestSessionToken = "IQoJb3JpZ2luX2VjEJr//////////wEaCXVzLWVhc3QtMSJHMEUCIQDexampleTokenValue0000000004"
)

func consoleTestCredentials() *types.AWSCredentials {
	return &types.AWSCredentials{
		AccessKeyID:     consoleTestAccessKeyID,
		SecretAccessKey: consoleTestSecretKey,
		SessionToken:    consoleTestSessionToken,
	}
}

// secretForms returns each credential value in the encodings a leaked federation
// URL or session document would carry it: raw, query-escaped, and path-escaped.
func secretForms() []string {
	var forms []string
	for _, secret := range []string{consoleTestAccessKeyID, consoleTestSecretKey, consoleTestSessionToken} {
		forms = append(forms, secret, url.QueryEscape(secret), url.PathEscape(secret))
	}
	return forms
}

// TestGetConsoleURL_FederationErrorDoesNotLeakCredentials is the regression for
// `atmos auth console` printing the full federation URL, with the URL-encoded
// access key, secret key, and session token, when the federation call fails.
func TestGetConsoleURL_FederationErrorDoesNotLeakCredentials(t *testing.T) {
	tests := []struct {
		name     string
		response func(req *http.Request) (*http.Response, error)
	}{
		{
			// What net/http returns for a transport failure: the error names the full URL.
			name: "transport error",
			response: func(req *http.Request) (*http.Response, error) {
				return nil, &url.Error{Op: "Get", URL: req.URL.String(), Err: errors.New("Forbidden")}
			},
		},
		{
			// A non-200 answer: the status error names the request URL, and the body may echo it.
			name: "error status",
			response: func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: http.StatusBadRequest,
					Header:     http.Header{"Content-Type": []string{"text/plain"}},
					Body:       io.NopCloser(bytes.NewBufferString("bad request for " + req.URL.String())),
				}, nil
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			client := pkghttp.NewMockClient(ctrl)
			var requested string
			client.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
				requested = req.URL.String()
				return tt.response(req)
			})

			_, _, err := NewConsoleURLGenerator(client).GetConsoleURL(context.Background(), consoleTestCredentials(), types.ConsoleURLOptions{})
			require.Error(t, err)
			require.Contains(t, requested, url.QueryEscape(consoleTestSessionToken), "precondition: the request URL carries the credentials")

			msg := err.Error()
			for _, form := range secretForms() {
				assert.NotContains(t, msg, form, "the error must not contain any form of the credentials")
			}
			assert.NotContains(t, msg, "Session=%7B", "the Session query parameter must be redacted")
			assert.Contains(t, msg, AWSFederationEndpoint, "the error still names the endpoint")
			assert.ErrorIs(t, err, errUtils.ErrHTTPRequestFailed)
		})
	}
}

// TestGetConsoleURL_FederationErrorKeepsCause verifies that redaction keeps a
// cancellation or deadline in the error chain so callers can still detect it.
func TestGetConsoleURL_FederationErrorKeepsCause(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := pkghttp.NewMockClient(ctrl)
	client.EXPECT().Do(gomock.Any()).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		return nil, &url.Error{Op: "Get", URL: req.URL.String(), Err: context.DeadlineExceeded}
	})

	_, _, err := NewConsoleURLGenerator(client).GetConsoleURL(context.Background(), consoleTestCredentials(), types.ConsoleURLOptions{})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.NotContains(t, err.Error(), url.QueryEscape(consoleTestSessionToken))
}

// TestGetConsoleURL_RegistersCredentialsWithMasker verifies that the credentials
// sent to the federation endpoint are registered with the output masker, so a
// stray print of the URL or session document is masked too.
func TestGetConsoleURL_RegistersCredentialsWithMasker(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := pkghttp.NewMockClient(ctrl)
	client.EXPECT().Do(gomock.Any()).Return(&http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(`{"SigninToken":"synthetic-signin-token"}`)),
	}, nil)

	loginURL, _, err := NewConsoleURLGenerator(client).GetConsoleURL(context.Background(), consoleTestCredentials(), types.ConsoleURLOptions{})
	require.NoError(t, err)

	masked := atmosio.MaskString("session " + url.QueryEscape(consoleTestSessionToken) + " key " + consoleTestSecretKey)
	assert.NotContains(t, masked, consoleTestSecretKey)
	assert.NotContains(t, masked, url.QueryEscape(consoleTestSessionToken))
	// The sign-in URL itself is the command's output and must stay usable.
	assert.True(t, strings.Contains(atmosio.MaskString(loginURL), "synthetic-signin-token"), "the sign-in token must not be masked")
}
