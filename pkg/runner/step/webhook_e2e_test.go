package step

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestWebhook_EndToEndThroughExecutor exercises the full workflow execution path:
// a real local HTTP server, dispatched through the StepExecutor + step registry
// (the same machinery `atmos workflow` uses), with the response captured as a
// variable that a downstream step can template against.
func TestWebhook_EndToEndThroughExecutor(t *testing.T) {
	var gotPath, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"id":"run-99"}`)
	}))
	defer srv.Close()

	workflow := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{
				Name:    "trigger",
				Type:    "webhook",
				URL:     srv.URL + "/deploy",
				Method:  "POST",
				Headers: map[string]string{"Content-Type": "application/json"},
				Body:    `{"env":"prod"}`,
				Expect:  &schema.WebhookExpect{Status: []int{200}, Response: []string{`/"id"/`}},
			},
		},
	}

	executor := NewStepExecutor()
	err := executor.RunAll(context.Background(), workflow)
	require.NoError(t, err)

	// The webhook response was received and stored as a variable.
	assert.Equal(t, "/deploy", gotPath)
	assert.JSONEq(t, `{"env":"prod"}`, gotBody)

	result, ok := executor.GetResult("trigger")
	require.True(t, ok)
	assert.JSONEq(t, `{"id":"run-99"}`, result.Value)
	assert.Equal(t, http.StatusOK, result.Metadata[metaStatusCode])

	// A downstream step can reference the webhook response via templates.
	rendered, err := executor.Variables().Resolve(`{{ .steps.trigger.value }}`)
	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"run-99"}`, rendered)
}

// TestWebhook_EndToEndRetryThroughExecutor verifies retry composes end-to-end:
// a flaky server (503 then 200) succeeds when run through the executor with a retry policy.
func TestWebhook_EndToEndRetryThroughExecutor(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	workflow := &schema.WorkflowDefinition{
		Steps: []schema.WorkflowStep{
			{
				Name:  "flaky",
				Type:  "webhook",
				URL:   srv.URL,
				Retry: fastRetry(t, 4),
			},
		},
	}

	executor := NewStepExecutor()
	require.NoError(t, executor.RunAll(context.Background(), workflow))
	assert.Equal(t, 2, calls)

	result, ok := executor.GetResult("flaky")
	require.True(t, ok)
	assert.Equal(t, "ok", result.Value)
	assert.Equal(t, 2, result.Metadata[metaAttempts])
}
