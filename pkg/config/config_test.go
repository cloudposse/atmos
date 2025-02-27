package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

// test base path and atmosConfigFilePath
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
	// write atmos.yaml content
	f.WriteString("base_path: ./ \n")
	f.WriteString("logs:\n")
	f.WriteString("  file: /dev/stderr")
	f.WriteString("  level: Info")
	f.Close()
	// get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current working directory: %v", err)
	}
	defer os.Chdir(cwd)
	// change to tmp dir
	os.Chdir(tmpDir)
	// initialize atmos configuration
	atmosConfig, err := InitCliConfig(configAndStacksInfo, false)
	if err != nil {
		t.Fatalf("Failed to initialize atmos config: %v", err)
	}
	if atmosConfig.BasePath != "." {
		t.Errorf("Base path should be %s, got %s", ".", atmosConfig.BasePath)
	}
	if atmosConfig.CliConfigPath != tmpDir {
		t.Errorf("Cli config path should be %s, got %s", tmpDir, atmosConfig.CliConfigPath)
	}
	infoBase, err := os.Stat(atmosConfig.BasePath)
	if err != nil {
		t.Fatalf("Failed to stat base path: %v", err)
	}
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
