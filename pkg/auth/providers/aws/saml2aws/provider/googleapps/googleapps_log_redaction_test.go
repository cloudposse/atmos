package googleapps

import (
	"bytes"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// Regression (code-scanning: clear-text logging of sensitive information): form extraction logged
// every valued input, including authentication state, passwords and tokens. Only names may be logged.
func TestExtractInputsByFormQuery_DebugLogsNamesOnly(t *testing.T) {
	logs := captureDebugLogs(t)
	doc := mustDoc(t, `<form id="f" action="/next">`+
		`<input name="gxf" value="secret-gxf-state">`+
		`<input name="Passwd" value="hunter2-password">`+
		`<input name="TL" value="secret-tl-token"></form>`, "https://accounts.google.com/start")

	form, _, err := extractInputsByFormQuery(doc, "#f")
	require.NoError(t, err)
	// Extraction itself is unchanged.
	assert.Equal(t, "secret-gxf-state", form.Get("gxf"))
	assert.Equal(t, "hunter2-password", form.Get("Passwd"))

	output := logs.String()
	require.NotEmpty(t, output, "debug logging must be captured for this test to mean anything")
	assert.Contains(t, output, "gxf")
	assert.Contains(t, output, "Passwd")
	assert.NotContains(t, output, "secret-gxf-state")
	assert.NotContains(t, output, "hunter2-password")
	assert.NotContains(t, output, "secret-tl-token")
}

// A full password login with debug logging on must not log the form values it passes through.
func TestAuthenticate_DebugLogsContainNoFormValues(t *testing.T) {
	logs := captureDebugLogs(t)
	f := newFakeGoogle(t, passwordFlowRoutes(t, static(samlPage)))
	kc, loginDetails := f.client()

	got, err := kc.Authenticate(loginDetails)
	require.NoError(t, err)
	assert.Equal(t, testSAMLResponse, got)

	output := logs.String()
	require.NotEmpty(t, output, "debug logging must be captured for this test to mean anything")
	assert.NotContains(t, output, "galx-token")
	assert.NotContains(t, output, loginDetails.Password)
}
