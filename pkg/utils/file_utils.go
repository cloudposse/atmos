package utils

import (
	"fmt"
	"io/fs"
	"net/url"
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
	yamlExtensions := []string{YamlFileExtension, YmlFileExtension, YamlTemplateExtension, YmlTemplateExtension}
	ext := filepath.Ext(file)
	if ext == ".tmpl" {
		// For .tmpl files, we check if the full extension is .yaml.tmpl or .yml.tmpl
		baseExt := filepath.Ext(strings.TrimSuffix(file, ext))
		ext = baseExt + ext
	}
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

// IsSocket checks if a file is a socket
func IsSocket(path string) (bool, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	isSocket := fileInfo.Mode().Type() == fs.ModeSocket
	return isSocket, nil
}

// SearchConfigFile searches for a config file in the provided path.
// If the path has a file extension, it checks if the file exists.
// If the path does not have a file extension, it checks for the existence of the file with the provided path and the possible config file extensions
func SearchConfigFile(path string) (string, bool) {
	// Check if the provided path has a file extension and the file exists
	if filepath.Ext(path) != "" {
		return path, FileExists(path)
	}

	// Check the possible config file extensions
	configExtensions := []string{YamlFileExtension, YmlFileExtension, YamlTemplateExtension, YmlTemplateExtension}
	for _, ext := range configExtensions {
		filePath := path + ext
		if FileExists(filePath) {
			return filePath, true
		}
	}

	return "", false
}

// IsURL checks if a string is a URL
func IsURL(s string) bool {
	u, err := url.Parse(s)
	if err != nil {
		return false
	}

	validSchemes := []string{"http", "https"}
	schemeValid := false
	for _, scheme := range validSchemes {
		if u.Scheme == scheme {
			schemeValid = true
			break
		}
	}

	return schemeValid
}

// GetFileNameFromURL extracts the file name from a URL
func GetFileNameFromURL(rawURL string) (string, error) {
	if rawURL == "" {
		return "", fmt.Errorf("empty URL provided")
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	// Extract the path from the URL
	urlPath := parsedURL.Path

	// Get the base name of the path
	fileName := path.Base(urlPath)
	if fileName == "/" || fileName == "." {
		return "", fmt.Errorf("unable to extract filename from URL: %s", rawURL)
	}
	return fileName, nil
}

// ParseGitHubURL parses a GitHub URL and returns the owner, repo, file path and branch
func ParseGitHubURL(rawURL string) (owner, repo, filePath, branch string, err error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", "", "", "", fmt.Errorf("invalid URL: %w", err)
	}

	if !strings.Contains(parsedURL.Host, "github.com") {
		return "", "", "", "", fmt.Errorf("URL is not a GitHub URL")
	}

	pathParts := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")
	if len(pathParts) < 4 || pathParts[2] != "blob" {
		return "", "", "", "", fmt.Errorf("URL format not supported. Expected: /owner/repo/blob/branch/filepath")
	}

	owner = pathParts[0]
	repo = pathParts[1]
	branch = pathParts[3]
	filePath = strings.Join(pathParts[4:], "/")

	return owner, repo, filePath, branch, nil
}
