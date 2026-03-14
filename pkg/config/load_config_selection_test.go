package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseConfigSelectionFromOsArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		basePath   string
		config     []string
		configPath []string
	}{
		{
			name:       "no flags",
			args:       []string{"terraform", "plan"},
			basePath:   "",
			config:     nil,
			configPath: nil,
		},
		{
			name:       "base-path only",
			args:       []string{"--base-path", "/my/path", "terraform", "plan"},
			basePath:   "/my/path",
			config:     nil,
			configPath: nil,
		},
		{
			name:       "base-path with equals syntax",
			args:       []string{"--base-path=/my/path"},
			basePath:   "/my/path",
			config:     nil,
			configPath: nil,
		},
		{
			name:       "config single file",
			args:       []string{"--config", "/etc/atmos.yaml"},
			basePath:   "",
			config:     []string{"/etc/atmos.yaml"},
			configPath: nil,
		},
		{
			name:       "config multiple files",
			args:       []string{"--config", "a.yaml", "--config", "b.yaml"},
			basePath:   "",
			config:     []string{"a.yaml", "b.yaml"},
			configPath: nil,
		},
		{
			name:       "config comma-separated",
			args:       []string{"--config=a.yaml,b.yaml"},
			basePath:   "",
			config:     []string{"a.yaml", "b.yaml"},
			configPath: nil,
		},
		{
			name:       "config-path single dir",
			args:       []string{"--config-path", "/etc/atmos.d"},
			basePath:   "",
			config:     nil,
			configPath: []string{"/etc/atmos.d"},
		},
		{
			name:       "config-path multiple dirs",
			args:       []string{"--config-path", "dir1", "--config-path", "dir2"},
			basePath:   "",
			config:     nil,
			configPath: []string{"dir1", "dir2"},
		},
		{
			name:       "all flags together",
			args:       []string{"--base-path", "/root", "--config", "c.yaml", "--config-path", "d1", "--profile", "dev"},
			basePath:   "/root",
			config:     []string{"c.yaml"},
			configPath: []string{"d1"},
		},
		{
			name:       "flags mixed with unknown flags",
			args:       []string{"--unknown", "val", "--base-path", "/p", "--verbose"},
			basePath:   "/p",
			config:     nil,
			configPath: nil,
		},
		{
			name:       "whitespace trimming",
			args:       []string{"--base-path", "  /my/path  "},
			basePath:   "/my/path",
			config:     nil,
			configPath: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sel := ParseConfigSelectionFromOsArgs(tt.args)
			assert.Equal(t, tt.basePath, sel.BasePath, "BasePath")
			assert.Equal(t, tt.config, sel.Config, "Config")
			assert.Equal(t, tt.configPath, sel.ConfigPath, "ConfigPath")
		})
	}
}

func TestConfigSelectionFromEnv(t *testing.T) {
	tests := []struct {
		name       string
		envVars    map[string]string
		basePath   string
		config     []string
		configPath []string
	}{
		{
			name:       "no env vars",
			envVars:    nil,
			basePath:   "",
			config:     nil,
			configPath: nil,
		},
		{
			name:       "ATMOS_BASE_PATH",
			envVars:    map[string]string{"ATMOS_BASE_PATH": "/env/path"},
			basePath:   "/env/path",
			config:     nil,
			configPath: nil,
		},
		{
			name:       "ATMOS_CONFIG single",
			envVars:    map[string]string{"ATMOS_CONFIG": "/etc/atmos.yaml"},
			basePath:   "",
			config:     []string{"/etc/atmos.yaml"},
			configPath: nil,
		},
		{
			name:       "ATMOS_CONFIG comma-separated",
			envVars:    map[string]string{"ATMOS_CONFIG": "a.yaml,b.yaml"},
			basePath:   "",
			config:     []string{"a.yaml", "b.yaml"},
			configPath: nil,
		},
		{
			name:       "ATMOS_CONFIG_PATH comma-separated",
			envVars:    map[string]string{"ATMOS_CONFIG_PATH": "dir1, dir2"},
			basePath:   "",
			config:     nil,
			configPath: []string{"dir1", "dir2"},
		},
		{
			name: "all env vars",
			envVars: map[string]string{
				"ATMOS_BASE_PATH":   "/base",
				"ATMOS_CONFIG":      "c.yaml",
				"ATMOS_CONFIG_PATH": "d1",
			},
			basePath:   "/base",
			config:     []string{"c.yaml"},
			configPath: []string{"d1"},
		},
		{
			name:       "whitespace and empty entries",
			envVars:    map[string]string{"ATMOS_CONFIG": " a.yaml , , b.yaml "},
			basePath:   "",
			config:     []string{"a.yaml", "b.yaml"},
			configPath: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.envVars {
				t.Setenv(k, v)
			}
			sel := ConfigSelectionFromEnv()
			assert.Equal(t, tt.basePath, sel.BasePath, "BasePath")
			assert.Equal(t, tt.config, sel.Config, "Config")
			assert.Equal(t, tt.configPath, sel.ConfigPath, "ConfigPath")
		})
	}
}
