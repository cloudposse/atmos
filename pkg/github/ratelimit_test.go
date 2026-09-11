package github

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/go-github/v59/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/tests/testhelpers/httpmock"
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

// The three tests below used to be live-network "_Integration" tests gated on
// testing.Short(), hitting the real github.com rate_limit endpoint through CheckRateLimit's
// default client (newGitHubClient -> RepoEndpoints). They now point RepoEndpoints at the
// httpmock GitHub facade via GITHUB_SERVER_URL/GITHUB_API_URL (t.Setenv), so they exercise the
// exact same production code path deterministically, in-process, on every PR, with no live
// GitHub dependency.

func TestCheckRateLimit_ViaMock(t *testing.T) {
	mock := httpmock.NewGitHubMockServer(t)
	mock.Setenv(t)
	resetAt := time.Now().Add(time.Hour).Truncate(time.Second)
	mock.SetRateLimit(4321, resetAt)

	ctx := context.Background()
	status, err := CheckRateLimit(ctx)

	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, 4321, status.Remaining)
	assert.Equal(t, 5000, status.Limit)
	assert.Equal(t, resetAt.Unix(), status.ResetAt.Unix())
}

func TestWaitForRateLimit_SufficientRemaining_ViaMock(t *testing.T) {
	mock := httpmock.NewGitHubMockServer(t)
	mock.Setenv(t)
	mock.SetRateLimit(5000, time.Now().Add(time.Hour))

	ctx := context.Background()

	// With a very low threshold, we should not need to wait.
	err := WaitForRateLimit(ctx, 0)
	assert.NoError(t, err)
}

func TestShouldWaitForRateLimit_ViaMock(t *testing.T) {
	mock := httpmock.NewGitHubMockServer(t)
	mock.Setenv(t)
	mock.SetRateLimit(5000, time.Now().Add(time.Hour))

	ctx := context.Background()

	// With threshold of 0, should never need to wait.
	shouldWait := ShouldWaitForRateLimit(ctx, 0)
	assert.False(t, shouldWait)
}

// TestWaitForRateLimit_ExhaustedButAlreadyReset_ViaMock asserts the current behavior of
// waitForRateLimitImpl (pkg/github/ratelimit.go) when remaining is 0 but the reset time has
// already passed: it returns immediately without blocking, rather than computing a negative
// wait duration.
func TestWaitForRateLimit_ExhaustedButAlreadyReset_ViaMock(t *testing.T) {
	mock := httpmock.NewGitHubMockServer(t)
	mock.Setenv(t)
	mock.SetRateLimit(0, time.Now().Add(-time.Minute))

	start := time.Now()
	err := WaitForRateLimit(context.Background(), 5)
	elapsed := time.Since(start)

	require.NoError(t, err)
	assert.Less(t, elapsed, 5*time.Second, "an already-passed reset must not block")
}
