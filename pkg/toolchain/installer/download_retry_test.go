package installer

import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestIsRetryableDownloadError verifies that transient network failures while
// downloading (including a connection reset by peer during the response-body
// read) are classified as retryable, while terminal errors are not.
func TestIsRetryableDownloadError(t *testing.T) {
	connReset := &net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.ECONNRESET)}

	// The exact shape produced by writeResponseToCache for a body-read reset.
	markedReset := errors.Join(
		errUtils.ErrDownloadRetryable,
		fmt.Errorf("%w: failed to read response body: %w", ErrHTTPRequest, connReset),
	)
	// The same error without the explicit retryable marker — must still be
	// caught structurally.
	unmarkedReset := fmt.Errorf("%w: failed to read response body: %w", ErrHTTPRequest, connReset)

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"404 is terminal", ErrHTTP404, false},
		{"explicitly retryable", errUtils.ErrDownloadRetryable, true},
		{"body-read reset (marked)", markedReset, true},
		{"body-read reset (structural)", unmarkedReset, true},
		{"non-transient error", errors.New("checksum mismatch"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isRetryableDownloadError(tt.err))
		})
	}
}
