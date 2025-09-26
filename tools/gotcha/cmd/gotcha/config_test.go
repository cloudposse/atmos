package cmd

import (
	"os"
	"path/filepath"
	"testing"

	constants "github.com/cloudposse/atmos/tools/gotcha/pkg/constants"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigLoading(t *testing.T) {
	// Save and restore original config
	defer viper.Reset()

	tests := []struct {
		name           string
		configContent  string
		configFile     string
		envVars        map[string]string
		expectedOutput string
		expectedShow   string
	}{
		{
			name: "load from config file",
			configContent: `output: from-config.json
show: failed
timeout: 10s`,
			expectedOutput: "from-config.json",
			expectedShow:   "failed",
		},
		{
			name: "environment variables override config",
			configContent: `output: from-config.json
show: failed`,
			envVars: map[string]string{
				"GOTCHA_OUTPUT": "from-env.json",
				"GOTCHA_SHOW":   "all",
			},
			expectedOutput: "from-env.json",
			expectedShow:   "all",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper for each test
			viper.Reset()

			// Create temp config file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, ".gotcha.yaml")
			if tt.configContent != "" {
				err := os.WriteFile(configPath, []byte(tt.configContent), constants.DefaultFilePerms)
				require.NoError(t, err)
			}

			// Set environment variables
			for key, value := range tt.envVars {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}

			// Set config file path
			if tt.configFile != "" {
				configFile = tt.configFile
			} else {
				configFile = configPath
			}

			// Initialize config
			initConfig()

			// Check values
			assert.Equal(t, tt.expectedOutput, viper.GetString("output"))
			assert.Equal(t, tt.expectedShow, viper.GetString("show"))
		})
	}
}

func TestConfigPrecedence(t *testing.T) {
	// Test that precedence is: flags > env > config > defaults
	defer viper.Reset()

	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".gotcha.yaml")
	configContent := `output: config-output.json
show: failed
timeout: 5s
alert: false`

	err := os.WriteFile(configPath, []byte(configContent), constants.DefaultFilePerms)
	require.NoError(t, err)

	// Test 1: Config file values
	viper.Reset()
	configFile = configPath
	initConfig()
	assert.Equal(t, "config-output.json", viper.GetString("output"))
	assert.Equal(t, "failed", viper.GetString("show"))
	assert.Equal(t, "5s", viper.GetString("timeout"))
	assert.Equal(t, false, viper.GetBool("alert"))

	// Test 2: Environment variables override config
	viper.Reset()
	os.Setenv("GOTCHA_OUTPUT", "env-output.json")
	os.Setenv("GOTCHA_SHOW", "all")
	defer os.Unsetenv("GOTCHA_OUTPUT")
	defer os.Unsetenv("GOTCHA_SHOW")

	configFile = configPath
	initConfig()
	assert.Equal(t, "env-output.json", viper.GetString("output"))
	assert.Equal(t, "all", viper.GetString("show"))
	assert.Equal(t, "5s", viper.GetString("timeout")) // Still from config
	assert.Equal(t, false, viper.GetBool("alert"))    // Still from config

	// Test 3: Flags override everything (simulated by viper.Set)
	viper.Set("output", "flag-output.json")
	assert.Equal(t, "flag-output.json", viper.GetString("output"))
	assert.Equal(t, "all", viper.GetString("show"))   // Still from env
	assert.Equal(t, "5s", viper.GetString("timeout")) // Still from config
}

func TestCustomConfigFile(t *testing.T) {
	defer viper.Reset()

	// Create two config files
	tmpDir := t.TempDir()

	defaultConfig := filepath.Join(tmpDir, ".gotcha.yaml")
	err := os.WriteFile(defaultConfig, []byte(`output: default.json`), constants.DefaultFilePerms)
	require.NoError(t, err)

	customConfig := filepath.Join(tmpDir, "custom.yaml")
	err = os.WriteFile(customConfig, []byte(`output: custom.json`), constants.DefaultFilePerms)
	require.NoError(t, err)

	// Test loading custom config
	viper.Reset()
	configFile = customConfig
	initConfig()
	assert.Equal(t, "custom.json", viper.GetString("output"))
}

func TestMissingConfigFile(t *testing.T) {
	defer viper.Reset()

	// Point to non-existent config
	configFile = "/non/existent/config.yaml"

	// Should not panic, just use defaults
	assert.NotPanics(t, func() {
		initConfig()
	})

	// Can still set via environment
	os.Setenv("GOTCHA_OUTPUT", "env-output.json")
	defer os.Unsetenv("GOTCHA_OUTPUT")

	viper.Reset()
	initConfig()
	assert.Equal(t, "env-output.json", viper.GetString("output"))
}
