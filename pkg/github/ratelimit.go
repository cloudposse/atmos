package github

import (
	"context"
	"fmt"
	"time"

	"github.com/google/go-github/v59/github"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

// RateLimitStatus contains information about GitHub API rate limits.
type RateLimitStatus struct {
	// Remaining is the number of requests remaining in the current rate limit window.
	Remaining int
	// Limit is the maximum number of requests allowed in the rate limit window.
	Limit int
	// ResetAt is when the rate limit will reset.
	ResetAt time.Time
}

// RateLimitService defines the interface for rate limit operations.
// This allows for mocking in tests.
//
//go:generate go run go.uber.org/mock/mockgen@latest -source=ratelimit.go -destination=mock_ratelimit_test.go -package=github
type RateLimitService interface {
	Get(ctx context.Context) (*github.RateLimits, *github.Response, error)
}

// rateLimitChecker wraps the rate limit checking logic.
type rateLimitChecker struct {
	service RateLimitService
}

// defaultRateLimitChecker returns a checker using the real GitHub client.
func defaultRateLimitChecker(ctx context.Context) *rateLimitChecker {
	client := newGitHubClient(ctx)
	return &rateLimitChecker{service: client.RateLimit}
}

// checkRateLimit performs the actual rate limit check.
func (c *rateLimitChecker) checkRateLimit(ctx context.Context) (*RateLimitStatus, error) {
	limits, _, err := c.service.Get(ctx)
	if err != nil {
		log.Debug("Failed to check GitHub rate limit", "error", err)
		return nil, err
	}

	if limits == nil || limits.Core == nil {
		log.Debug("GitHub rate limit response missing core limits")
		return nil, nil
	}

	return &RateLimitStatus{
		Remaining: limits.Core.Remaining,
		Limit:     limits.Core.Limit,
		ResetAt:   limits.Core.Reset.Time,
	}, nil
}

// CheckRateLimit queries the GitHub API for current rate limit status.
// Returns nil status (not error) if the check fails, to avoid blocking operations.
func CheckRateLimit(ctx context.Context) (*RateLimitStatus, error) {
	defer perf.Track(nil, "github.CheckRateLimit")()

	checker := defaultRateLimitChecker(ctx)
	return checker.checkRateLimit(ctx)
}

// CheckRateLimitWithService queries the GitHub API using a custom service.
// This is primarily used for testing with mock services.
func CheckRateLimitWithService(ctx context.Context, service RateLimitService) (*RateLimitStatus, error) {
	defer perf.Track(nil, "github.CheckRateLimitWithService")()

	checker := &rateLimitChecker{service: service}
	return checker.checkRateLimit(ctx)
}

// WaitForRateLimit checks GitHub rate limits and waits if necessary.
// If remaining requests are below minRemaining, it waits until the rate limit resets.
// Uses a spinner UI in TTY mode, otherwise simple output.
// Returns nil on success or context cancellation error.
// Does not return error on rate limit check failures (to avoid blocking operations).
func WaitForRateLimit(ctx context.Context, minRemaining int) error {
	defer perf.Track(nil, "github.WaitForRateLimit")()

	status, err := CheckRateLimit(ctx)
	if err != nil {
		// Don't block on rate limit check failures.
		log.Debug("Skipping rate limit wait due to check failure", "error", err)
		return nil
	}

	if status == nil {
		return nil
	}

	if status.Remaining >= minRemaining {
		log.Debug("GitHub rate limit check passed",
			"remaining", status.Remaining,
			"limit", status.Limit,
			"reset_at", status.ResetAt)
		return nil
	}

	waitDuration := time.Until(status.ResetAt)
	if waitDuration <= 0 {
		// Rate limit should have already reset.
		return nil
	}

	log.Warn("GitHub rate limit low, waiting for reset",
		"remaining", status.Remaining,
		"limit", status.Limit,
		"wait_duration", waitDuration.Round(time.Second))

	progressMsg := fmt.Sprintf("GitHub rate limit low (%d remaining), waiting %s for reset",
		status.Remaining, waitDuration.Round(time.Second))
	completedMsg := "GitHub rate limit reset, continuing"

	return spinner.ExecWithSpinner(progressMsg, completedMsg, func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(waitDuration):
			return nil
		}
	})
}

// ShouldWaitForRateLimit checks if rate limit is low enough to warrant waiting.
// This is a non-blocking check that just returns the decision.
func ShouldWaitForRateLimit(ctx context.Context, minRemaining int) bool {
	defer perf.Track(nil, "github.ShouldWaitForRateLimit")()

	status, err := CheckRateLimit(ctx)
	if err != nil || status == nil {
		return false
	}

	return status.Remaining < minRemaining
}

// ShouldWaitForRateLimitWithService checks if rate limit is low using a custom service.
// This is primarily used for testing with mock services.
func ShouldWaitForRateLimitWithService(ctx context.Context, service RateLimitService, minRemaining int) bool {
	defer perf.Track(nil, "github.ShouldWaitForRateLimitWithService")()

	status, err := CheckRateLimitWithService(ctx, service)
	if err != nil || status == nil {
		return false
	}

	return status.Remaining < minRemaining
}
