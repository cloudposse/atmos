package pro

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// mockRefresher tracks RefreshToken calls and returns a configurable error.
type mockRefresher struct {
	calls     int
	returnErr error
}

func (m *mockRefresher) RefreshToken() error {
	m.calls++
	return m.returnErr
}

// newMockRefresher returns a mockRefresher that succeeds on RefreshToken.
func newMockRefresher() *mockRefresher {
	return &mockRefresher{}
}

// fastRetryConfig returns a retryConfig with minimal delays for fast tests.
func fastRetryConfig() retryConfig {
	return retryConfig{maxRetries: 3, baseDelay: time.Millisecond}
}

func TestDoWithRetry_SuccessOnFirstAttempt(t *testing.T) {
	cfg := fastRetryConfig()

	callCount := 0
	err := doWithRetry("TestOp", func() error {
		callCount++
		return nil
	}, newMockRefresher(), cfg)

	require.NoError(t, err)
	assert.Equal(t, 1, callCount)
}

func TestDoWithRetry_ServerErrorThenSuccess(t *testing.T) {
	cfg := fastRetryConfig()

	callCount := 0
	err := doWithRetry("TestOp", func() error {
		callCount++
		if callCount == 1 {
			return &APIError{StatusCode: 500, Operation: "TestOp", Err: fmt.Errorf("internal server error")}
		}
		return nil
	}, newMockRefresher(), cfg)

	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
}

func TestDoWithRetry_AuthErrorThenSuccess(t *testing.T) {
	cfg := fastRetryConfig()
	refresher := newMockRefresher()

	callCount := 0
	err := doWithRetry("TestOp", func() error {
		callCount++
		if callCount == 1 {
			return &APIError{StatusCode: 401, Operation: "TestOp", Err: fmt.Errorf("unauthorized")}
		}
		return nil
	}, refresher, cfg)

	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
	assert.Equal(t, 1, refresher.calls, "RefreshToken should be called once on 401")
}

func TestDoWithRetry_AuthRefreshFailureAbortsRetry(t *testing.T) {
	cfg := fastRetryConfig()
	refresher := &mockRefresher{returnErr: errors.Join(errUtils.ErrTokenRefreshFailed, fmt.Errorf("OIDC unavailable"))}

	callCount := 0
	err := doWithRetry("TestOp", func() error {
		callCount++
		return &APIError{StatusCode: 401, Operation: "TestOp", Err: fmt.Errorf("unauthorized")}
	}, refresher, cfg)

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrTokenRefreshFailed), "should contain ErrTokenRefreshFailed")
	assert.Equal(t, 1, callCount, "should not retry after refresh failure")
	assert.Equal(t, 1, refresher.calls)
}

func TestDoWithRetry_NonRetryableNonAPIError(t *testing.T) {
	cfg := fastRetryConfig()
	refresher := newMockRefresher()

	callCount := 0
	err := doWithRetry("TestOp", func() error {
		callCount++
		return errors.Join(errUtils.ErrFailedToCreateAuthRequest, fmt.Errorf("invalid URL"))
	}, refresher, cfg)

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrFailedToCreateAuthRequest), "should contain ErrFailedToCreateAuthRequest")
	assert.Equal(t, 1, callCount, "should not retry on ErrFailedToCreateAuthRequest")
	assert.Equal(t, 0, refresher.calls)
}

func TestDoWithRetry_NonRetryable400(t *testing.T) {
	cfg := fastRetryConfig()

	callCount := 0
	err := doWithRetry("TestOp", func() error {
		callCount++
		return &APIError{StatusCode: 400, Operation: "TestOp", Err: fmt.Errorf("bad request")}
	}, newMockRefresher(), cfg)

	require.Error(t, err)
	assert.Equal(t, 1, callCount, "should not retry on 400")
}

func TestDoWithRetry_NonRetryable403(t *testing.T) {
	cfg := fastRetryConfig()

	callCount := 0
	err := doWithRetry("TestOp", func() error {
		callCount++
		return &APIError{StatusCode: 403, Operation: "TestOp", Err: fmt.Errorf("forbidden")}
	}, newMockRefresher(), cfg)

	require.Error(t, err)
	assert.Equal(t, 1, callCount, "should not retry on 403")
}

func TestDoWithRetry_NonRetryable404(t *testing.T) {
	cfg := fastRetryConfig()

	callCount := 0
	err := doWithRetry("TestOp", func() error {
		callCount++
		return &APIError{StatusCode: 404, Operation: "TestOp", Err: fmt.Errorf("not found")}
	}, newMockRefresher(), cfg)

	require.Error(t, err)
	assert.Equal(t, 1, callCount, "should not retry on 404")
}

func TestDoWithRetry_AllRetriesExhausted(t *testing.T) {
	cfg := fastRetryConfig()

	callCount := 0
	err := doWithRetry("TestOp", func() error {
		callCount++
		return &APIError{StatusCode: 502, Operation: "TestOp", Err: fmt.Errorf("bad gateway")}
	}, newMockRefresher(), cfg)

	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrUploadRetryExhausted))
	assert.Equal(t, 4, callCount, "1 initial + 3 retries")
}

func TestDoWithRetry_NetworkErrorRetried(t *testing.T) {
	cfg := fastRetryConfig()

	callCount := 0
	err := doWithRetry("TestOp", func() error {
		callCount++
		if callCount == 1 {
			return fmt.Errorf("connection refused")
		}
		return nil
	}, newMockRefresher(), cfg)

	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
}

func TestDoWithRetry_ExponentialBackoff(t *testing.T) {
	// Use a measurable base delay to verify exponential increase.
	cfg := retryConfig{maxRetries: 3, baseDelay: 10 * time.Millisecond}

	start := time.Now()
	err := doWithRetry("TestOp", func() error {
		return &APIError{StatusCode: 503, Operation: "TestOp", Err: fmt.Errorf("unavailable")}
	}, newMockRefresher(), cfg)

	require.Error(t, err)
	elapsed := time.Since(start)
	// Exponential backoff: 10ms + 20ms + 40ms = 70ms minimum.
	assert.GreaterOrEqual(t, elapsed.Milliseconds(), int64(50), "should have waited at least ~70ms (with tolerance)")
}
