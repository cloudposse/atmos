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

func TestSanitizeLogMessage(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "password in key=value",
			input: "login failed password=s3cret123",
			want:  "login failed password=[REDACTED]",
		},
		{
			name:  "token with colon separator",
			input: "auth token: abc123def",
			want:  "auth token=[REDACTED]",
		},
		{
			name:  "api_key with equals",
			input: "using api_key=AKIAIOSFODNN7EXAMPLE",
			want:  "using api_key=[REDACTED]",
		},
		{
			name:  "case insensitive",
			input: "PASSWORD=hunter2 TOKEN=xyz",
			want:  "PASSWORD=[REDACTED] TOKEN=[REDACTED]",
		},
		{
			name:  "no sensitive data",
			input: "opening browser for authentication",
			want:  "opening browser for authentication",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sanitizeLogMessage(tt.input))
		})
	}
}

func TestSanitizeFieldValue(t *testing.T) {
	// Sensitive field keys should have their values redacted.
	assert.Equal(t, "[REDACTED]", sanitizeFieldValue("password", "hunter2"))
	assert.Equal(t, "[REDACTED]", sanitizeFieldValue("Password", "secret"))
	assert.Equal(t, "[REDACTED]", sanitizeFieldValue("token", "abc123"))
	assert.Equal(t, "[REDACTED]", sanitizeFieldValue("api_key", "AKIAEXAMPLE"))
	assert.Equal(t, "[REDACTED]", sanitizeFieldValue("secret", "s3cret"))
	assert.Equal(t, "[REDACTED]", sanitizeFieldValue("credential", "cred123"))
	assert.Equal(t, "[REDACTED]", sanitizeFieldValue("session_id", "sess123"))
	assert.Equal(t, "[REDACTED]", sanitizeFieldValue("authorization", "Bearer xyz"))
	assert.Equal(t, "[REDACTED]", sanitizeFieldValue("cookie", "session=abc"))
	assert.Equal(t, "[REDACTED]", sanitizeFieldValue("private_key", "-----BEGIN RSA"))
	assert.Equal(t, "[REDACTED]", sanitizeFieldValue("client_secret", "cs_live_xxx"))

	// Substring matching: composite key names are also caught.
	assert.Equal(t, "[REDACTED]", sanitizeFieldValue("x_auth_token", "tok123"))
	assert.Equal(t, "[REDACTED]", sanitizeFieldValue("saml_session_cookie", "sess"))

	// Non-sensitive keys should pass through unchanged.
	assert.Equal(t, "browser", sanitizeFieldValue("provider", "browser"))
	assert.Equal(t, "https://idp.example.com", sanitizeFieldValue("url", "https://idp.example.com"))
	assert.Equal(t, 42, sanitizeFieldValue("attempts", 42))

	// Non-sensitive key with embedded sensitive data in the string value.
	result := sanitizeFieldValue("details", "login failed password=s3cret123")
	assert.NotContains(t, result, "s3cret123", "embedded password in non-sensitive key's value must be redacted")
	assert.Contains(t, result, "[REDACTED]")
}

func TestLogrusAdapter_Write_RedactsSensitiveData(t *testing.T) {
	// Verify that sensitive data in JSON messages is redacted before logging.
	rec := &recordingLogger{}
	adapter := &logrusAdapter{logger: rec}

	// JSON message with password in msg field.
	msg := `{"level":"error","msg":"login failed password=s3cret123","provider":"browser"}` + "\n"
	_, err := adapter.Write([]byte(msg))
	assert.NoError(t, err)
	assert.Equal(t, "error", rec.level)
	assert.NotContains(t, rec.msg, "s3cret123", "password value must be redacted from msg")
	assert.Contains(t, rec.msg, "[REDACTED]")

	// Non-JSON fallback with sensitive data.
	rec2 := &recordingLogger{}
	adapter2 := &logrusAdapter{logger: rec2}
	_, err = adapter2.Write([]byte("auth token=abc123def456\n"))
	assert.NoError(t, err)
	assert.NotContains(t, rec2.msg, "abc123def456", "token must be redacted in fallback path")
	assert.Contains(t, rec2.msg, "[REDACTED]")
}
