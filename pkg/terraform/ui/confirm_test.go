package ui

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestConfirmApply_NoTTY verifies that ConfirmApply short-circuits with
// ErrStreamingNotSupported when stdin/stdout are not a TTY, which is always
// the case under `go test`. This exercises the early-return guard without
// invoking the interactive huh prompt.
func TestConfirmApply_NoTTY(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "non-tty stdin and stdout under go test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confirmed, err := ConfirmApply()

			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrStreamingNotSupported)
			assert.False(t, confirmed, "confirmed must remain false when the TTY guard rejects the prompt")
		})
	}
}

// TestConfirmDestroy_NoTTY verifies that ConfirmDestroy short-circuits with
// ErrStreamingNotSupported when stdin/stdout are not a TTY, which is always
// the case under `go test`. This exercises the early-return guard without
// invoking the interactive huh prompt.
func TestConfirmDestroy_NoTTY(t *testing.T) {
	tests := []struct {
		name string
	}{
		{name: "non-tty stdin and stdout under go test"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confirmed, err := ConfirmDestroy()

			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrStreamingNotSupported)
			assert.False(t, confirmed, "confirmed must remain false when the TTY guard rejects the prompt")
		})
	}
}
