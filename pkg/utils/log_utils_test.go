package utils

import (
	"bytes"
	"os"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
)

func TestPrintMessage(t *testing.T) {
	// Save stdout
	oldStdout := os.Stdout

	// Create a pipe
	r, w, _ := os.Pipe()
	os.Stdout = w

	message := "test message"
	PrintMessage(message)

	// Close the writer to get all output
	w.Close()

	// Read the output
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	// Restore stdout
	os.Stdout = oldStdout

	assert.Contains(t, buf.String(), message)
}

func TestLogLevels(t *testing.T) {
	tests := []struct {
		name     string
		logFunc  func(msg any, keyvals ...any)
		message  string
		expected string
	}{
		{
			name:     "LogTrace",
			logFunc:  log.Debug,
			message:  "trace message",
			expected: "trace message",
		},
		{
			name:     "LogDebug",
			logFunc:  log.Debug,
			message:  "debug message",
			expected: "debug message",
		},
		{
			name:     "LogInfo",
			logFunc:  log.Info,
			message:  "info message",
			expected: "info message",
		},
		{
			name:     "LogWarning",
			logFunc:  log.Warn,
			message:  "warning message",
			expected: "warning message",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var logBuffer bytes.Buffer
			oldLogger := log.Default()
			defer func() { log.SetDefault(oldLogger) }()

			customLogger := log.NewWithOptions(
				&logBuffer,
				log.Options{
					Level: log.DebugLevel,
				},
			)
			log.SetDefault(customLogger)

			tc.logFunc(tc.message)
			assert.Contains(t, logBuffer.String(), tc.expected)
		})
	}
}
