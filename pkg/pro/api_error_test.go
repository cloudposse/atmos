package pro

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIError_Error(t *testing.T) {
	err := &APIError{
		StatusCode: 500,
		Operation:  "UploadInstanceStatus",
		Err:        fmt.Errorf("internal server error"),
	}
	assert.Equal(t, "UploadInstanceStatus: HTTP 500: internal server error", err.Error())
}

func TestAPIError_Unwrap(t *testing.T) {
	inner := fmt.Errorf("inner error")
	err := &APIError{StatusCode: 401, Operation: "Upload", Err: inner}
	assert.Equal(t, inner, err.Unwrap())
}

func TestAPIError_ErrorsAs(t *testing.T) {
	inner := fmt.Errorf("inner")
	apiErr := &APIError{StatusCode: 503, Operation: "Upload", Err: inner}
	wrapped := fmt.Errorf("wrapping: %w", apiErr)

	var extracted *APIError
	require.True(t, errors.As(wrapped, &extracted))
	assert.Equal(t, 503, extracted.StatusCode)
	assert.Equal(t, "Upload", extracted.Operation)
}

func TestAPIError_ErrorsIs(t *testing.T) {
	sentinel := errors.New("sentinel")
	apiErr := &APIError{StatusCode: 500, Operation: "Op", Err: sentinel}
	assert.True(t, errors.Is(apiErr, sentinel))
}

func TestAPIError_IsRetryable(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       bool
	}{
		{"401 is retryable", 401, true},
		{"500 is retryable", 500, true},
		{"502 is retryable", 502, true},
		{"503 is retryable", 503, true},
		{"400 not retryable", 400, false},
		{"403 not retryable", 403, false},
		{"404 not retryable", 404, false},
		{"200 not retryable", 200, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &APIError{StatusCode: tt.statusCode, Operation: "Op", Err: fmt.Errorf("err")}
			assert.Equal(t, tt.want, err.IsRetryable())
		})
	}
}

func TestAPIError_IsAuthError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		want       bool
	}{
		{"401 is auth error", 401, true},
		{"403 not auth error", 403, false},
		{"500 not auth error", 500, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &APIError{StatusCode: tt.statusCode, Operation: "Op", Err: fmt.Errorf("err")}
			assert.Equal(t, tt.want, err.IsAuthError())
		})
	}
}
