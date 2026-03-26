package pro

import (
	"errors"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
)

const (
	// DefaultMaxRetries is the maximum number of retry attempts for upload operations.
	DefaultMaxRetries = 3
	// DefaultBaseDelay is the initial backoff delay before the first retry.
	DefaultBaseDelay = 1 * time.Second

	logKeyMaxRetries = "max_retries"
	logKeyAttempt    = "attempt"
)

// sleeper abstracts time.Sleep for testability.
type sleeper interface {
	Sleep(d time.Duration)
}

// realSleeper uses time.Sleep.
type realSleeper struct{}

func (realSleeper) Sleep(d time.Duration) { time.Sleep(d) }

// retryConfig holds configuration for the retry helper.
type retryConfig struct {
	maxRetries int
	baseDelay  time.Duration
	sleeper    sleeper
}

// defaultRetryConfig returns the default retry configuration.
func defaultRetryConfig() retryConfig {
	return retryConfig{
		maxRetries: DefaultMaxRetries,
		baseDelay:  DefaultBaseDelay,
		sleeper:    realSleeper{},
	}
}

// tokenRefresher abstracts token refresh for testability.
type tokenRefresher interface {
	RefreshToken() error
}

// doWithRetry executes fn with retry logic for transient failures.
// On 401 errors, it calls refresher.RefreshToken() before retrying.
// On 5xx or network errors, it retries with exponential backoff.
// On 400/403/404, it returns immediately without retrying.
func doWithRetry(operation string, fn func() error, refresher tokenRefresher, cfg retryConfig) error {
	var lastErr error

	for attempt := 0; attempt <= cfg.maxRetries; attempt++ {
		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		// Check if this is the last attempt — don't bother classifying.
		if attempt == cfg.maxRetries {
			break
		}

		if shouldAbort := handleRetryableError(operation, lastErr, attempt, cfg, refresher); shouldAbort != nil {
			return shouldAbort
		}

		// Exponential backoff: baseDelay * 2^attempt.
		delay := cfg.baseDelay * (1 << uint(attempt))
		cfg.sleeper.Sleep(delay)
	}

	log.Error("Upload failed after all retries.",
		logKeyOperation, operation,
		logKeyMaxRetries, cfg.maxRetries,
		"error", lastErr,
	)

	return errors.Join(errUtils.ErrUploadRetryExhausted, lastErr)
}

// handleRetryableError inspects the error and logs the retry attempt.
// Returns non-nil to signal the caller should abort (non-retryable or refresh failure).
// Returns nil to signal the caller should proceed with the retry.
func handleRetryableError(operation string, lastErr error, attempt int, cfg retryConfig, refresher tokenRefresher) error {
	var apiErr *APIError
	if !errors.As(lastErr, &apiErr) {
		// Deterministic local errors (e.g. bad URL) — do not retry.
		if errors.Is(lastErr, errUtils.ErrFailedToCreateAuthRequest) {
			return lastErr
		}

		// Remaining non-API errors are likely transient network issues — retry.
		log.Warn("Upload failed with network error, retrying.",
			logKeyOperation, operation,
			logKeyAttempt, attempt+1,
			logKeyMaxRetries, cfg.maxRetries,
			"error", lastErr,
		)
		return nil
	}

	if !apiErr.IsRetryable() {
		// 400, 403, 404 — non-retryable, return immediately.
		return lastErr
	}

	if apiErr.IsAuthError() {
		return handleAuthRetry(operation, lastErr, attempt, cfg, refresher)
	}

	// 5xx — retry without token refresh.
	log.Warn("Upload received server error, retrying.",
		logKeyOperation, operation,
		logKeyAttempt, attempt+1,
		logKeyMaxRetries, cfg.maxRetries,
		logKeyStatus, apiErr.StatusCode,
	)

	return nil
}

// handleAuthRetry refreshes the token on 401 errors before retrying.
// Returns non-nil to abort if token refresh fails.
func handleAuthRetry(operation string, lastErr error, attempt int, cfg retryConfig, refresher tokenRefresher) error {
	log.Warn("Upload received 401, refreshing token before retry.",
		logKeyOperation, operation,
		logKeyAttempt, attempt+1,
		logKeyMaxRetries, cfg.maxRetries,
	)

	if refreshErr := refresher.RefreshToken(); refreshErr != nil {
		log.Error("Token refresh failed, aborting retries.",
			logKeyOperation, operation,
			"error", refreshErr,
		)
		return errors.Join(lastErr, refreshErr)
	}

	return nil
}
