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
	defer func() {
		logrus.SetFormatter(originalFormatter)
		logrus.SetLevel(originalLevel)
	}()

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

	// Verify level is set to Info.
	assert.Equal(t, logrus.InfoLevel, logrus.GetLevel())
}
