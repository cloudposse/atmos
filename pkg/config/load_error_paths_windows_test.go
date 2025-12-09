//go:build windows
// +build windows

package config

import (
	"os"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// TestReadSystemConfig_WindowsEmptyAppData tests Windows APPDATA handling at load.go:198-201.
func TestReadSystemConfig_WindowsEmptyAppData(t *testing.T) {
	// Save original env var.
	origAppData := os.Getenv(WindowsAppDataEnvVar)
	defer os.Setenv(WindowsAppDataEnvVar, origAppData)

	// Test with empty LOCALAPPDATA.
	os.Setenv(WindowsAppDataEnvVar, "")

	v := viper.New()
	v.SetConfigType("yaml")

	err := readSystemConfig(v)
	assert.NoError(t, err) // Should not error, just skip.
}
