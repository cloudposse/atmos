package step

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

var _ = schema.WorkflowStep{Method: "", Expect: &schema.HTTPExpect{Status: []int{}, Response: []string{}}}

func TestHTTPHandler_ExpectationRetriesOnlySafeMethods(t *testing.T) {
	t.Parallel()
	for _, method := range []struct {
		name, value string
		attempts    int
	}{
		{"default GET", "", 3},
		{"GET", http.MethodGet, 3},
		{"normalized GET", " get ", 3},
		{"HEAD", http.MethodHead, 3},
		{"OPTIONS", http.MethodOptions, 3},
		{"POST", http.MethodPost, 1},
		{"PUT", http.MethodPut, 1},
		{"PATCH", http.MethodPatch, 1},
		{"DELETE", http.MethodDelete, 1},
	} {
		for _, mismatch := range []struct {
			name    string
			status  int
			expect  *schema.HTTPExpect
			wantErr error
		}{
			{"successful response body", http.StatusOK, &schema.HTTPExpect{Response: []string{"ready"}}, errUtils.ErrHTTPStepUnexpectedResponse},
			{"successful response status", http.StatusCreated, &schema.HTTPExpect{Status: []int{http.StatusOK}}, errUtils.ErrHTTPStepUnexpectedStatus},
			{"client error status", http.StatusNotFound, &schema.HTTPExpect{Status: []int{http.StatusOK}}, errUtils.ErrHTTPStepUnexpectedStatus},
		} {
			t.Run(method.name+"/"+mismatch.name, func(t *testing.T) {
				t.Parallel()
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					w.WriteHeader(mismatch.status)
					_, err := w.Write([]byte("pending"))
					assert.NoError(t, err)
				}))
				defer server.Close()
				step := &schema.WorkflowStep{Name: "expectation", Type: "http", URL: server.URL, Method: method.value, Expect: mismatch.expect, Retry: fastRetry(t, 3)}
				result, err := mustGetHTTPHandler(t).Execute(context.Background(), step, NewVariables())
				require.ErrorIs(t, err, mismatch.wantErr)
				require.NotNil(t, result)
				assert.EqualValues(t, method.attempts, calls.Load())
				assert.Equal(t, method.attempts, result.Metadata[metaAttempts])
			})
		}
	}
}

// Existing transport/status rules and explicit conditions remain independent of
// expectation polling, including for operators that configure retryable writes.
func TestHTTPHandler_WriteRetryRulesRemainExplicit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		status     int
		conditions []string
	}{
		{name: "server error", status: http.StatusServiceUnavailable},
		{name: "rate limit", status: http.StatusTooManyRequests},
		{name: "explicit condition", status: http.StatusBadRequest, conditions: []string{"^400 "}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					w.WriteHeader(tc.status)
					return
				}
				_, err := w.Write([]byte("ready"))
				assert.NoError(t, err)
			}))
			defer server.Close()
			step := &schema.WorkflowStep{Name: "write retry", Type: "http", URL: server.URL, Method: http.MethodPost, Expect: &schema.HTTPExpect{Status: []int{200}, Response: []string{"ready"}}, Retry: fastRetry(t, 3, tc.conditions...)}
			result, err := mustGetHTTPHandler(t).Execute(context.Background(), step, NewVariables())
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.EqualValues(t, 2, calls.Load())
			assert.Equal(t, 2, result.Metadata[metaAttempts])
		})
	}
}
