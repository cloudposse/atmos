package config

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

func TestBasePathComputingWithBasePathArg(t *testing.T) {
	configLoader := &ConfigLoader{}

	info := schema.ConfigAndStacksInfo{
		BasePathFromArg: ".",
	}

	expectedPath, _ := filepath.Abs(".")

	result, err := configLoader.BasePathComputing(info)

	assert.NoError(t, err)
	assert.Equal(t, expectedPath, result)
	// test non-existent base path
	info.BasePathFromArg = "invalid/path"
	expectedPath, _ = filepath.Abs("invalid/path")
	result, err = configLoader.BasePathComputing(info)
	assert.Error(t, err)
	assert.Equal(t, "", result)

	// test base pat not directory
	tempDir, err := os.MkdirTemp("", "config-test")
	if err != nil {
		t.Fatal(err)
	}
	tempDir = filepath.FromSlash(tempDir)
	defer os.RemoveAll(tempDir)
	configFile1 := filepath.Join(tempDir, "config1.yaml")
	err = os.WriteFile(configFile1, []byte("key1: value1"), 0644)
	assert.NoError(t, err)
	info.BasePathFromArg = configFile1
	result, err = configLoader.BasePathComputing(info)
	assert.Error(t, err)
	assert.Equal(t, "", result)

}

// test env base path
func TestBasePathComputingWithEnvVar(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "config-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	configLoader := &ConfigLoader{}
	// set env
	os.Setenv("ATMOS_BASE_PATH", tempDir)
	defer os.Unsetenv("ATMOS_BASE_PATH")
	info := schema.ConfigAndStacksInfo{
		BasePathFromArg: "",
	}
	expectedPath, _ := filepath.Abs(tempDir)
	result, err := configLoader.BasePathComputing(info)
	assert.NoError(t, err)
	assert.Equal(t, expectedPath, result)
}

// test  base_path Set in Configuration
func TestBasePathComputingWithBasePathSetInConfiguration(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "config-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)
	configLoader := &ConfigLoader{}
	configLoader.atmosConfig.BasePath = tempDir
	info := schema.ConfigAndStacksInfo{
		BasePathFromArg: "",
	}
	expectedPath, _ := filepath.Abs(tempDir)
	result, err := configLoader.BasePathComputing(info)
	assert.NoError(t, err)
	assert.Equal(t, expectedPath, result)
	// test base pat not abs directory
	// change pwd to temp
	startingDir, err := os.Getwd()
	if err != nil {
		fmt.Printf("Failed to get the current working directory: %v\n", err)
		os.Exit(1) // Exit with a non-zero code to indicate failure
	}
	defer func() {
		// Change back to the original working directory after the test
		if err := os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to the starting directory: %v", err)
		}
	}()
	err = os.Chdir(tempDir)
	if err != nil {
		t.Fatalf("Failed to change directory to %q: %v", tempDir, err)
	}
	// dir under temp
	subDirTemp := filepath.Join(tempDir, "sub-dir")
	err = os.Mkdir(subDirTemp, 0755)
	if err != nil {
		t.Fatalf("Failed to create directory %q: %v", subDirTemp, err)
	}
	expectedPath, _ = filepath.Abs(subDirTemp)

	configLoader.atmosConfig.BasePath = "sub-dir"
	result, err = configLoader.BasePathComputing(info)
	assert.NoError(t, err)
	assert.Equal(t, expectedPath, result)

}
