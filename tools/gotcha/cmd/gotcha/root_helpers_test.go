package cmd

import (
	"testing"

	"github.com/charmbracelet/lipgloss"
	log "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected log.Level
	}{
		{"debug level", "debug", log.DebugLevel},
		{"info level", "info", log.InfoLevel},
		{"warn level", "warn", log.WarnLevel},
		{"warning level", "warning", log.WarnLevel},
		{"error level", "error", log.ErrorLevel},
		{"fatal level", "fatal", log.FatalLevel},
		{"invalid level defaults to info", "invalid", log.InfoLevel},
		{"empty string defaults to info", "", log.InfoLevel},
		{"mixed case", "DEBUG", log.DebugLevel}, // Should handle uppercase
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseLogLevel(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetLoggerStyles(t *testing.T) {
	tests := []struct {
		name         string
		expectStyles bool
	}{
		{
			name:         "get logger styles",
			expectStyles: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			styles := getLoggerStyles()
			
			// Check that styles are not nil
			assert.NotNil(t, styles)
			
			// Verify structure of returned styles
			assert.NotNil(t, styles.Levels)
			assert.NotNil(t, styles.Key)
			assert.NotNil(t, styles.Value)
			
			// Check that level styles are properly set
			debugStyle, ok := styles.Levels[log.DebugLevel]
			assert.True(t, ok)
			assert.IsType(t, lipgloss.Style{}, debugStyle)
			
			infoStyle, ok := styles.Levels[log.InfoLevel]
			assert.True(t, ok)
			assert.IsType(t, lipgloss.Style{}, infoStyle)
			
			warnStyle, ok := styles.Levels[log.WarnLevel]
			assert.True(t, ok)
			assert.IsType(t, lipgloss.Style{}, warnStyle)
			
			errorStyle, ok := styles.Levels[log.ErrorLevel]
			assert.True(t, ok)
			assert.IsType(t, lipgloss.Style{}, errorStyle)
			
			fatalStyle, ok := styles.Levels[log.FatalLevel]
			assert.True(t, ok)
			assert.IsType(t, lipgloss.Style{}, fatalStyle)
		})
	}
}

func TestInitGlobalLogger(t *testing.T) {
	tests := []struct {
		name string
	}{
		{
			name: "initialize global logger",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This should not panic
			initGlobalLogger()
			
			// Verify logger is initialized
			logger := log.Default()
			assert.NotNil(t, logger)
		})
	}
}

func TestBindAndParseFlagsHelper(t *testing.T) {
	tests := []struct {
		name        string
		expectError bool
	}{
		{
			name:        "basic flag binding",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := createRootCommand()
			setupGlobalFlags(cmd)
			
			err := bindAndParseFlags(cmd)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}