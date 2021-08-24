package utils

import (
	"os"
	"path"
	"path/filepath"
	"strings"
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

// ConvertPathsToAbsolutePaths converts a slice of paths to a slice of absolute paths
func ConvertPathsToAbsolutePaths(paths []string) ([]string, error) {
	res := []string{}

	for _, dir := range paths {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return nil, err
		}
		res = append(res, abs)
	}

	return res, nil
}

// JoinAbsolutePathWithPaths joins a base path with each item in the path slice and returns a slice of absolute paths
func JoinAbsolutePathWithPaths(basePath string, paths []string) ([]string, error) {
	res := []string{}

	for _, p := range paths {
		res = append(res, path.Join(basePath, p))
	}

	return res, nil
}

// TrimBasePathFromPath trims the base path prefix from the path
func TrimBasePathFromPath(basePath string, path string) string {
	return strings.TrimPrefix(path, basePath)
}
