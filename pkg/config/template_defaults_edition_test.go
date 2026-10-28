package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestLoadConfigTemplateDefaultsEdition(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   bool
	}{
		{"unpinned", "", true},
		{"before transition", "edition: \"2026-10-25\"\n", false},
		{"on transition", "edition: \"2026-10-26\"\n", true},
		{"after transition", "edition: \"2026-10-27\"\n", true},
		{"month includes transition", "edition: \"2026-10\"\n", true},
		{"explicit enable beats old edition", "edition: \"2026-10-25\"\ntemplates:\n  settings:\n    enabled: true\n    sprig: {enabled: true}\n    gomplate: {enabled: true}\n", true},
		{"explicit disable beats current default", "templates:\n  settings:\n    enabled: false\n    sprig: {enabled: false}\n    gomplate: {enabled: false}\n", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			writeEditionTestConfig(t, "base_path: ./\n"+tt.config)
			config, err := LoadConfig(&schema.ConfigAndStacksInfo{})
			require.NoError(t, err)
			assert.Equal(t, tt.want, config.Templates.Settings.Enabled)
			assert.Equal(t, tt.want, config.Templates.Settings.Sprig.Enabled)
			assert.Equal(t, tt.want, config.Templates.Settings.Gomplate.Enabled)
		})
	}
}

func TestLoadConfigTemplateDefaultsWithoutFile(t *testing.T) {
	for _, pin := range []string{"", "2026-10-25", "2026-10-26"} {
		t.Run("pin="+pin, func(t *testing.T) {
			t.Chdir(t.TempDir())
			t.Setenv("ATMOS_EDITION", pin)
			config, err := LoadConfig(&schema.ConfigAndStacksInfo{})
			require.NoError(t, err)
			assert.True(t, config.Templates.Settings.Enabled)
			assert.True(t, config.Templates.Settings.Sprig.Enabled)
			assert.True(t, config.Templates.Settings.Gomplate.Enabled)
		})
	}
}
