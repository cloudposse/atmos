package pro

import (
	"context"
	"errors"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// DefaultMaxRetries is the maximum number of retry attempts for upload operations.
	// Total attempts = 1 (initial) + DefaultMaxRetries.
	DefaultMaxRetries = 3
	// DefaultBaseDelay is the initial backoff delay before the first retry.
	DefaultBaseDelay = 1 * time.Second

	logKeyMaxRetries = "max_retries"
	logKeyAttempt    = "attempt"
)

// tokenRefresher abstracts token refresh for testability.
type tokenRefresher interface {
	RefreshToken() error
}

// retryConfig holds configuration for the retry helper.
// It is translated into a schema.RetryConfig for the generic retry package.
type retryConfig struct {
	maxRetries int
	baseDelay  time.Duration
}

// defaultRetryConfig returns the default retry configuration.
func defaultRetryConfig() retryConfig {
	return retryConfig{
		maxRetries: DefaultMaxRetries,
		baseDelay:  DefaultBaseDelay,
	}
}

// toSchemaConfig converts the pro-specific retryConfig to a schema.RetryConfig
// suitable for the generic retry package.
func (c retryConfig) toSchemaConfig() schema.RetryConfig {
	// Total attempts = 1 (initial) + maxRetries.
	maxAttempts := c.maxRetries + 1
	return schema.RetryConfig{
		MaxAttempts:     &maxAttempts,
		BackoffStrategy: schema.BackoffExponential,
		InitialDelay:    &c.baseDelay,
		Multiplier:      float64Ptr(2.0),
	}
}

func float64Ptr(f float64) *float64 { return &f }

// doWithRetry executes fn with retry logic for transient failures.
// On 401 errors, it calls refresher.RefreshToken() before retrying.
// On 5xx or network errors, it retries with exponential backoff.
// On 400/403/404, it returns immediately without retrying.
func doWithRetry(operation string, fn func() error, refresher tokenRefresher, cfg retryConfig) error {
	attempt := 0
	schemaCfg := cfg.toSchemaConfig()

	// refreshErr captures a token refresh failure so it can be joined into the
	// returned error even though the predicate can only signal "don't retry".
	var refreshErr error

	err := retry.WithPredicate(context.Background(), &schemaCfg, func() error {
		attempt++
		return fn()
	}, func(err error) bool {
		shouldRetry, refErr := classifyError(operation, err, attempt, cfg, refresher)
		if refErr != nil {
			refreshErr = refErr
		}
		return shouldRetry
	})
	if err != nil {
		// Attach refresh failure if one occurred.
		if refreshErr != nil {
			return errors.Join(err, refreshErr)
		}

		// Wrap "max attempts exceeded" from the generic retry package with our
		// domain-specific sentinel so callers can errors.Is for it.
		if attempt > cfg.maxRetries {
			log.Error(
				"Upload failed after all retries.",
				logKeyOperation, operation,
				logKeyMaxRetries, cfg.maxRetries,
				"error", err,
			)
			return wrapErr(errUtils.ErrUploadRetryExhausted, err)
		}
	}

	return err
}

// classifyError determines whether an error is retryable and performs side effects
// (token refresh, logging). Returns (shouldRetry, refreshErr).
func classifyError(operation string, lastErr error, attempt int, cfg retryConfig, refresher tokenRefresher) (bool, error) {
	var apiErr *APIError
	if !errors.As(lastErr, &apiErr) {
		// Deterministic local errors (e.g. bad URL) — do not retry.
		if errors.Is(lastErr, errUtils.ErrFailedToCreateAuthRequest) {
			return false, nil
		}

		// Remaining non-API errors are likely transient network issues — retry.
		log.Warn(
			"Upload failed with network error, retrying.",
			logKeyOperation, operation,
			logKeyAttempt, attempt,
			logKeyMaxRetries, cfg.maxRetries,
			"error", lastErr,
		)
		return true, nil
	}

	if !apiErr.IsRetryable() {
		// 400, 403, 404 — non-retryable, return immediately.
		return false, nil
	}

	if apiErr.IsAuthError() {
		log.Warn(
			"Upload received 401, refreshing token before retry.",
			logKeyOperation, operation,
			logKeyAttempt, attempt,
			logKeyMaxRetries, cfg.maxRetries,
		)

		if refreshErr := refresher.RefreshToken(); refreshErr != nil {
			log.Error(
				"Token refresh failed, aborting retries.",
				logKeyOperation, operation,
				"error", refreshErr,
			)
			return false, refreshErr
		}
		return true, nil
	}

	// 5xx — retry without token refresh.
	log.Warn(
		"Upload received server error, retrying.",
		logKeyOperation, operation,
		logKeyAttempt, attempt,
		logKeyMaxRetries, cfg.maxRetries,
		logKeyStatus, apiErr.StatusCode,
	)

	return true, nil
}
