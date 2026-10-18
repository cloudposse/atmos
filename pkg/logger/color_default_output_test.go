package logger

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestColorDefaultOutput verifies nil destinations fall back to stderr and that
// restoring automatic detection without a configured destination keeps files plain.
func TestColorDefaultOutput(t *testing.T) {
	for _, key := range []string{"NO_COLOR", "CLICOLOR_FORCE", "FORCE_COLOR"} {
		t.Setenv(key, "")
	}
	path := filepath.Join(t.TempDir(), "stderr.log")
	stderr, err := os.Create(path)
	require.NoError(t, err)
	original := os.Stderr
	os.Stderr = stderr
	t.Cleanup(func() { os.Stderr = original; _ = stderr.Close() })
	logger := New()
	logger.SetColorEnabled(false, false)
	logger.SetColorEnabled(true, false)
	logger.Error("automatic detection restored")
	logger.SetOutput(nil)
	logger.SetColorEnabled(false, false)
	logger.Error("\x1b[31mplain record\x1b[0m")
	output, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(output), "automatic detection restored")
	assert.Contains(t, string(output), "plain record")
	assert.NotContains(t, string(output), "\x1b")
}
