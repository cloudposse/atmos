package logger

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/charmbracelet/lipgloss"
	log "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCharmLogger(t *testing.T) {
	logger := GetCharmLogger()
	require.NotNil(t, logger, "Should return a non-nil logger")

	// These should not panic.
	assert.NotPanics(t, func() {
		logger.SetLevel(log.InfoLevel)
		logger.SetTimeFormat("")
	})
}

func TestGetCharmLoggerWithOutput(t *testing.T) {
	tempDir := os.TempDir()
	logFile := filepath.Join(tempDir, "charm_test.log")
	defer os.Remove(logFile)

	f, err := os.Create(logFile)
	require.NoError(t, err, "Should create log file without error")

	logger := GetCharmLoggerWithOutput(f)
	require.NotNil(t, logger, "Should return a non-nil logger")

	logger.SetTimeFormat("")
	logger.Info("File test message")

	f.Close()

	data, err := os.ReadFile(logFile)
	require.NoError(t, err, "Should read log file without error")

	content := string(data)
	assert.Contains(t, content, "INFO", "Should have INFO level in file")
	assert.Contains(t, content, "File test message", "Should contain the message")
}

// Test the actual styling implementation.
func TestCharmLoggerStylingDetails(t *testing.T) {
	styles := getAtmosLogStyles()

	assert.NotEqual(t, lipgloss.Style{}, styles.Levels[log.ErrorLevel], "ERROR level should have styling")
	assert.NotEqual(t, lipgloss.Style{}, styles.Levels[log.WarnLevel], "WARN level should have styling")
	assert.NotEqual(t, lipgloss.Style{}, styles.Levels[log.InfoLevel], "INFO level should have styling")
	assert.NotEqual(t, lipgloss.Style{}, styles.Levels[log.DebugLevel], "DEBUG level should have styling")

	assert.Contains(t, styles.Levels[log.ErrorLevel].Render("ERROR"), "ERROR", "ERROR label should be styled")
	assert.Contains(t, styles.Levels[log.WarnLevel].Render("WARN"), "WARN", "WARN label should be styled")
	assert.Contains(t, styles.Levels[log.InfoLevel].Render("INFO"), "INFO", "INFO label should be styled")
	assert.Contains(t, styles.Levels[log.DebugLevel].Render("DEBUG"), "DEBUG", "DEBUG label should be styled")

	assert.NotNil(t, styles.Keys["err"], "err key should have styling")
	assert.NotNil(t, styles.Values["err"], "err value should have styling")
	assert.NotNil(t, styles.Keys["component"], "component key should have styling")
	assert.NotNil(t, styles.Keys["stack"], "stack key should have styling")
}

func ExampleGetCharmLogger() {
	logger := GetCharmLogger()

	logger.SetTimeFormat("2006-01-02 15:04:05")
	logger.SetLevel(log.InfoLevel)

	logger.Info("User logged in", "user_id", "12345", "component", "auth")

	logger.Error("Failed to process request",
		"err", "connection timeout",
		"component", "api",
		"duration", "1.5s")

	logger.Warn("Resource utilization high",
		"component", "database",
		"stack", "prod-ue1",
		"usage", "95%")

	logger.Debug("Processing request",
		"request_id", "abc123",
		"component", "api",
		"endpoint", "/users",
		"method", "GET")
}
