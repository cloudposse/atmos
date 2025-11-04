package logger

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

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

			// Verify formatter is TextFormatter with correct settings.
			formatter, ok := logrus.StandardLogger().Formatter.(*logrus.TextFormatter)
			assert.True(t, ok, "Formatter should be TextFormatter")
			if formatter != nil {
				assert.True(t, formatter.DisableTimestamp, "DisableTimestamp should be true")
				assert.True(t, formatter.DisableColors, "DisableColors should be true")
				assert.True(t, formatter.DisableQuote, "DisableQuote should be true")
			}

			// Verify level matches Atmos level.
			assert.Equal(t, tt.expectedLogrusLevel, logrus.GetLevel())
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
