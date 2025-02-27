package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

// TestInitCliConfig should initialize atmos configuration with the correct base path and atmos Config File Path.
// It should also check that the base path and atmos Config File Path are correctly set and directory.
func TestInitCliConfig(t *testing.T) {
	atmosConfigFilePath := "test-config.yaml"
	configAndStacksInfo := schema.ConfigAndStacksInfo{}
	// create tmp folder
	tmpDir, err := os.MkdirTemp("", "atmos-config-test")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	// create atmos.yaml file
	atmosConfigFilePath = filepath.Join(tmpDir, atmosConfigFilePath)
	f, err := os.Create(atmosConfigFilePath)
	if err != nil {
		t.Fatalf("Failed to create atmos.yaml file: %v", err)
	}
	content := []string{
		"base_path: ./\n",
		"logs:\n",
		"  file: /dev/stderr\n",
		"  level: Info",
	}

	for _, line := range content {
		if _, err := f.WriteString(line); err != nil {
			t.Fatalf("Failed to write to config file: %v", err)
		}
	}
	f.Close()

	// get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Errorf("Failed to change directory back: %v", err)
		}
	}()
	// change to tmp dir
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Failed to change to temp directory: %v", err)
	}
	// initialize atmos configuration
	atmosConfig, err := InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		t.Fatalf("Failed to initialize atmos config: %v", err)
	}
	if atmosConfig.BasePath != "." {
		t.Errorf("Base path should be %s, got %s", ".", atmosConfig.BasePath)
	}
	// assert that atmos Config File Path is set correctly
	assert.Contains(t, atmosConfig.CliConfigPath, tmpDir)

	infoBase, err := os.Stat(atmosConfig.BasePath)
	if err != nil {
		t.Fatalf("Failed to stat base path: %v", err)
	}
	// Check base path and atmos Config File Path are directories
	if !infoBase.IsDir() {
		t.Errorf("Base path should be a directory, got %s", atmosConfig.BasePath)
	}
	infoCliConfig, err := os.Stat(atmosConfig.CliConfigPath)
	if err != nil {
		t.Fatalf("Failed to stat cli config path: %v", err)
	}
	if !infoCliConfig.IsDir() {
		t.Errorf("Cli config path should be a directory, got %s", atmosConfig.CliConfigPath)
	}
}
