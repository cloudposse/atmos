package registry

import (
	"errors"
	"io"
	"net"
	"syscall"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// Maximum retry attempts for transient network failures.
	transientRetryMaxAttempts = 5
	// Initial backoff delay before the first retry.
	transientRetryInitialDelay = 1 * time.Second
	// Maximum backoff delay between retries.
	transientRetryMaxDelay = 10 * time.Second
)

// IsTransientNetworkError reports whether err is a transient network failure
// that is safe to retry by re-issuing the request — for example a connection
// reset by peer, a broken pipe, a truncated body read, or a network timeout.
//
// Detection is structural (via errors.Is/As against syscall and net errors),
// not string matching, so it works regardless of how the error was wrapped and
// across platforms. A reset can occur while reading a response body, so callers
// must retry the whole request (not just the round-trip) for it to recover.
func IsTransientNetworkError(err error) bool {
	defer perf.Track(nil, "registry.IsTransientNetworkError")()

	if err == nil {
		return false
	}

	// Network timeouts (e.g. i/o timeout) are transient.
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	// Connection resets, aborts, broken pipes, and refusals are transient.
	// These syscall errnos are defined on all supported platforms (on Windows
	// they map to the corresponding WSA* codes).
	for _, errno := range []syscall.Errno{
		syscall.ECONNRESET,
		syscall.ECONNABORTED,
		syscall.EPIPE,
		syscall.ECONNREFUSED,
		syscall.ETIMEDOUT,
	} {
		if errors.Is(err, errno) {
			return true
		}
	}

	// A truncated body read (server closed mid-stream) is transient.
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	return false
}

// TransientRetryConfig returns the retry configuration used for transient
// network failures: a bounded number of attempts with exponential backoff.
func TransientRetryConfig() *schema.RetryConfig {
	defer perf.Track(nil, "registry.TransientRetryConfig")()

	maxAttempts := transientRetryMaxAttempts
	initialDelay := transientRetryInitialDelay
	maxDelay := transientRetryMaxDelay
	return &schema.RetryConfig{
		MaxAttempts:     &maxAttempts,
		BackoffStrategy: schema.BackoffExponential,
		InitialDelay:    &initialDelay,
		MaxDelay:        &maxDelay,
	}
}
