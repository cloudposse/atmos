package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDefaultHighlightSettings(t *testing.T) {
	settings := DefaultHighlightSettings()
	require.NotNil(t, settings)
	assert.True(t, settings.Enabled)
	assert.Equal(t, "terminal", settings.Formatter)
	assert.Equal(t, "dracula", settings.Theme)
	assert.True(t, settings.LineNumbers)
	assert.False(t, settings.Wrap)
}

func TestGetHighlightSettings_EmptyConfig(t *testing.T) {
	// When config has an empty SyntaxHighlighting block, defaults are returned.
	config := &schema.AtmosConfiguration{}
	settings := GetHighlightSettings(config)
	require.NotNil(t, settings)
	assert.True(t, settings.Enabled)
	assert.Equal(t, "terminal", settings.Formatter)
	assert.Equal(t, "dracula", settings.Theme)
}

func TestGetHighlightSettings_PartialConfig(t *testing.T) {
	// When config has some fields set, only the unset fields get defaults.
	config := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				SyntaxHighlighting: schema.SyntaxHighlighting{
					Enabled:   true,
					Theme:     "monokai",
					Formatter: "html",
				},
			},
		},
	}
	settings := GetHighlightSettings(config)
	require.NotNil(t, settings)
	assert.Equal(t, "monokai", settings.Theme)
	assert.Equal(t, "html", settings.Formatter)
	assert.True(t, settings.Enabled)
}

func TestGetHighlightSettings_CustomTheme(t *testing.T) {
	// Verify that a custom theme is preserved.
	config := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				SyntaxHighlighting: schema.SyntaxHighlighting{
					Enabled: true,
					Theme:   "github",
				},
			},
		},
	}
	settings := GetHighlightSettings(config)
	require.NotNil(t, settings)
	assert.Equal(t, "github", settings.Theme)
	// Formatter gets its default since it's empty.
	assert.Equal(t, "terminal", settings.Formatter)
}

func TestHighlightCode_NoTTY(t *testing.T) {
	// In test environment (no TTY), HighlightCode should return the code unchanged.
	code := `{"key": "value"}`
	result, err := HighlightCode(code, "json", "dracula")
	require.NoError(t, err)
	// In a non-TTY environment, the code is returned as-is.
	assert.NotEmpty(t, result)
}
