package okta

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/creds"
)

// captureDebugLogs captures the global logrus output at debug level for the test.
func captureDebugLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	origOut, origLevel := logrus.StandardLogger().Out, logrus.GetLevel()
	logrus.SetOutput(&buf)
	logrus.SetLevel(logrus.DebugLevel)
	t.Cleanup(func() {
		logrus.SetOutput(origOut)
		logrus.SetLevel(origLevel)
	})
	return &buf
}

// Regression (code-scanning: clear-text logging of sensitive information): with debug logging on,
// a full Okta login must not log the password, the session token carried in the redirect URL,
// or the SAML assertion, which can be exchanged for AWS credentials until it expires.
func TestAuthenticate_DebugLogsContainNoSecrets(t *testing.T) {
	const assertionMarker = "secret-assertion-body"
	saml := base64.StdEncoding.EncodeToString([]byte("<saml:Assertion>" + assertionMarker + "</saml:Assertion>"))
	logs := captureDebugLogs(t)

	f := newFakeOkta(t)
	f.authn = func(map[string]any) string { return `{"status":"SUCCESS","sessionToken":"secret-session-token"}` }
	f.extra["/login/sessionCookieRedirect"] = func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, samlTargetPage(f.ts.URL, saml))
	}

	oc, loginDetails := f.client(true)
	got, err := oc.Authenticate(loginDetails)
	require.NoError(t, err)
	require.Equal(t, saml, got)

	output := logs.String()
	require.NotEmpty(t, output, "debug logging must be captured for this test to mean anything")
	assert.NotContains(t, output, loginDetails.Password)
	assert.NotContains(t, output, "secret-session-token")
	assert.NotContains(t, output, assertionMarker)
	assert.NotContains(t, output, saml)
}

// Regression: createSession logged loginDetails with %v (including the password) when the
// session token was empty.
func TestCreateSession_EmptyTokenDoesNotLogPassword(t *testing.T) {
	logs := captureDebugLogs(t)

	_, _, err := (&Client{}).createSession(&creds.LoginDetails{Username: "user@example.com", Password: "hunter2"}, "")
	require.Error(t, err)

	assert.NotContains(t, logs.String(), "hunter2")
	assert.Contains(t, logs.String(), "unable to create an Okta session")
}
