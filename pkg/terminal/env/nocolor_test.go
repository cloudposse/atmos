package env

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveNoColor(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		atmosNoColor string
		noColor      string
		viperNoColor bool
		want         bool
	}{
		{"nothing set", nil, "", "", false, false},
		{"env true and CLI false wins", []string{"--no-color=false", "version"}, "true", "", true, false},
		{"env true, no CLI", []string{"version"}, "true", "", true, true},
		{"CLI true beats env false", []string{"--no-color"}, "false", "", false, true},
		{"CLI true, viper false", []string{"--no-color=true"}, "", "", false, true},
		{"NO_COLOR is absolute over CLI false", []string{"--no-color=false"}, "", "1", false, true},
		{"NO_COLOR is absolute over CLI false and env false", []string{"--no-color=false"}, "false", "1", false, true},
		{"no CLI, viper value honored", []string{"version"}, "", "", true, true},
		{"no CLI, viper false", []string{"version"}, "", "", false, false},
		{"args after double dash are ignored", []string{"--", "--no-color"}, "", "", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("ATMOS_NO_COLOR", tt.atmosNoColor)
			t.Setenv("NO_COLOR", tt.noColor)
			assert.Equal(t, tt.want, ResolveNoColor(tt.args, tt.viperNoColor))
		})
	}
}
