package pro

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/script"
	starlarkengine "github.com/cloudposse/atmos/pkg/script/starlark"
)

// Not parallel: initializes the global I/O masker and isolates CI environment settings.
func TestAutomationErrorBuilderSentryReporting(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "false")
	require.NoError(t, iolib.Initialize())
	iolib.RegisterSecret("automation-private-value")
	events := make(chan *sentry.Event, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		events <- decodeExceptionEnvelope(t, data)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	config := &schema.AtmosConfiguration{}
	config.Errors.Sentry = schema.SentryConfig{
		Enabled: true,
		DSN:     strings.Replace(server.URL, "://", "://public@", 1) + "/1",
	}
	reporter := NewErrorReporter(config, "automation-test", ExceptionTransportOptions{})
	_, scriptErr := starlarkengine.New().Execute(t.Context(), script.Spec{Name: "release.star", Source: `
(errors.build("Deployment blocked")
    .with_hint("Set an owner before deploying")
    .with_hint("Diagnostic automation-private-value")
    .with_context("component", "api")
    .with_exit_code(3)
    .fail())
`})
	require.Error(t, scriptErr)
	reporter.Capture(scriptErr, map[string]string{"command": "atmos"})
	reporter.Capture(scriptErr, map[string]string{"command": "atmos"})
	reporter.Flush(t.Context())
	require.Len(t, events, 1, "the same script failure is reported once")
	event := <-events
	assert.Equal(t, "3", event.Tags["atmos.exit_code"])
	assert.Equal(t, "atmos", event.Tags["atmos.command"])
	assert.Equal(t, "automation-test", event.Tags["atmos.execution_id"])
	require.NotEmpty(t, event.Exception)
	var hints []string
	for _, breadcrumb := range event.Breadcrumbs {
		if breadcrumb.Category == "hint" {
			hints = append(hints, breadcrumb.Message)
		}
	}
	assert.Contains(t, hints, "Set an owner before deploying")
	encoded, err := json.Marshal(event)
	require.NoError(t, err)
	assert.Contains(t, string(encoded), "component=api")
	assert.NotContains(t, string(encoded), "automation-private-value")
}
