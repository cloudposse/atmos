package registry

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsTransientNetworkError(t *testing.T) {
	// Mirrors the real "connection reset by peer" error chain produced by the
	// net/http stack while reading a response body.
	connReset := &net.OpError{Op: "read", Net: "tcp", Err: os.NewSyscallError("read", syscall.ECONNRESET)}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"connection reset by peer", connReset, true},
		{"wrapped connection reset", fmt.Errorf("HTTP request failed: failed to read response body: %w", connReset), true},
		{"broken pipe", os.NewSyscallError("write", syscall.EPIPE), true},
		{"connection refused", os.NewSyscallError("dial", syscall.ECONNREFUSED), true},
		{"unexpected EOF", io.ErrUnexpectedEOF, true},
		{"network timeout", &net.DNSError{IsTimeout: true}, true},
		{"clean EOF is not transient", io.EOF, false},
		{"plain error is not transient", errors.New("boom"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsTransientNetworkError(tt.err))
		})
	}
}

func TestTransientRetryConfig(t *testing.T) {
	c := TransientRetryConfig()
	require.NotNil(t, c)
	require.NotNil(t, c.MaxAttempts)
	assert.Positive(t, *c.MaxAttempts)
	require.NotNil(t, c.InitialDelay)
	require.NotNil(t, c.MaxDelay)
}
