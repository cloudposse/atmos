package pro

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
)

func TestHandleAPIResponse_PayloadTooLarge(t *testing.T) {
	t.Parallel()
	for _, body := range []string{"Request Entity Too Large", "", `{"error":"too big"}`} {
		t.Run(body, func(t *testing.T) {
			t.Parallel()
			response := &http.Response{StatusCode: http.StatusRequestEntityTooLarge, Body: io.NopCloser(strings.NewReader(body))}
			err := handleAPIResponse(response, "Upload")
			require.ErrorIs(t, err, errUtils.ErrPayloadTooLarge)
			assert.NotErrorIs(t, err, errUtils.ErrFailedToUnmarshalAPIResponse)
			var apiErr *APIError
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, http.StatusRequestEntityTooLarge, apiErr.StatusCode)
			assert.False(t, apiErr.IsRetryable())
			assert.Contains(t, strings.Join(cockroachErrors.GetAllHints(err), " "), "Atmos Pro body size limit")
		})
	}
}

func TestDoWithRetry_PayloadTooLargeIsNotRetried(t *testing.T) {
	t.Parallel()
	calls := 0
	refresher := newMockRefresher()
	err := doWithRetry("Upload", func() error {
		calls++
		return wrapErr(errUtils.ErrFailedToUploadStacks, &APIError{
			StatusCode: http.StatusRequestEntityTooLarge, Operation: "Upload", Err: errUtils.ErrPayloadTooLarge,
		})
	}, refresher, fastRetryConfig())
	require.ErrorIs(t, err, errUtils.ErrPayloadTooLarge)
	assert.Equal(t, 1, calls)
	assert.Zero(t, refresher.calls)
}

func TestUploadExecData_PayloadTooLarge(t *testing.T) {
	t.Parallel()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
	}))
	defer server.Close()
	client := &AtmosProAPIClient{
		BaseURL: server.URL, BaseAPIEndpoint: "api", APIToken: "test-token", HTTPClient: server.Client(),
	}
	response, err := client.UploadExecData(&dtos.ExecDataUploadRequest{
		ExecutionID: "11111111-1111-4111-8111-111111111111", Data: json.RawMessage(`{"version":1}`),
	})
	assert.Nil(t, response)
	require.ErrorIs(t, err, errUtils.ErrPayloadTooLarge)
	assert.NotErrorIs(t, err, errUtils.ErrFailedToUnmarshalAPIResponse)
	assert.Equal(t, int32(1), calls.Load())
}
