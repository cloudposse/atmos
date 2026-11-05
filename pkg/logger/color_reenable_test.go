package logger

import (
	"bytes"
	"testing"

	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSetColorEnabledRestoresAutomaticDetection verifies that re-enabling color after an
// opt-out brings color back even when no explicit profile was ever set.
func TestSetColorEnabledRestoresAutomaticDetection(t *testing.T) {
	// Non-TTY writers only report color through CLICOLOR_FORCE, which makes detection deterministic.
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")

	tests := []struct {
		name      string
		firstOpt  [2]bool // enabled, disabled passed to the first SetColorEnabled call.
		secondOpt [2]bool // enabled, disabled passed to the second SetColorEnabled call.
		wantFirst bool
		wantAgain bool
	}{
		{"global veto then enable", [2]bool{true, true}, [2]bool{true, false}, false, true},
		{"logs.color false then enable", [2]bool{false, false}, [2]bool{true, false}, false, true},
		{"enabled stays colored", [2]bool{true, false}, [2]bool{true, false}, true, true},
		{"still vetoed stays plain", [2]bool{true, true}, [2]bool{true, true}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := New()
			logger.SetLevel(DebugLevel)
			var output bytes.Buffer
			logger.SetOutput(&output)

			logger.SetColorEnabled(tt.firstOpt[0], tt.firstOpt[1])
			logger.Debug("first")
			require.Contains(t, output.String(), "first")
			assert.Equal(t, tt.wantFirst, bytes.Contains(output.Bytes(), []byte("\x1b")))

			output.Reset()
			logger.SetColorEnabled(tt.secondOpt[0], tt.secondOpt[1])
			logger.Debug("second")
			require.Contains(t, output.String(), "second")
			assert.Equal(t, tt.wantAgain, bytes.Contains(output.Bytes(), []byte("\x1b")))
		})
	}
}

// TestSetColorEnabledReenableWithoutForcedColorStaysPlain is the negative path: restoring
// automatic detection must not invent color on a writer that does not support it.
func TestSetColorEnabledReenableWithoutForcedColorStaysPlain(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "")
	t.Setenv("FORCE_COLOR", "")

	logger := New()
	logger.SetLevel(DebugLevel)
	var output bytes.Buffer
	logger.SetOutput(&output)

	logger.SetColorEnabled(true, true)
	logger.SetColorEnabled(true, false)
	logger.Debug("plain")
	require.Contains(t, output.String(), "plain")
	assert.NotContains(t, output.String(), "\x1b")
}

// TestSetColorEnabledReenableKeepsExplicitProfile ensures an explicit profile still wins.
func TestSetColorEnabledReenableKeepsExplicitProfile(t *testing.T) {
	logger := New()
	logger.SetLevel(DebugLevel)
	var output bytes.Buffer
	logger.SetOutput(&output)
	logger.SetColorProfile(termenv.TrueColor)

	logger.SetColorEnabled(true, true)
	logger.SetColorEnabled(true, false)
	logger.Debug("explicit")
	assert.Contains(t, output.String(), "\x1b")
}
