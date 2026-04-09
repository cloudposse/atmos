package logger

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

// recordingLogger captures the level and message of each log call for assertion.
type recordingLogger struct {
	level string
	msg   string
}

func (r *recordingLogger) Error(msg string, _ ...interface{}) { r.level = "error"; r.msg = msg }
func (r *recordingLogger) Warn(msg string, _ ...interface{})  { r.level = "warn"; r.msg = msg }
func (r *recordingLogger) Info(msg string, _ ...interface{})  { r.level = "info"; r.msg = msg }
func (r *recordingLogger) Debug(msg string, _ ...interface{}) { r.level = "debug"; r.msg = msg }

func TestLogrusAdapter_Write(t *testing.T) {
	adapter := newLogrusAdapter()

	// Test writing a simple message.
	message := "test message"
	n, err := adapter.Write([]byte(message))
	assert.NoError(t, err)
	assert.Equal(t, len(message), n)

	// Test writing message with trailing newline (logrus adds this).
	messageWithNewline := "test message\n"
	n, err = adapter.Write([]byte(messageWithNewline))
	assert.NoError(t, err)
	assert.Equal(t, len(messageWithNewline), n)
}

// TestLogrusAdapter_Write_RoutesToCorrectLevel verifies that Write() parses
// the JSON level field and dispatches to the correct Atmos log level. Uses a
// recording logger injected into the adapter to capture the actual level each
// message was routed to.
func TestLogrusAdapter_Write_RoutesToCorrectLevel(t *testing.T) {
	tests := []struct {
		name      string
		message   string
		wantLevel string
		wantMsg   string
	}{
		{
			name:      "error level",
			message:   `{"level":"error","msg":"authentication failed","provider":"browser"}` + "\n",
			wantLevel: "error",
			wantMsg:   "authentication failed",
		},
		{
			name:      "fatal routes to error",
			message:   `{"level":"fatal","msg":"critical error"}` + "\n",
			wantLevel: "error",
			wantMsg:   "critical error",
		},
		{
			name:      "panic routes to error",
			message:   `{"level":"panic","msg":"panic occurred"}` + "\n",
			wantLevel: "error",
			wantMsg:   "panic occurred",
		},
		{
			name:      "warning level",
			message:   `{"level":"warning","msg":"retrying connection"}` + "\n",
			wantLevel: "warn",
			wantMsg:   "retrying connection",
		},
		{
			name:      "warn level (alternate spelling)",
			message:   `{"level":"warn","msg":"warning message"}` + "\n",
			wantLevel: "warn",
			wantMsg:   "warning message",
		},
		{
			name:      "info level",
			message:   `{"level":"info","msg":"authentication successful"}` + "\n",
			wantLevel: "info",
			wantMsg:   "authentication successful",
		},
		{
			name:      "debug level",
			message:   `{"level":"debug","msg":"processing request"}` + "\n",
			wantLevel: "debug",
			wantMsg:   "processing request",
		},
		{
			name:      "trace routes to debug",
			message:   `{"level":"trace","msg":"detailed trace"}` + "\n",
			wantLevel: "debug",
			wantMsg:   "detailed trace",
		},
		{
			name:      "missing level defaults to info",
			message:   `{"msg":"no level field"}` + "\n",
			wantLevel: "info",
			wantMsg:   "no level field",
		},
		{
			name:      "mixed case level normalized",
			message:   `{"level":"ERROR","msg":"mixed case error"}` + "\n",
			wantLevel: "error",
			wantMsg:   "mixed case error",
		},
		{
			name:      "non-JSON fallback routes to info",
			message:   "plain text log message\n",
			wantLevel: "info",
			wantMsg:   "plain text log message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &recordingLogger{}
			adapter := &logrusAdapter{logger: rec}

			n, err := adapter.Write([]byte(tt.message))
			assert.NoError(t, err)
			assert.Equal(t, len(tt.message), n)
			assert.Equal(t, tt.wantLevel, rec.level, "message routed to wrong level")
			assert.Equal(t, tt.wantMsg, rec.msg, "message text mismatch")
		})
	}
}

func TestConfigureLogrusForAtmos(t *testing.T) {
	// Store original logrus configuration to restore after test.
	originalFormatter := logrus.StandardLogger().Formatter
	originalLevel := logrus.GetLevel()
	originalAtmosLevel := GetLevel()
	defer func() {
		logrus.SetFormatter(originalFormatter)
		logrus.SetLevel(originalLevel)
		SetLevel(originalAtmosLevel)
	}()

	// Test with different Atmos log levels.
	tests := []struct {
		atmosLevel          Level
		expectedLogrusLevel logrus.Level
	}{
		{TraceLevel, logrus.DebugLevel},
		{DebugLevel, logrus.DebugLevel},
		{InfoLevel, logrus.InfoLevel},
		{WarnLevel, logrus.WarnLevel},
		{ErrorLevel, logrus.ErrorLevel},
		{FatalLevel, logrus.FatalLevel},
	}

	for _, tt := range tests {
		t.Run(LevelToString(tt.atmosLevel), func(t *testing.T) {
			// Set Atmos log level.
			SetLevel(tt.atmosLevel)

			// Configure logrus.
			ConfigureLogrusForAtmos()

			// Verify formatter is JSONFormatter with correct settings.
			formatter, ok := logrus.StandardLogger().Formatter.(*logrus.JSONFormatter)
			assert.True(t, ok, "Formatter should be JSONFormatter")
			if formatter != nil {
				assert.True(t, formatter.DisableTimestamp, "DisableTimestamp should be true")
			}

			// Verify level matches Atmos level.
			assert.Equal(t, tt.expectedLogrusLevel, logrus.GetLevel())
		})
	}
}

// Edge-case tests for Write() — covers nil, empty, and malformed inputs.
func TestLogrusAdapter_Write_EdgeCases(t *testing.T) {
	adapter := newLogrusAdapter()

	tests := []struct {
		name  string
		input []byte
	}{
		{
			name:  "empty byte slice",
			input: []byte{},
		},
		{
			name:  "nil byte slice",
			input: nil,
		},
		{
			name:  "whitespace only",
			input: []byte("   \n"),
		},
		{
			name:  "malformed JSON — truncated",
			input: []byte(`{"level":"error","msg":"trunc`),
		},
		{
			name:  "malformed JSON — bare brace",
			input: []byte("{"),
		},
		{
			name:  "JSON array instead of object",
			input: []byte(`["not","an","object"]` + "\n"),
		},
		{
			name:  "empty JSON object",
			input: []byte("{}\n"),
		},
		{
			name:  "JSON with missing msg field",
			input: []byte(`{"level":"error","details":"no msg key"}` + "\n"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write must never panic or return an error, regardless of input.
			n, err := adapter.Write(tt.input)
			assert.NoError(t, err, "Write must never return an error")
			assert.Equal(t, len(tt.input), n, "Write must return the input length")
		})
	}
}

func TestAtmosLevelToLogrus(t *testing.T) {
	tests := []struct {
		atmosLevel Level
		expected   logrus.Level
	}{
		{TraceLevel, logrus.DebugLevel},
		{DebugLevel, logrus.DebugLevel},
		{InfoLevel, logrus.InfoLevel},
		{WarnLevel, logrus.WarnLevel},
		{ErrorLevel, logrus.ErrorLevel},
		{FatalLevel, logrus.FatalLevel},
	}

	for _, tt := range tests {
		t.Run(LevelToString(tt.atmosLevel), func(t *testing.T) {
			result := atmosLevelToLogrus(tt.atmosLevel)
			assert.Equal(t, tt.expected, result)
		})
	}
}
