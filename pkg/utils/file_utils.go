package utils

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

var ErrFailedToProcessHclFile = errors.New("failed to process HCL file")

// IsDirectory checks if the path is a directory.
func IsDirectory(path string) (bool, error) {
	defer perf.Track(nil, "utils.IsDirectory")()

	fileInfo, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return fileInfo.IsDir(), err
}

// FileExists checks if the file exists and is not a directory.
func FileExists(filename string) bool {
	defer perf.Track(nil, "utils.FileExists")()

	fileInfo, err := os.Stat(filename)
	if os.IsNotExist(err) || err != nil {
		return false
	}
	return !fileInfo.IsDir()
}

// FileOrDirExists checks if the file or directory exists.
func FileOrDirExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

// IsYaml checks if the file has YAML extension (does not check file schema, nor validates the file).
func IsYaml(file string) bool {
	defer perf.Track(nil, "utils.IsYaml")()

	yamlExtensions := []string{YamlFileExtension, YmlFileExtension, YamlTemplateExtension, YmlTemplateExtension}
	ext := filepath.Ext(file)
	if ext == ".tmpl" {
		// For .tmpl files, we check if the full extension is .yaml.tmpl or .yml.tmpl
		baseExt := filepath.Ext(strings.TrimSuffix(file, ext))
		ext = baseExt + ext
	}
	return SliceContainsString(yamlExtensions, ext)
}

// ConvertPathsToAbsolutePaths converts a slice of paths to a slice of absolute paths.
func ConvertPathsToAbsolutePaths(paths []string) ([]string, error) {
	defer perf.Track(nil, "utils.ConvertPathsToAbsolutePaths")()

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

// JoinPaths joins a base path with each item in the path slice and returns a slice of joined paths.
// This is a pure path construction function without filesystem validation.
func JoinPaths(basePath string, paths []string) ([]string, error) {
	defer perf.Track(nil, "utils.JoinPaths")()

	res := []string{}

	for _, p := range paths {
		res = append(res, JoinPath(basePath, p))
	}

	return res, nil
}

// TrimBasePathFromPath trims the base path prefix from the path.
func TrimBasePathFromPath(basePath string, path string) string {
	basePath = filepath.ToSlash(basePath)
	path = filepath.ToSlash(path)
	return strings.TrimPrefix(path, basePath)
}

// IsPathAbsolute checks if the provided file path is absolute.
func IsPathAbsolute(path string) bool {
	return filepath.IsAbs(path)
}

// ExpandTilde expands ~ and ~/ in paths to the user's home directory.
// Returns the expanded path if it starts with ~, otherwise returns the original path.
func ExpandTilde(path string) (string, error) {
	defer perf.Track(nil, "utils.ExpandTilde")()

	if !strings.HasPrefix(path, "~") {
		return path, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}

	if path == "~" {
		return homeDir, nil
	}

	if strings.HasPrefix(path, "~/") {
		return filepath.Join(homeDir, path[2:]), nil
	}

	// Path starts with ~ but not ~/ (e.g., ~user), return as-is.
	// Go doesn't support ~user expansion.
	return path, nil
}

// handleEmptyPaths handles the case when one or both paths are empty.
func handleEmptyPaths(basePath, providedPath string) (string, bool) {
	if basePath == "" && providedPath == "" {
		return "", true
	}
	if basePath == "" {
		return providedPath, true
	}
	if providedPath == "" {
		return basePath, true
	}
	return "", false
}

// isWindowsAbsolutePath checks if a path is absolute on Windows.
// This handles special Windows cases that filepath.IsAbs might miss:
// - Paths starting with / (treated as absolute on current drive)
// - Single backslash paths like \Windows (absolute on current drive)
// Note: Double backslash (\\) paths are handled by filepath.IsAbs as UNC paths.
func isWindowsAbsolutePath(path string) bool {
	if len(path) == 0 {
		return false
	}
	// Only treat single backslash or forward slash at start as absolute
	// Double backslash (\\) is handled by filepath.IsAbs for UNC paths
	if path[0] == '/' {
		return true
	}
	if path[0] == '\\' && (len(path) == 1 || path[1] != '\\') {
		return true
	}
	return false
}

// JoinPath joins two paths handling absolute paths correctly.
// If the second path is absolute, it returns the second path.
// Otherwise, it joins the paths using filepath.Join which:
//   - Normalizes path separators to the OS-specific separator
//   - Cleans the resulting path (removes . and .. elements)
//   - Handles empty paths appropriately
//
// This function follows standard Go path behavior and does NOT check
// if the path exists on the filesystem.
func JoinPath(basePath string, providedPath string) string {
	defer perf.Track(nil, "utils.JoinPath")()

	// Handle empty paths
	if result, handled := handleEmptyPaths(basePath, providedPath); handled {
		return result
	}

	// If the provided path is an absolute path, return it
	if filepath.IsAbs(providedPath) {
		return providedPath
	}

	// On Windows, handle special cases that filepath.IsAbs doesn't catch
	// (paths starting with \ or / are absolute on Windows)
	if runtime.GOOS == "windows" && isWindowsAbsolutePath(providedPath) {
		return providedPath
	}

	// Join the base path with the provided path using standard Go behavior
	// filepath.Join will:
	// - Clean the path (remove . and .. elements)
	// - Normalize separators to OS-specific (\ on Windows, / on Unix)
	return filepath.Join(basePath, providedPath)
}

// JoinPathAndValidate joins paths and validates the result exists in filesystem.
// It builds on JoinPath for path construction and adds filesystem validation.
func JoinPathAndValidate(basePath string, providedPath string) (string, error) {
	defer perf.Track(nil, "utils.JoinPathAndValidate")()

	// Step 1: Use pure path construction
	constructedPath := JoinPath(basePath, providedPath)

	// Step 2: Convert to absolute path if needed
	if !filepath.IsAbs(constructedPath) {
		absPath, err := filepath.Abs(constructedPath)
		if err != nil {
			return "", err
		}
		constructedPath = absPath
	}

	// Step 3: Validate existence
	if _, err := os.Stat(constructedPath); err != nil {
		return "", err
	}

	return constructedPath, nil
}

// EnsureDir accepts a file path and creates all the intermediate subdirectories
func EnsureDir(fileName string) error {
	defer perf.Track(nil, "utils.EnsureDir")()

	dirName := filepath.Dir(fileName)
	if _, err := os.Stat(dirName); err != nil {
		err := os.MkdirAll(dirName, os.ModePerm)
		if err != nil {
			return err
		}
	}
	return nil
}

// SliceOfPathsContainsPath checks if a slice of file paths contains a path.
func SliceOfPathsContainsPath(paths []string, checkPath string) bool {
	for _, v := range paths {
		dir := filepath.Dir(v)
		if dir == checkPath {
			return true
		}
	}
	return false
}

// GetAllFilesInDir returns all files in the provided directory and all subdirectories.
func GetAllFilesInDir(dir string) ([]string, error) {
	defer perf.Track(nil, "utils.GetAllFilesInDir")()

	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			files = append(files, strings.TrimPrefix(TrimBasePathFromPath(dir, path), "/"))
		}
		return nil
	})
	return files, err
}

// GetAllYamlFilesInDir returns all YAML files in the provided directory and all subdirectories.
func GetAllYamlFilesInDir(dir string) ([]string, error) {
	defer perf.Track(nil, "utils.GetAllYamlFilesInDir")()

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

// IsSocket checks if a file is a socket.
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
// If the path does not have a file extension, it checks for the existence of the file with the provided path and the possible config file extensions.
func SearchConfigFile(path string) (string, bool) {
	defer perf.Track(nil, "utils.SearchConfigFile")()

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

// IsURL checks if a string is a URL.
func IsURL(s string) bool {
	defer perf.Track(nil, "utils.IsURL")()

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

// GetFileNameFromURL extracts the file name from a URL.
func GetFileNameFromURL(rawURL string) (string, error) {
	defer perf.Track(nil, "utils.GetFileNameFromURL")()

	if rawURL == "" {
		return "", errUtils.ErrEmptyURL
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	// Extract the path from the URL
	urlPath := parsedURL.Path

	// Get the base name of the path
	fileName := filepath.Base(urlPath)
	if fileName == "/" || fileName == "." {
		return "", fmt.Errorf("%w: %s", errUtils.ErrInvalidURL, rawURL)
	}
	return fileName, nil
}

// ResolveRelativePath checks if a path is relative to the current directory and if so,
// resolves it relative to the current file's directory. It ensures the resolved path
// exists within the base path.
func ResolveRelativePath(path string, basePath string) string {
	defer perf.Track(nil, "utils.ResolveRelativePath")()

	if path == "" {
		return path
	}

	// Convert all paths to use forward slashes for consistency in processing
	normalizedPath := filepath.ToSlash(path)
	normalizedBasePath := filepath.ToSlash(basePath)

	// Atmos import paths are generally relative paths, however, there are two types of relative paths:
	//   1. Paths relative to the base path (most common) - e.g. "mixins/region/us-east-2"
	//   2. Paths relative to the current file's directory (less common) - e.g. "./_defaults" imports will be relative to `./`
	//
	// Here we check if the path starts with "." or ".." to identify if it's relative to the current file.
	// If it is, we'll convert it to be relative to the file doing the import, rather than the `base_path`.
	parts := strings.Split(normalizedPath, "/")
	firstElement := filepath.Clean(parts[0])
	if firstElement == "." || firstElement == ".." {
		// Join the current local path with the current stack file path
		baseDir := filepath.Dir(normalizedBasePath)
		relativePath := filepath.Join(baseDir, normalizedPath)
		// Return in original format, OS-specific
		return filepath.FromSlash(relativePath)
	}

	// For non-relative paths, return as-is in original format
	return path
}

// GetLineEnding returns the appropriate line ending for the current platform.
func GetLineEnding() string {
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}
