package config

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// sanitizeImport redacts credentials and query values from URLs while leaving paths intact.
// Sanitizes credentials from any URL scheme (http, https, git, ssh, s3, gcs, oci, etc.).
func sanitizeImport(s string) string {
	// Handle go-getter style URLs with :: separator (e.g., git::https://...).
	parts := strings.SplitN(s, "::", 2)
	var prefix string
	urlPart := s

	if len(parts) == 2 {
		prefix = parts[0] + "::"
		urlPart = parts[1]
	}

	u, err := url.Parse(urlPart)
	if err != nil {
		// Unparsable; return as-is (might be SCP-style git).
		return s
	}

	// Clear credentials regardless of scheme.
	if u.User != nil {
		u.User = nil
	}

	// Clear query params (may contain tokens/credentials).
	if u.RawQuery != "" {
		u.RawQuery = ""
	}

	return prefix + u.String()
}

type importTypes int

const (
	LOCAL   importTypes = 0
	REMOTE  importTypes = 1
	ADAPTER importTypes = 2
)

var defaultFileSystem = filesystem.NewOSFileSystem()

// ResolvedPaths represents a resolved import path with its file location and type.
type ResolvedPaths struct {
	FilePath    string      // Absolute path to the resolved file.
	ImportPaths string      // Original import path from atmos config.
	ImportType  importTypes // Type of import (LOCAL, REMOTE, or ADAPTER).
}

// processConfigImports It reads the import paths from the source configuration.
// It processes imports from the source configuration and merges them into the destination configuration.
func processConfigImports(source *schema.AtmosConfiguration, dst *viper.Viper) error {
	return processConfigImportsWithFS(source, dst, defaultFileSystem)
}

// processConfigImportsWithFS processes imports using a FileSystem implementation.
func processConfigImportsWithFS(source *schema.AtmosConfiguration, dst *viper.Viper, fs filesystem.FileSystem) error {
	if source == nil || dst == nil {
		return errUtils.ErrSourceDestination
	}
	if len(source.Import) == 0 {
		return nil
	}
	importPaths := source.Import
	basePath, err := filepath.Abs(source.BasePath)
	if err != nil {
		return err
	}
	tempDir, err := fs.MkdirTemp("", "atmos-import-*")
	if err != nil {
		return err
	}
	defer func() {
		if err := fs.RemoveAll(tempDir); err != nil {
			log.Debug("Failed to remove temp directory", "path", tempDir, "error", err)
		}
	}()
	resolvedPaths, err := processImports(basePath, importPaths, tempDir, 1, MaximumImportLvL)
	if err != nil {
		return err
	}

	log.Debug("processConfigImports resolved paths", "count", len(resolvedPaths))

	for _, resolvedPath := range resolvedPaths {
		// Trace: log what we're about to merge (sanitized).
		log.Trace("attempting to merge import", "import", sanitizeImport(resolvedPath.ImportPaths), "file_path", resolvedPath.FilePath)
		err := mergeConfigFile(resolvedPath.FilePath, dst)
		if err != nil {
			log.Trace("error loading config file", "import", sanitizeImport(resolvedPath.ImportPaths), "file_path", resolvedPath.FilePath, "error", err)
			continue
		}
		log.Trace("successfully merged config from import", "import", sanitizeImport(resolvedPath.ImportPaths), "file_path", resolvedPath.FilePath)
	}

	return nil
}

// ProcessImportsFromAdapter is the public entry point for adapters to process nested imports.
// Adapters should call this when they discover nested import statements in resolved configs.
func ProcessImportsFromAdapter(basePath string, importPaths []string, tempDir string, currentDepth, maxDepth int) ([]ResolvedPaths, error) {
	return processImports(basePath, importPaths, tempDir, currentDepth, maxDepth)
}

func processImports(basePath string, importPaths []string, tempDir string, currentDepth, maxDepth int) (resolvedPaths []ResolvedPaths, err error) {
	if basePath == "" {
		return nil, errUtils.ErrBasePath
	}
	if tempDir == "" {
		return nil, errUtils.ErrTempDir
	}
	if currentDepth > maxDepth {
		return nil, errUtils.ErrMaxImportDepth
	}
	basePath, err = filepath.Abs(basePath)
	if err != nil {
		log.Debug("failed to get absolute path for base path", "path", basePath, "error", err)
		return nil, err
	}

	ctx := context.Background()

	for _, importPath := range importPaths {
		if importPath == "" {
			continue
		}

		var paths []ResolvedPaths
		var resolveErr error

		// Use the adapter registry to find the appropriate adapter.
		adapter := FindImportAdapter(importPath)
		paths, resolveErr = adapter.Resolve(ctx, importPath, basePath, tempDir, currentDepth, maxDepth)

		if resolveErr != nil {
			log.Debug("failed to resolve import", "path", importPath, "error", resolveErr)
			continue
		}
		resolvedPaths = append(resolvedPaths, paths...)
	}

	return resolvedPaths, nil
}

// SearchAtmosConfig searches for a config file in path. The path is directory, file or a pattern.
func SearchAtmosConfig(path string) ([]string, error) {
	if stat, err := os.Stat(path); err == nil {
		if !stat.IsDir() {
			return []string{path}, nil
		}
	}
	// Generate patterns based on whether path is a directory or a file/pattern
	patterns := generatePatterns(path)

	// Find files matching the patterns
	atmosFilePaths, err := findMatchingFiles(patterns)
	if err != nil {
		return nil, fmt.Errorf("failed to find matching files: %w", err)
	}
	// Convert paths to absolute paths
	atmosFilePathsAbsolute, err := convertToAbsolutePaths(atmosFilePaths)
	if err != nil {
		return nil, fmt.Errorf("failed to convert paths to absolute paths: %w", err)
	}
	// Prioritize and sort files
	atmosFilePathsAbsolute = detectPriorityFiles(atmosFilePathsAbsolute)
	atmosFilePathsAbsolute = sortFilesByDepth(atmosFilePathsAbsolute)
	return atmosFilePathsAbsolute, nil
}

// Helper function to generate search patterns for extension yaml,yml.
func generatePatterns(path string) []string {
	isDir := false
	if stat, err := os.Stat(path); err == nil && stat.IsDir() {
		isDir = true
	}
	if isDir {
		// Search for all .yaml and .yml
		patterns := []string{
			filepath.Join(path, "**", "*.yaml"),
			filepath.Join(path, "**", "*.yml"),
		}
		return patterns
	}
	ext := filepath.Ext(path)
	if ext == "" {
		// If no extension, append .yaml and .yml
		patterns := []string{
			path + ".yaml",
			path + ".yml",
		}
		return patterns
	}
	// If extension is present, use the path as-is
	return []string{path}
}

// Helper function to convert paths to absolute paths.
func convertToAbsolutePaths(filePaths []string) ([]string, error) {
	var absPaths []string
	for _, path := range filePaths {
		absPath, err := filepath.Abs(path)
		if err != nil {
			log.Debug("Error getting absolute path for file", "path", path, "error", err)
			continue
		}
		absPaths = append(absPaths, absPath)
	}

	if len(absPaths) == 0 {
		return nil, errUtils.ErrNoValidAbsolutePaths
	}

	return absPaths, nil
}

// detectPriorityFiles detects which files will have priority. The longer .yaml extensions win over the shorter .yml extensions, if both files exist in the same path.
func detectPriorityFiles(files []string) []string {
	// Map to store the highest priority file for each base name
	priorityMap := make(map[string]string)

	// Iterate through the list of files
	for _, file := range files {
		dir := filepath.Dir(file)
		baseName := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
		ext := filepath.Ext(file)

		// Construct a unique key for the folder + base name
		key := filepath.Join(dir, baseName)

		// Assign .yaml as priority if it exists, or fallback to .yml
		if existingFile, exists := priorityMap[key]; exists {
			if ext == ".yaml" {
				priorityMap[key] = file // Replace .yml with .yaml
			} else if ext == ".yml" && filepath.Ext(existingFile) == ".yaml" {
				continue // Keep .yaml priority
			}
		} else {
			priorityMap[key] = file // First occurrence, add file
		}
	}

	// Collect results from the map
	var result []string
	for _, file := range priorityMap {
		result = append(result, file)
	}

	return result
}

// sortFilesByDepth sorts a list of file paths by the depth of their directories.
// Files with the same depth are sorted alphabetically by name.
func sortFilesByDepth(files []string) []string {
	// Precompute depths for each file path
	type fileDepth struct {
		path  string
		depth int
	}

	var fileDepths []fileDepth
	for _, file := range files {
		cleanPath := filepath.Clean(file)
		dir := filepath.ToSlash(filepath.Dir(cleanPath))
		depth := len(strings.Split(dir, "/"))
		fileDepths = append(fileDepths, fileDepth{path: file, depth: depth})
	}

	// Sort by depth, and alphabetically by name as a tiebreaker
	sort.Slice(fileDepths, func(i, j int) bool {
		if fileDepths[i].depth == fileDepths[j].depth {
			// If depths are the same, compare file names alphabetically
			return fileDepths[i].path < fileDepths[j].path
		}
		// Otherwise, compare by depth
		return fileDepths[i].depth < fileDepths[j].depth
	})

	// Extract sorted paths
	sortedFiles := make([]string, len(fileDepths))
	for i, fd := range fileDepths {
		sortedFiles[i] = fd.path
	}

	return sortedFiles
}

// Helper function to find files matching the patterns.
func findMatchingFiles(patterns []string) ([]string, error) {
	var filePaths []string
	for _, pattern := range patterns {
		matches, err := u.GetGlobMatches(pattern)
		if err != nil {
			continue
		}
		filePaths = append(filePaths, matches...)
	}

	if len(filePaths) == 0 {
		return nil, errUtils.ErrNoFileMatchPattern
	}

	return filePaths, nil
}
