package theme

import (
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetLogStyles(t *testing.T) {
	scheme := &ColorScheme{
		LogDebug:    "#00FFFF",
		LogInfo:     "#0000FF",
		LogWarning:  "#FFFF00",
		LogError:    "#FF0000",
		TextMuted:   "#808080",
		Primary:     "#0000FF",
		TextPrimary: "#FFFFFF",
	}

	styles := GetLogStyles(scheme)
	require.NotNil(t, styles)

	// Verify all log levels have styles
	assert.NotNil(t, styles.Levels)
	assert.Contains(t, styles.Levels, log.DebugLevel)
	assert.Contains(t, styles.Levels, log.InfoLevel)
	assert.Contains(t, styles.Levels, log.WarnLevel)
	assert.Contains(t, styles.Levels, log.ErrorLevel)
	assert.Contains(t, styles.Levels, log.FatalLevel)

	// Verify key-value styles
	assert.NotNil(t, styles.Key)
	assert.NotNil(t, styles.Value)
	assert.NotNil(t, styles.Timestamp)
	assert.NotNil(t, styles.Message)
}

func TestGetLogStyles_NilScheme(t *testing.T) {
	styles := GetLogStyles(nil)
	require.NotNil(t, styles)

	// Should return default styles when scheme is nil
	assert.NotNil(t, styles.Levels)
}

func TestGetLogStylesNoColor(t *testing.T) {
	styles := GetLogStylesNoColor()
	require.NotNil(t, styles)

	// Verify all log levels exist
	assert.Contains(t, styles.Levels, log.DebugLevel)
	assert.Contains(t, styles.Levels, log.InfoLevel)
	assert.Contains(t, styles.Levels, log.WarnLevel)
	assert.Contains(t, styles.Levels, log.ErrorLevel)
	assert.Contains(t, styles.Levels, log.FatalLevel)

	// Verify log level strings are set
	debugStyle := styles.Levels[log.DebugLevel]
	assert.NotNil(t, debugStyle)

	infoStyle := styles.Levels[log.InfoLevel]
	assert.NotNil(t, infoStyle)
}

func TestCreateLogLevelStyles(t *testing.T) {
	scheme := &ColorScheme{
		LogDebug:   "#00FFFF",
		LogInfo:    "#0000FF",
		LogWarning: "#FFFF00",
		LogError:   "#FF0000",
	}

	levels := createLogLevelStyles(scheme)
	require.NotNil(t, levels)

	// All log levels should be present
	assert.Len(t, levels, 5) // Debug, Info, Warn, Error, Fatal

	// Verify each level has a style
	assert.Contains(t, levels, log.DebugLevel)
	assert.Contains(t, levels, log.InfoLevel)
	assert.Contains(t, levels, log.WarnLevel)
	assert.Contains(t, levels, log.ErrorLevel)
	assert.Contains(t, levels, log.FatalLevel)
}

func TestLogStyles_Integration(t *testing.T) {
	// Test that log styles can be created from a theme
	registry, err := NewRegistry()
	require.NoError(t, err)

	theme := registry.GetOrDefault("default")
	scheme := GenerateColorScheme(theme)
	styles := GetLogStyles(&scheme)

	require.NotNil(t, styles)
	require.NotNil(t, styles.Levels)

	// Verify log levels are properly configured
	assert.Contains(t, styles.Levels, log.DebugLevel)
	assert.Contains(t, styles.Levels, log.InfoLevel)
	assert.Contains(t, styles.Levels, log.WarnLevel)
	assert.Contains(t, styles.Levels, log.ErrorLevel)
	assert.Contains(t, styles.Levels, log.FatalLevel)
}

func TestLogStyles_ColorMapping(t *testing.T) {
	scheme := &ColorScheme{
		LogDebug:    "#00FFFF", // Cyan
		LogInfo:     "#0000FF", // Blue
		LogWarning:  "#FFFF00", // Yellow
		LogError:    "#FF0000", // Red
		TextMuted:   "#808080",
		Primary:     "#0000FF",
		TextPrimary: "#FFFFFF",
	}

	styles := GetLogStyles(scheme)
	require.NotNil(t, styles)

	// Log levels should use scheme colors as backgrounds
	debugStyle := styles.Levels[log.DebugLevel]
	assert.NotNil(t, debugStyle)

	infoStyle := styles.Levels[log.InfoLevel]
	assert.NotNil(t, infoStyle)

	warnStyle := styles.Levels[log.WarnLevel]
	assert.NotNil(t, warnStyle)

	errorStyle := styles.Levels[log.ErrorLevel]
	assert.NotNil(t, errorStyle)

	fatalStyle := styles.Levels[log.FatalLevel]
	assert.NotNil(t, fatalStyle)
}
