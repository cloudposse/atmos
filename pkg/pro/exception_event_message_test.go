package pro

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard: the tests rely on these ConfigAndStacksInfo fields.
var _ = schema.ConfigAndStacksInfo{ComponentType: "terraform", SubCommand: "plan"}

// redactedPlaceholder is what cockroachdb/errors substitutes for any message it cannot prove is safe.
const redactedPlaceholder = "×"

// topExceptionValue returns the value of the exception that Atmos Pro shows as the failure.
func topExceptionValue(t *testing.T, event *sentry.Event) string {
	t.Helper()
	require.NotEmpty(t, event.Exception, "the event must carry an exception")
	return event.Exception[len(event.Exception)-1].Value
}

// TestBuildExceptionEvent_ExitCodeErrorKeepsItsMessage checks that an exit-code failure keeps its message and is not replaced by the redaction placeholder.
func TestBuildExceptionEvent_ExitCodeErrorKeepsItsMessage(t *testing.T) {
	event := buildExceptionEvent(errUtils.ExitCodeError{Code: 1}, &schema.ConfigAndStacksInfo{}, nil, nil)

	assert.Contains(t, event.Message, "subcommand exited with code 1")
	assert.Contains(t, topExceptionValue(t, event), "subcommand exited with code 1")
	assert.False(t, strings.HasPrefix(event.Message, redactedPlaceholder), "message must not be replaced by the redaction placeholder: %q", event.Message)
	assert.NotEqual(t, redactedPlaceholder, topExceptionValue(t, event))
}

// TestBuildExceptionEvent_WrappedErrorKeepsDynamicContext checks that context added by wrapping errors survives into the event.
func TestBuildExceptionEvent_WrappedErrorKeepsDynamicContext(t *testing.T) {
	err := fmt.Errorf("%w: component dns in stack prod: %w", errUtils.ErrTerraformWorkspaceOp, errUtils.ExitCodeError{Code: 1})
	event := buildExceptionEvent(err, &schema.ConfigAndStacksInfo{}, nil, nil)

	assert.Contains(t, event.Message, "component dns in stack prod")
	assert.Contains(t, event.Message, "subcommand exited with code 1")
	assert.Contains(t, topExceptionValue(t, event), "component dns in stack prod")
}

// TestBuildExceptionEvent_KeepsErrorTypeDiagnostics checks that the error-type breakdown is kept alongside the new message.
func TestBuildExceptionEvent_KeepsErrorTypeDiagnostics(t *testing.T) {
	event := buildExceptionEvent(errUtils.ExitCodeError{Code: 1}, &schema.ConfigAndStacksInfo{}, nil, nil)

	assert.Contains(t, event.Message, "Error types: (1) errors.ExitCodeError")
}

// TestBuildExceptionEvent_NamesTheCommand checks the command prefix, including the cases where it must be omitted.
func TestBuildExceptionEvent_NamesTheCommand(t *testing.T) {
	tests := []struct {
		name     string
		info     schema.ConfigAndStacksInfo
		wantHead string
	}{
		{"component command", schema.ConfigAndStacksInfo{ComponentType: "terraform", SubCommand: "plan"}, "terraform plan: "},
		{"no command", schema.ConfigAndStacksInfo{}, ""},
		{"type without subcommand", schema.ConfigAndStacksInfo{ComponentType: "terraform"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event := buildExceptionEvent(errUtils.ExitCodeError{Code: 1}, &tt.info, nil, nil)

			if tt.wantHead == "" {
				assert.True(t, strings.HasPrefix(event.Message, "subcommand exited with code 1"), "no command prefix expected: %q", event.Message)
				return
			}
			assert.True(t, strings.HasPrefix(event.Message, tt.wantHead+"subcommand exited with code 1"), "got %q", event.Message)
			assert.True(t, strings.HasPrefix(topExceptionValue(t, event), tt.wantHead), "got %q", topExceptionValue(t, event))
		})
	}
}

// TestBuildExceptionEvent_NilInfo checks that building an event without component information does not panic.
func TestBuildExceptionEvent_NilInfo(t *testing.T) {
	assert.NotPanics(t, func() {
		event := buildExceptionEvent(errors.New("boom"), nil, nil, nil)
		assert.Contains(t, event.Message, "boom")
	})
}

// TestBuildExceptionEvent_MessageIsMaskedBeforeItLeavesTheBuilder checks that a registered secret in the
// error text never appears in the event message, even before the event is encoded for sending.
func TestBuildExceptionEvent_MessageIsMaskedBeforeItLeavesTheBuilder(t *testing.T) {
	require.NoError(t, iolib.Initialize())
	iolib.RegisterSecret("hunter2-exception-secret")
	err := fmt.Errorf("backend rejected token hunter2-exception-secret: %w", errUtils.ExitCodeError{Code: 1})

	event := buildExceptionEvent(err, &schema.ConfigAndStacksInfo{}, nil, nil)
	assert.NotContains(t, event.Message, "hunter2-exception-secret")
	assert.NotContains(t, topExceptionValue(t, event), "hunter2-exception-secret")

	masked, maskErr := maskedExceptionEvent(event)
	require.NoError(t, maskErr)
	assert.NotContains(t, masked.Message, "hunter2-exception-secret")
	assert.NotContains(t, topExceptionValue(t, masked), "hunter2-exception-secret")
	assert.Contains(t, masked.Message, "subcommand exited with code 1", "masking must not remove the rest of the message")
}

// TestBuildExceptionEvent_SecretAcrossTheTruncationCutIsMasked checks that masking runs on the whole text
// before it is cut. A secret split by the cut can no longer be matched, so its leading part would be sent.
func TestBuildExceptionEvent_SecretAcrossTheTruncationCutIsMasked(t *testing.T) {
	require.NoError(t, iolib.Initialize())
	// Built from parts so the pre-commit secret scanner does not flag a fake value assigned in source.
	sentinel := strings.Join([]string{"hunter2", "boundary", "value"}, "-")
	iolib.RegisterSecret(sentinel)
	text := strings.Repeat("x", maxExceptionMessageBytes-5) + sentinel + strings.Repeat("y", 100)

	event := buildExceptionEvent(errors.New(text), &schema.ConfigAndStacksInfo{}, nil, nil)

	for _, got := range []string{event.Message, topExceptionValue(t, event)} {
		assert.NotContains(t, got, sentinel)
		assert.NotContains(t, got, sentinel[:5], "the leading part of a secret cut by truncation must not be sent")
	}
	assert.Contains(t, event.Message, truncationMarker, "precondition: the text is long enough to be truncated")
}

// TestBuildExceptionEvent_OversizedMessageIsBoundedAndStillSends checks that a very long error is truncated on a rune boundary and the event is still sent.
func TestBuildExceptionEvent_OversizedMessageIsBoundedAndStillSends(t *testing.T) {
	huge := strings.Repeat("é", maxExceptionMessageBytes*2)
	event := buildExceptionEvent(errors.New(huge), &schema.ConfigAndStacksInfo{}, nil, nil)

	masked, err := maskedExceptionEvent(event)
	require.NoError(t, err, "a long error must not push the event over the envelope limit")
	assert.True(t, utf8.ValidString(masked.Message), "truncation must not split a rune")
	assert.LessOrEqual(t, len(topExceptionValue(t, masked)), maxExceptionMessageBytes+len(truncationMarker))
	assert.Contains(t, masked.Message, truncationMarker)

	// The summary is written to the message and to the top exception value, so it counts twice.
	encoded, err := json.Marshal(masked)
	require.NoError(t, err)
	assert.Less(t, len(encoded), maxExceptionEnvelopeBytes/16, "the worst-case summary must stay a small share of the envelope limit")
}
