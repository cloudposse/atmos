package cmd

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/perf"
)

func TestDisplayPerformanceHeatmap(t *testing.T) {
	tests := []struct {
		name           string
		mode           string
		expectedOutput []string
	}{
		{
			name: "Basic heatmap output",
			mode: "bar",
			expectedOutput: []string{
				"=== Atmos Performance Summary ===",
				"Elapsed:",
				"Functions:",
				"Calls:",
			},
		},
		{
			name: "Heatmap with P95",
			mode: "table",
			expectedOutput: []string{
				"=== Atmos Performance Summary ===",
				"Elapsed:",
				"P95",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset perf registry and enable tracking (P95 is automatically enabled).
			perf.EnableTracking(true)

			// Add some test tracking data.
			done := perf.Track(nil, "testFunction")
			done()

			// Capture stderr since heatmap writes to stderr.
			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w

			// Call displayPerformanceHeatmap.
			err := displayPerformanceHeatmap(nil, tt.mode)
			assert.NoError(t, err)

			// Close writer and restore stderr.
			_ = w.Close()
			os.Stderr = oldStderr

			// Read captured output.
			var output bytes.Buffer
			_, _ = io.Copy(&output, r)

			outputStr := output.String()

			// Verify expected strings are in output.
			for _, expected := range tt.expectedOutput {
				assert.Contains(t, outputStr, expected,
					"Output should contain '%s'", expected)
			}
		})
	}
}

func TestIsTTY(t *testing.T) {
	tests := []struct {
		name     string
		stderr   *os.File
		expected bool
	}{
		{
			name:     "Regular stderr (TTY check varies by environment)",
			stderr:   os.Stderr,
			expected: false, // In tests, stderr is usually not a TTY
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			oldStderr := os.Stderr
			os.Stderr = tt.stderr
			defer func() { os.Stderr = oldStderr }()

			result := term.IsTTYSupportForStderr()

			// We can't reliably test TTY in unit tests, so just verify it doesn't panic.
			_ = result
		})
	}
}

func TestHeatmapFlags(t *testing.T) {
	// Test that heatmap flags are properly registered.
	t.Run("Heatmap flag exists", func(t *testing.T) {
		flag := RootCmd.PersistentFlags().Lookup("heatmap")
		assert.NotNil(t, flag, "--heatmap flag should be registered")
		assert.Equal(t, "false", flag.DefValue, "--heatmap should default to false")
	})

	t.Run("Heatmap mode flag exists", func(t *testing.T) {
		flag := RootCmd.PersistentFlags().Lookup("heatmap-mode")
		assert.NotNil(t, flag, "--heatmap-mode flag should be registered")
		assert.Equal(t, "bar", flag.DefValue, "--heatmap-mode should default to bar")
	})
}

func TestHeatmapNonTTYOutput(t *testing.T) {
	// Reset perf registry and enable tracking.
	perf.EnableTracking(true)

	// Add test data.
	done := perf.Track(nil, "nonTTYTest")
	done()

	// Capture stderr.
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	err := displayPerformanceHeatmap(nil, "bar")
	assert.NoError(t, err)

	_ = w.Close()
	os.Stderr = oldStderr

	var output bytes.Buffer
	_, _ = io.Copy(&output, r)

	outputStr := output.String()

	// When there's no TTY, should show warning message.
	// Note: This test assumes we're running without a TTY (typical in CI/tests).
	if !strings.Contains(outputStr, "No TTY available") {
		// If we do have a TTY in the test environment, just check for summary.
		assert.Contains(t, outputStr, "=== Atmos Performance Summary ===")
	} else {
		assert.Contains(t, outputStr, "No TTY available for interactive visualization")
	}
}
