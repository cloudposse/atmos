package step

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func webhookIntPtr(i int) *int { return &i }

func webhookDurPtr(t *testing.T, s string) *time.Duration {
	t.Helper()
	d, err := time.ParseDuration(s)
	require.NoError(t, err)
	return &d
}

// fastRetry builds a constant-backoff retry config with a tiny delay for tests.
func fastRetry(t *testing.T, maxAttempts int, conditions ...string) *schema.RetryConfig {
	t.Helper()
	return &schema.RetryConfig{
		MaxAttempts:     webhookIntPtr(maxAttempts),
		BackoffStrategy: schema.BackoffConstant,
		InitialDelay:    webhookDurPtr(t, "1ms"),
		Conditions:      conditions,
	}
}

func TestWebhookHandler_Validate(t *testing.T) {
	handler, ok := Get("webhook")
	require.True(t, ok)

	tests := []struct {
		name    string
		step    *schema.WorkflowStep
		wantErr error
	}{
		{
			name:    "missing url",
			step:    &schema.WorkflowStep{Name: "wh", Type: "webhook"},
			wantErr: errUtils.ErrWebhookURLRequired,
		},
		{
			name:    "invalid method",
			step:    &schema.WorkflowStep{Name: "wh", Type: "webhook", URL: "https://example.com", Method: "FETCH"},
			wantErr: errUtils.ErrWebhookInvalidMethod,
		},
		{
			name: "body and form conflict",
			step: &schema.WorkflowStep{
				Name: "wh", Type: "webhook", URL: "https://example.com",
				Body: "raw", Form: map[string]string{"a": "b"},
			},
			wantErr: errUtils.ErrWebhookBodyFormConflict,
		},
		{
			name: "invalid expect regex",
			step: &schema.WorkflowStep{
				Name: "wh", Type: "webhook", URL: "https://example.com",
				Expect: &schema.WebhookExpect{Response: []string{"("}},
			},
			wantErr: errUtils.ErrWebhookInvalidExpectPattern,
		},
		{
			name: "valid",
			step: &schema.WorkflowStep{
				Name: "wh", Type: "webhook", URL: "https://example.com", Method: "post",
				Expect: &schema.WebhookExpect{Status: []int{200}, Response: []string{"/ok/"}},
			},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.Validate(tt.step)
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestWebhookHandler_GetWithQuery runs an end-to-end GET against a real local server.
func TestWebhookHandler_GetWithQuery(t *testing.T) {
	var gotMethod, gotQuery, gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotQuery = r.URL.Query().Get("ref")
		gotHeader = r.Header.Get("X-Token")
		_, _ = io.WriteString(w, "pong")
	}))
	defer srv.Close()

	handler, _ := Get("webhook")
	step := &schema.WorkflowStep{
		Name:    "ping",
		Type:    "webhook",
		URL:     srv.URL,
		Method:  "GET",
		Query:   map[string]string{"ref": "abc123"},
		Headers: map[string]string{"X-Token": "secret"},
	}
	require.NoError(t, handler.Validate(step))

	result, err := handler.Execute(context.Background(), step, NewVariables())
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "pong", result.Value)
	assert.Equal(t, http.MethodGet, gotMethod)
	assert.Equal(t, "abc123", gotQuery)
	assert.Equal(t, "secret", gotHeader)
	assert.Equal(t, http.StatusOK, result.Metadata[metaStatusCode])
	assert.Equal(t, 1, result.Metadata[metaAttempts])
}

// TestWebhookHandler_PostRawBody verifies raw body POST end-to-end.
func TestWebhookHandler_PostRawBody(t *testing.T) {
	var gotBody, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	handler, _ := Get("webhook")
	step := &schema.WorkflowStep{
		Name:    "post",
		Type:    "webhook",
		URL:     srv.URL,
		Method:  "POST",
		Headers: map[string]string{"Content-Type": "application/json"},
		Body:    `{"status":"deployed"}`,
	}
	require.NoError(t, handler.Validate(step))

	result, err := handler.Execute(context.Background(), step, NewVariables())
	require.NoError(t, err)
	assert.Equal(t, `{"status":"deployed"}`, gotBody)
	assert.Equal(t, "application/json", gotCT)
	assert.Equal(t, `{"ok":true}`, result.Value)
	assert.Equal(t, http.StatusCreated, result.Metadata[metaStatusCode])
}

// TestWebhookHandler_PostFormURLEncoded verifies form params default to urlencoded.
func TestWebhookHandler_PostFormURLEncoded(t *testing.T) {
	var gotCT, gotStatus, gotEnv string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		require.NoError(t, r.ParseForm())
		gotStatus = r.PostFormValue("status")
		gotEnv = r.PostFormValue("env")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	handler, _ := Get("webhook")
	step := &schema.WorkflowStep{
		Name:   "form",
		Type:   "webhook",
		URL:    srv.URL,
		Method: "POST",
		Form:   map[string]string{"status": "deployed", "env": "prod"},
	}
	require.NoError(t, handler.Validate(step))

	_, err := handler.Execute(context.Background(), step, NewVariables())
	require.NoError(t, err)
	assert.Equal(t, contentTypeForm, gotCT)
	assert.Equal(t, "deployed", gotStatus)
	assert.Equal(t, "prod", gotEnv)
}

// TestWebhookHandler_PostFormJSON verifies form params are JSON-encoded when Content-Type is JSON.
func TestWebhookHandler_PostFormJSON(t *testing.T) {
	var gotBody, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	handler, _ := Get("webhook")
	step := &schema.WorkflowStep{
		Name:    "formjson",
		Type:    "webhook",
		URL:     srv.URL,
		Method:  "POST",
		Headers: map[string]string{"Content-Type": "application/json"},
		Form:    map[string]string{"status": "deployed"},
	}
	require.NoError(t, handler.Validate(step))

	_, err := handler.Execute(context.Background(), step, NewVariables())
	require.NoError(t, err)
	assert.Equal(t, "application/json", gotCT)
	assert.JSONEq(t, `{"status":"deployed"}`, gotBody)
}

// TestWebhookHandler_ExpectStatusOverride confirms a non-2xx success code is accepted.
func TestWebhookHandler_ExpectStatusOverride(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted) // 202.
	}))
	defer srv.Close()

	handler, _ := Get("webhook")
	step := &schema.WorkflowStep{
		Name:   "expect",
		Type:   "webhook",
		URL:    srv.URL,
		Expect: &schema.WebhookExpect{Status: []int{202}},
	}
	require.NoError(t, handler.Validate(step))

	result, err := handler.Execute(context.Background(), step, NewVariables())
	require.NoError(t, err)
	assert.Equal(t, http.StatusAccepted, result.Metadata[metaStatusCode])
}

// TestWebhookHandler_ExpectResponseRegex covers both matching and non-matching bodies.
func TestWebhookHandler_ExpectResponseRegex(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantErr error
	}{
		{name: "matches", body: `{"status":"deployed"}`, wantErr: nil},
		{name: "no match", body: `{"status":"pending"}`, wantErr: errUtils.ErrWebhookUnexpectedResponse},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, tt.body)
			}))
			defer srv.Close()

			handler, _ := Get("webhook")
			step := &schema.WorkflowStep{
				Name:   "regex",
				Type:   "webhook",
				URL:    srv.URL,
				Expect: &schema.WebhookExpect{Response: []string{`/"status"\s*:\s*"deployed"/`}},
			}
			require.NoError(t, handler.Validate(step))

			_, err := handler.Execute(context.Background(), step, NewVariables())
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestWebhookHandler_RetryOn5xx verifies the step retries server errors and then succeeds.
func TestWebhookHandler_RetryOn5xx(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(w, "recovered")
	}))
	defer srv.Close()

	handler, _ := Get("webhook")
	step := &schema.WorkflowStep{
		Name:  "retry5xx",
		Type:  "webhook",
		URL:   srv.URL,
		Retry: fastRetry(t, 5),
	}
	require.NoError(t, handler.Validate(step))

	result, err := handler.Execute(context.Background(), step, NewVariables())
	require.NoError(t, err)
	assert.Equal(t, "recovered", result.Value)
	assert.Equal(t, 3, calls)
	assert.Equal(t, 3, result.Metadata[metaAttempts])
}

// TestWebhookHandler_RetryOn429 verifies 429 Too Many Requests is retried.
func TestWebhookHandler_RetryOn429(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	handler, _ := Get("webhook")
	step := &schema.WorkflowStep{
		Name:  "retry429",
		Type:  "webhook",
		URL:   srv.URL,
		Retry: fastRetry(t, 3),
	}
	require.NoError(t, handler.Validate(step))

	_, err := handler.Execute(context.Background(), step, NewVariables())
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
}

// TestWebhookHandler_NoRetryOn4xx is the negative path: 404 must fail fast, not retry.
func TestWebhookHandler_NoRetryOn4xx(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	handler, _ := Get("webhook")
	step := &schema.WorkflowStep{
		Name:  "no-retry",
		Type:  "webhook",
		URL:   srv.URL,
		Retry: fastRetry(t, 5),
	}
	require.NoError(t, handler.Validate(step))

	_, err := handler.Execute(context.Background(), step, NewVariables())
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWebhookUnexpectedStatus)
	assert.Equal(t, 1, calls, "404 must not be retried")
}

// TestWebhookHandler_RetryConditions verifies retry.conditions can force retry of an
// otherwise non-retryable status (e.g. 400).
func TestWebhookHandler_RetryConditions(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 2 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	handler, _ := Get("webhook")
	step := &schema.WorkflowStep{
		Name:  "conditions",
		Type:  "webhook",
		URL:   srv.URL,
		Retry: fastRetry(t, 3, `^400 `),
	}
	require.NoError(t, handler.Validate(step))

	_, err := handler.Execute(context.Background(), step, NewVariables())
	require.NoError(t, err)
	assert.Equal(t, 2, calls)
}

// TestWebhookHandler_TemplateResolution verifies url/headers/body resolve from env vars.
func TestWebhookHandler_TemplateResolution(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	vars := NewVariables()
	vars.SetEnv("JOB_ID", "42")
	vars.SetEnv("TOKEN", "t0ken")
	vars.SetEnv("SHA", "deadbeef")

	handler, _ := Get("webhook")
	step := &schema.WorkflowStep{
		Name:    "tmpl",
		Type:    "webhook",
		URL:     srv.URL + "/hook/{{ .env.JOB_ID }}",
		Method:  "POST",
		Headers: map[string]string{"Authorization": "Bearer {{ .env.TOKEN }}"},
		Body:    `{"sha":"{{ .env.SHA }}"}`,
	}
	require.NoError(t, handler.Validate(step))

	_, err := handler.Execute(context.Background(), step, vars)
	require.NoError(t, err)
	assert.Equal(t, "/hook/42", gotPath)
	assert.Equal(t, "Bearer t0ken", gotAuth)
	assert.JSONEq(t, `{"sha":"deadbeef"}`, gotBody)
}

// TestWebhookHandler_TransportError verifies an unreachable endpoint fails (no panic).
func TestWebhookHandler_TransportError(t *testing.T) {
	handler, _ := Get("webhook")
	step := &schema.WorkflowStep{
		Name:    "down",
		Type:    "webhook",
		URL:     "http://127.0.0.1:1", // Port 1 should refuse connections.
		Timeout: "200ms",
	}
	require.NoError(t, handler.Validate(step))

	_, err := handler.Execute(context.Background(), step, NewVariables())
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWebhookRequestFailed)
}
