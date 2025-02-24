package config

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/hashicorp/go-getter"
	"github.com/pkg/errors"
	"github.com/spf13/viper"
)

var (
	ErrBasePath           = errors.New("base path required to process imports")
	ErrTempDir            = errors.New("temporary directory required to process imports")
	ErrResolveLocal       = errors.New("failed to resolve local import path")
	ErrSourceDestination  = errors.New("source and destination cannot be nil")
	ErrImportPathRequired = errors.New("import path required to process imports")
	ErrNOFileMatchPattern = errors.New("no files matching patterns found")
)

type importTypes int

const (
	LOCAL  importTypes = 0
	REMOTE importTypes = 1
)

// import Resolved Paths.
type ResolvedPaths struct {
	filePath    string
	importPaths string // import path from atmos config
	importType  importTypes
}

// processConfigImports It reads the import paths from the source configuration.
// It processes imports from the source configuration and merges them into the destination configuration.
func processConfigImports(source *schema.AtmosConfiguration, dst *viper.Viper) error {
	if source == nil || dst == nil {
		return ErrSourceDestination
	}
	if len(source.Import) == 0 {
		return nil
	}
	importPaths := source.Import
	baseBath, err := filepath.Abs(source.BasePath)
	if err != nil {
		return err
	}
	tempDir, err := os.MkdirTemp("", "atmos-import-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDir)
	resolvedPaths, err := processImports(baseBath, importPaths, tempDir, 1, MaximumImportLvL)
	if err != nil {
		return err
	}

	for _, resolvedPath := range resolvedPaths {
		err := mergeConfigFile(resolvedPath.filePath, dst)
		if err != nil {
			log.Debug("error loading config file", "import", resolvedPath.importPaths, "file_path", resolvedPath.filePath, "error", err)
			continue
		}
		log.Debug("merged config from import", "import", resolvedPath.importPaths, "file_path", resolvedPath.filePath)
	}

	return nil
}

func processImports(basePath string, importPaths []string, tempDir string, currentDepth, maxDepth int) (resolvedPaths []ResolvedPaths, err error) {
	if basePath == "" {
		return nil, ErrBasePath
	}
	if tempDir == "" {
		return nil, ErrTempDir
	}
	if currentDepth > maxDepth {
		return nil, errors.New("maximum import depth reached")
	}
	basePath, err = filepath.Abs(basePath)
	if err != nil {
		log.Debug("failed to get absolute path for base path", "path", basePath, "error", err)
		return nil, err
	}

	for _, importPath := range importPaths {
		if importPath == "" {
			continue
		}

		if isRemoteImport(importPath) {
			// Handle remote imports
			paths, err := processRemoteImport(basePath, importPath, tempDir, currentDepth, maxDepth)
			if err != nil {
				log.Debug("failed to process remote import", "path", importPath, "error", err)
				continue
			}
			resolvedPaths = append(resolvedPaths, paths...)
		} else {
			// Handle local imports
			paths, err := processLocalImport(basePath, importPath, tempDir, currentDepth, maxDepth)
			if err != nil {
				log.Debug("failed to process local import", "path", importPath, "error", err)
				continue
			}
			resolvedPaths = append(resolvedPaths, paths...)
		}
	}

	return resolvedPaths, nil
}

// Helper to determine if the import is a supported remote source.
func isRemoteImport(importPath string) bool {
	return strings.HasPrefix(importPath, "http://") || strings.HasPrefix(importPath, "https://")
}

// Process remote imports.
func processRemoteImport(basePath, importPath, tempDir string, currentDepth, maxDepth int) ([]ResolvedPaths, error) {
	parsedURL, err := url.Parse(importPath)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		log.Debug("unsupported URL", "URL", importPath, "error", err)
		return nil, err
	}

	tempFile, err := downloadRemoteConfig(parsedURL.String(), tempDir)
	if err != nil {
		log.Debug("failed to download remote config", "path", importPath, "error", err)
		return nil, err
	}
	v := viper.New()
	v.SetConfigFile(tempFile)
	err = v.ReadInConfig()
	if err != nil {
		log.Debug("failed to read remote config", "path", importPath, "error", err)
		return nil, err
	}

	resolvedPaths := make([]ResolvedPaths, 0)
	resolvedPaths = append(resolvedPaths, ResolvedPaths{
		filePath:    tempFile,
		importPaths: importPath,
		importType:  REMOTE,
	})
	imports := v.GetStringSlice("import")
	importBasePath := v.GetString("base_path")
	if importBasePath == "" {
		importBasePath = basePath
	}

	// Recursively process imports from the remote file
	if len(imports) > 0 {
		nestedPaths, err := processImports(importBasePath, imports, tempDir, currentDepth+1, maxDepth)
		if err != nil {
			log.Debug("failed to process nested imports", "import", importPath, "err", err)
			return nil, err
		}
		resolvedPaths = append(resolvedPaths, nestedPaths...)
	}

	return resolvedPaths, nil
}

// Process local imports.
func processLocalImport(basePath string, importPath, tempDir string, currentDepth, maxDepth int) ([]ResolvedPaths, error) {
	if importPath == "" {
		return nil, ErrImportPathRequired
	}
	if !filepath.IsAbs(importPath) {
		importPath = filepath.Join(basePath, importPath)
	}
	if !strings.HasPrefix(filepath.Clean(importPath), filepath.Clean(basePath)) {
		log.Warn("Import path is outside of base directory",
			"importPath", importPath,
			"basePath", basePath,
		)
	}
	paths, err := SearchAtmosConfig(importPath)
	if err != nil {
		log.Debug("failed to resolve local import path", "path", importPath, "err", err)
		return nil, ErrResolveLocal
	}

	resolvedPaths := make([]ResolvedPaths, 0)
	// Load the local configuration file to check for further imports
	for _, path := range paths {
		v := viper.New()
		v.SetConfigFile(path)
		v.SetConfigType("yaml")
		err := v.ReadInConfig()
		if err != nil {
			log.Debug("failed to load local config", "path", path, "error", err)
			continue
		}
		resolvedPaths = append(resolvedPaths, ResolvedPaths{
			filePath:    path,
			importPaths: importPath,
			importType:  LOCAL,
		})
		imports := v.GetStringSlice("import")
		importBasePath := v.GetString("base_path")
		if importBasePath == "" {
			importBasePath = basePath
		}

		// Recursively process imports from the local file
		if len(imports) > 0 {
			nestedPaths, err := processImports(importBasePath, imports, tempDir, currentDepth+1, maxDepth)
			if err != nil {
				log.Debug("failed to process nested imports from", "path", path, "error", err)
				continue
			}
			resolvedPaths = append(resolvedPaths, nestedPaths...)
		}
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
		return nil, errors.New("no valid absolute paths found")
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
			log.Debug("no matches found for glob pattern", "path", pattern, "error", err)
			continue
		}
		filePaths = append(filePaths, matches...)
	}

	if len(filePaths) == 0 {
		return nil, ErrNOFileMatchPattern
	}

	return filePaths, nil
}

func downloadRemoteConfig(url string, tempDir string) (string, error) {
	// uniq name for the temp file
	fileName := fmt.Sprintf("atmos-import-%d.yaml", time.Now().UnixNano())
	tempFile := filepath.Join(tempDir, fileName)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := &getter.Client{
		Ctx:  ctx,
		Src:  url,
		Dst:  tempFile,
		Mode: getter.ClientModeFile,
	}
	err := client.Get()
	if err != nil {
		os.RemoveAll(tempFile)
		return "", fmt.Errorf("failed to download remote config: %w", err)
	}
	return tempFile, nil
}
