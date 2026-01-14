package github

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestCheckRateLimitWithService_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := NewMockRateLimitService(ctrl)

	resetTime := time.Now().Add(time.Hour)
	rateLimits := &github.RateLimits{
		Core: &github.Rate{
			Remaining: 4500,
			Limit:     5000,
			Reset:     github.Timestamp{Time: resetTime},
		},
	}

	mockService.EXPECT().
		Get(gomock.Any()).
		Return(rateLimits, nil, nil)

	ctx := context.Background()
	status, err := CheckRateLimitWithService(ctx, mockService)

	assert.NoError(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, 4500, status.Remaining)
	assert.Equal(t, 5000, status.Limit)
	assert.Equal(t, resetTime, status.ResetAt)
}

func TestCheckRateLimitWithService_APIError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := NewMockRateLimitService(ctrl)

	mockService.EXPECT().
		Get(gomock.Any()).
		Return(nil, nil, errors.New("API error"))

	ctx := context.Background()
	status, err := CheckRateLimitWithService(ctx, mockService)

	assert.Error(t, err)
	assert.Nil(t, status)
	assert.Contains(t, err.Error(), "API error")
}

func TestCheckRateLimitWithService_NilLimits(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := NewMockRateLimitService(ctrl)

	mockService.EXPECT().
		Get(gomock.Any()).
		Return(nil, nil, nil)

	ctx := context.Background()
	status, err := CheckRateLimitWithService(ctx, mockService)

	assert.NoError(t, err)
	assert.Nil(t, status)
}

func TestCheckRateLimitWithService_NilCore(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := NewMockRateLimitService(ctrl)

	rateLimits := &github.RateLimits{
		Core: nil, // Nil Core.
	}

	mockService.EXPECT().
		Get(gomock.Any()).
		Return(rateLimits, nil, nil)

	ctx := context.Background()
	status, err := CheckRateLimitWithService(ctx, mockService)

	assert.NoError(t, err)
	assert.Nil(t, status)
}

func TestShouldWaitForRateLimitWithService_BelowThreshold(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := NewMockRateLimitService(ctrl)

	rateLimits := &github.RateLimits{
		Core: &github.Rate{
			Remaining: 3,
			Limit:     5000,
			Reset:     github.Timestamp{Time: time.Now().Add(time.Hour)},
		},
	}

	mockService.EXPECT().
		Get(gomock.Any()).
		Return(rateLimits, nil, nil)

	ctx := context.Background()
	shouldWait := ShouldWaitForRateLimitWithService(ctx, mockService, 5)

	assert.True(t, shouldWait)
}

func TestShouldWaitForRateLimitWithService_AboveThreshold(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := NewMockRateLimitService(ctrl)

	rateLimits := &github.RateLimits{
		Core: &github.Rate{
			Remaining: 100,
			Limit:     5000,
			Reset:     github.Timestamp{Time: time.Now().Add(time.Hour)},
		},
	}

	mockService.EXPECT().
		Get(gomock.Any()).
		Return(rateLimits, nil, nil)

	ctx := context.Background()
	shouldWait := ShouldWaitForRateLimitWithService(ctx, mockService, 5)

	assert.False(t, shouldWait)
}

func TestShouldWaitForRateLimitWithService_APIError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := NewMockRateLimitService(ctrl)

	mockService.EXPECT().
		Get(gomock.Any()).
		Return(nil, nil, errors.New("API error"))

	ctx := context.Background()
	shouldWait := ShouldWaitForRateLimitWithService(ctx, mockService, 5)

	// Should return false on API errors (don't block operations).
	assert.False(t, shouldWait)
}

func TestShouldWaitForRateLimitWithService_NilStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := NewMockRateLimitService(ctrl)

	mockService.EXPECT().
		Get(gomock.Any()).
		Return(nil, nil, nil)

	ctx := context.Background()
	shouldWait := ShouldWaitForRateLimitWithService(ctx, mockService, 5)

	// Should return false on nil status.
	assert.False(t, shouldWait)
}

func TestCheckRateLimitWithService_CancelledContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := NewMockRateLimitService(ctrl)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	mockService.EXPECT().
		Get(gomock.Any()).
		Return(nil, nil, context.Canceled)

	status, err := CheckRateLimitWithService(ctx, mockService)

	assert.Error(t, err)
	assert.Nil(t, status)
	assert.True(t, errors.Is(err, context.Canceled))
}

func TestRateLimitStatus(t *testing.T) {
	// Test the RateLimitStatus struct.
	resetTime := time.Now().Add(time.Hour)
	status := RateLimitStatus{
		Remaining: 50,
		Limit:     60,
		ResetAt:   resetTime,
	}

	assert.Equal(t, 50, status.Remaining)
	assert.Equal(t, 60, status.Limit)
	assert.Equal(t, resetTime, status.ResetAt)
}

func TestCheckRateLimitWithService_ZeroRemaining(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockService := NewMockRateLimitService(ctrl)

	resetTime := time.Now().Add(time.Hour)
	rateLimits := &github.RateLimits{
		Core: &github.Rate{
			Remaining: 0,
			Limit:     5000,
			Reset:     github.Timestamp{Time: resetTime},
		},
	}

	mockService.EXPECT().
		Get(gomock.Any()).
		Return(rateLimits, nil, nil)

	ctx := context.Background()
	status, err := CheckRateLimitWithService(ctx, mockService)

	assert.NoError(t, err)
	assert.NotNil(t, status)
	assert.Equal(t, 0, status.Remaining)
	assert.Equal(t, 5000, status.Limit)
}

// Integration tests (require network, skip in short mode).

func TestCheckRateLimit_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping rate limit API test in short mode")
	}

	ctx := context.Background()
	status, err := CheckRateLimit(ctx)
	// The function may return nil, nil if the API call fails (e.g., rate limited).
	// This is by design to avoid blocking operations.
	if err != nil {
		t.Logf("Rate limit check returned error (expected in some environments): %v", err)
		return
	}

	if status == nil {
		t.Log("Rate limit check returned nil status (expected in some environments)")
		return
	}

	// If we got a status, validate it.
	assert.GreaterOrEqual(t, status.Limit, 0, "Limit should be non-negative")
	assert.GreaterOrEqual(t, status.Remaining, 0, "Remaining should be non-negative")
	assert.False(t, status.ResetAt.IsZero(), "ResetAt should not be zero")

	t.Logf("Rate limit status: %d/%d remaining, resets at %s",
		status.Remaining, status.Limit, status.ResetAt)
}

func TestWaitForRateLimit_SufficientRemaining_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping rate limit API test in short mode")
	}

	ctx := context.Background()

	// With a very low threshold, we should not need to wait.
	err := WaitForRateLimit(ctx, 0)
	assert.NoError(t, err)
}

func TestShouldWaitForRateLimit_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping rate limit API test in short mode")
	}

	ctx := context.Background()

	// With threshold of 0, should never need to wait.
	shouldWait := ShouldWaitForRateLimit(ctx, 0)
	assert.False(t, shouldWait)
}
