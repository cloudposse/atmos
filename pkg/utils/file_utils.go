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

// FileExists checks if the file exists and is not a directory
func FileExists(filename string) bool {
	fileInfo, err := os.Stat(filename)
	if os.IsNotExist(err) || err != nil {
		return false
	}
	return !fileInfo.IsDir()
}

// FileOrDirExists checks if the file or directory exists
func FileOrDirExists(filename string) bool {
	_, err := os.Stat(filename)
	if err != nil {
		return false
	}
	return true
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

// IsPathAbsolute checks if the provided file path is absolute
func IsPathAbsolute(path string) bool {
	return filepath.IsAbs(path)
}

// JoinAbsolutePathWithPath checks if the provided path is absolute. If the provided path is relative, it joins the base path with the path and returns the absolute path
func JoinAbsolutePathWithPath(basePath string, providedPath string) (string, error) {
	// If the provided path is an absolute path, return it
	if filepath.IsAbs(providedPath) {
		return providedPath, nil
	}

	// Join the base path with the provided path
	joinedPath := path.Join(basePath, providedPath)

	// If the joined path is an absolute path and exists in the file system, return it
	if filepath.IsAbs(joinedPath) {
		_, err := os.Stat(joinedPath)
		if err == nil {
			return joinedPath, nil
		}
	}

	// Convert the joined path to an absolute path
	absPath, err := filepath.Abs(joinedPath)
	if err != nil {
		return "", err
	}

	// Check if the final absolute path exists in the file system
	_, err = os.Stat(absPath)
	if err != nil {
		return "", err
	}

	return absPath, nil
}

// EnsureDir accepts a file path and creates all the intermediate subdirectories
func EnsureDir(fileName string) error {
	dirName := filepath.Dir(fileName)
	if _, err := os.Stat(dirName); err != nil {
		err := os.MkdirAll(dirName, os.ModePerm)
		if err != nil {
			return err
		}
	}
	return nil
}

// SliceOfPathsContainsPath checks if a slice of file paths contains a path
func SliceOfPathsContainsPath(paths []string, checkPath string) bool {
	for _, v := range paths {
		dir := path.Dir(v)
		if dir == checkPath {
			return true
		}
	}
	return false
}

// GetAllFilesInDir returns all files in the provided directory and all subdirectories
func GetAllFilesInDir(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			files = append(files, strings.TrimPrefix(TrimBasePathFromPath(dir, path), "/"))
		}
		return nil
	})
	return files, err
}

// GetAllYamlFilesInDir returns all YAML files in the provided directory and all subdirectories
func GetAllYamlFilesInDir(dir string) ([]string, error) {
	var res []string

	allFiles, err := GetAllFilesInDir(dir)
	if err != nil {
		return nil, err
	}

	for _, f := range allFiles {
		if IsYaml(f) {
			res = append(res, f)
		}
	}

	return res, nil
}
