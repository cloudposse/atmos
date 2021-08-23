package utils

import (
	"os"
	"path/filepath"
)

// IsDirectory checks if the path is a directory
func IsDirectory(path string) (bool, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return fileInfo.IsDir(), err
}

// FileExists checks if a file exists and is not a directory
func FileExists(filename string) bool {
	fileInfo, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return !fileInfo.IsDir()
}

// IsYaml checks if the file has YAML extension (does not check file schema, nor validates the file)
func IsYaml(file string) bool {
	yamlExtensions := []string{".yaml", ".yml"}
	ext := filepath.Ext(file)
	return SliceContainsString(yamlExtensions, ext)
}
