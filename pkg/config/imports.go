package config

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/condition"
	"github.com/cloudposse/atmos/pkg/filesystem"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/policy/guard"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// SanitizeImport redacts credentials and query values from URLs while leaving paths intact.
// Sanitizes credentials from any URL scheme (http, https, git, ssh, s3, gcs, oci, etc.).
// Exported so other config-loading packages (e.g. pkg/config/adapters) can sanitize import
// paths before logging them.
func SanitizeImport(s string) string {
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

// keyFilePath is the structured-log field name for a resolved import's file path.
const keyFilePath = "file_path"

var defaultFileSystem filesystem.FileSystem = filesystem.NewOSFileSystem()

// ResolvedPaths represents a resolved import path with its file location and type.
type ResolvedPaths struct {
	FilePath    string      // Absolute path to the resolved file.
	ImportPaths string      // Original import path from atmos config.
	ImportType  importTypes // Type of import (LOCAL, REMOTE, or ADAPTER).
	// SkipIfMissing carries the originating import's failure intent so that a merge/re-read
	// failure fails closed for a required import and warn-and-skips for an optional one,
	// matching how resolve failures are handled in processImports.
	SkipIfMissing bool
}

// processConfigImports It reads the import paths from the source configuration.
// It processes imports from the source configuration and merges them into the destination configuration.
func processConfigImports(source *schema.AtmosConfiguration, dst *viper.Viper) error {
	_, err := processConfigImportsWithBasePathSource(source, dst)
	return err
}

// processConfigImportsWithFS processes imports using a FileSystem implementation.
func processConfigImportsWithFS(source *schema.AtmosConfiguration, dst *viper.Viper, fs filesystem.FileSystem) error {
	_, err := processConfigImportsWithFSAndBasePathSource(source, dst, fs)
	return err
}

func processConfigImportsWithBasePathSource(source *schema.AtmosConfiguration, dst *viper.Viper) (string, error) {
	return processConfigImportsWithFSAndBasePathSource(source, dst, defaultFileSystem)
}

func processConfigImportsWithFSAndBasePathSource(source *schema.AtmosConfiguration, dst *viper.Viper, fs filesystem.FileSystem) (string, error) {
	if source == nil || dst == nil {
		return "", errUtils.ErrSourceDestination
	}
	if len(source.Import) == 0 {
		return "", nil
	}
	importPaths := source.Import
	tempDir, err := fs.MkdirTemp("", "atmos-import-*")
	if err != nil {
		return "", err
	}
	defer func() {
		if err := fs.RemoveAll(tempDir); err != nil {
			log.Debug("Failed to remove temp directory", "path", tempDir, "error", err)
		}
	}()
	// processImports resolves source.BasePath to an absolute path itself and rejects
	// an empty base path, so no redundant filepath.Abs is needed here.
	resolvedPaths, err := processImports(source, source.BasePath, importPaths, tempDir, 1, MaximumImportLvL)
	if err != nil {
		return "", err
	}

	log.Debug("processConfigImports resolved paths", "count", len(resolvedPaths))

	return mergeResolvedImports(resolvedPaths, dst, fs)
}

// mergeResolvedImports merges each resolved import file into dst and returns the
// directory of the last imported file that declares a base_path (empty if none).
// An optional import (SkipIfMissing) that fails to merge or re-read is skipped so a single
// bad import does not abort the whole configuration load; a required import that fails to
// merge returns errUtils.ErrRequiredImportFailed (fail closed).
func mergeResolvedImports(resolvedPaths []ResolvedPaths, dst *viper.Viper, fs filesystem.FileSystem) (string, error) {
	basePathSourceDir := ""
	for _, resolvedPath := range resolvedPaths {
		// Trace: log what we're about to merge (sanitized).
		log.Trace("attempting to merge import", "import", SanitizeImport(resolvedPath.ImportPaths), keyFilePath, resolvedPath.FilePath)
		err := mergeConfigFile(resolvedPath.FilePath, dst)
		if err != nil {
			if isPolicyError(err) {
				return "", err
			}
			if !resolvedPath.SkipIfMissing {
				return "", fmt.Errorf("%w: %s: %w", errUtils.ErrRequiredImportFailed, SanitizeImport(resolvedPath.ImportPaths), err)
			}
			log.Warn("error loading config file", "import", SanitizeImport(resolvedPath.ImportPaths), keyFilePath, resolvedPath.FilePath, "error", err)
			continue
		}
		log.Trace("successfully merged config from import", "import", SanitizeImport(resolvedPath.ImportPaths), keyFilePath, resolvedPath.FilePath)
		content, readErr := fs.ReadFile(resolvedPath.FilePath)
		if readErr != nil {
			log.Warn("error re-reading resolved import for base_path detection", "import", SanitizeImport(resolvedPath.ImportPaths), keyFilePath, resolvedPath.FilePath, "error", readErr)
			continue
		}
		declaresBasePath, _, parseErr := importBasePathDeclaration(content)
		if parseErr == nil && declaresBasePath {
			basePathSourceDir = filepath.Dir(resolvedPath.FilePath)
		}
	}
	return basePathSourceDir, nil
}

// ProcessImportsFromAdapter is the public entry point for adapters to process nested imports.
// Adapters should call this when they discover nested import statements in resolved configs.
// The atmosConfig parameter is threaded through so nested remote imports honor auth settings
// (GitHub token injection and the Atmos Pro credential broker); it may be nil.
//
// Each entry is a schema.ConfigImport, so nested imports honor the same per-import controls
// (skip_if_missing, when) as top-level CLI-config imports. Adapters that read a raw string
// list from Viper should convert it with NormalizeConfigImports first.
//
//nolint:revive // argument-limit: atmosConfig threaded through for remote-import auth.
func ProcessImportsFromAdapter(atmosConfig *schema.AtmosConfiguration, basePath string, imports []schema.ConfigImport, tempDir string, currentDepth, maxDepth int) ([]ResolvedPaths, error) {
	return processImports(atmosConfig, basePath, imports, tempDir, currentDepth, maxDepth)
}

//nolint:revive // argument-limit: atmosConfig threaded through for remote-import auth.
func processImports(atmosConfig *schema.AtmosConfiguration, basePath string, imports []schema.ConfigImport, tempDir string, currentDepth, maxDepth int) (resolvedPaths []ResolvedPaths, err error) {
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
	// Facts for evaluating each import's optional `when` condition. At config-load time the
	// subcommand is not yet known, so the available facts are CI detection (the `ci`/`local`
	// predicates) and the process environment (exposed to CEL as the `env` map, e.g.
	// `when: !cel 'env.DEPLOY_ENV == "prod"'`); see pkg/condition.
	whenCtx := condition.Context{CI: ci.IsCI(), Env: condition.OSEnv()}

	for _, imp := range imports {
		if imp.Path == "" {
			continue
		}

		// Conditional loading: skip an import whose `when` condition does not match the current
		// context (e.g. `when: local` on a CI runner). An unset condition always imports.
		if !imp.When.IsZero() && !imp.When.Evaluate(whenCtx) {
			log.Debug("skipping config import (when condition not met)", "path", SanitizeImport(imp.Path))
			continue
		}

		paths, resolveErr := resolveSingleImport(ctx, atmosConfig, imp, basePath, tempDir, currentDepth, maxDepth)
		if resolveErr != nil {
			if err := classifyImportResolveError(imp, resolveErr); err != nil {
				return nil, err
			}
			continue
		}

		// Stamp this import's failure intent onto the files it produced, so a merge failure later
		// fails closed for a required import and warn-and-skips for an optional one. A nested
		// import's own resolve-time intent is already applied in its deeper processImports pass;
		// at merge time the importing entry's intent governs its resolved subtree.
		for i := range paths {
			paths[i].SkipIfMissing = imp.SkipIfMissing
		}
		resolvedPaths = append(resolvedPaths, paths...)
	}

	return resolvedPaths, nil
}

// classifyImportResolveError decides how a resolve failure is treated. It returns a non-nil error
// to propagate (fail closed) and nil to warn-and-skip an optional import. Three classes always
// propagate regardless of this (parent) import's optionality, so an optional parent cannot mask
// them: policy denials, a required nested failure already surfaced by a deeper pass
// (ErrRequiredImportFailed), and a malformed import entry (ErrInvalidConfigImport) - a decode
// failure must not be silently converted into an optional omission. Otherwise a required
// (non-optional) import fails closed and an optional one warns and continues.
func classifyImportResolveError(imp schema.ConfigImport, resolveErr error) error {
	if guard.IsPolicyError(resolveErr) ||
		errors.Is(resolveErr, errUtils.ErrRequiredImportFailed) ||
		errors.Is(resolveErr, errUtils.ErrInvalidConfigImport) {
		return resolveErr
	}
	if !imp.SkipIfMissing {
		return fmt.Errorf("%w: %s: %w", errUtils.ErrRequiredImportFailed, SanitizeImport(imp.Path), resolveErr)
	}
	log.Warn("failed to resolve import", "path", SanitizeImport(imp.Path), "error", resolveErr)
	return nil
}

// resolveSingleImport applies the source-admission policy gate (for scheme-based adapters) and
// resolves one import via its registered adapter.
//
//nolint:revive // argument-limit: mirrors the adapter Resolve signature this delegates to.
func resolveSingleImport(ctx context.Context, atmosConfig *schema.AtmosConfiguration, imp schema.ConfigImport, basePath, tempDir string, currentDepth, maxDepth int) ([]ResolvedPaths, error) {
	// Use the adapter registry to find the appropriate adapter.
	adapter := FindImportAdapter(imp.Path)
	if len(adapter.Schemes()) > 0 {
		if gateErr := guard.Source(ctx, "import", imp.Path, basePath); gateErr != nil {
			return nil, gateErr
		}
	}
	return adapter.Resolve(ctx, imp.Path, basePath, tempDir, currentDepth, maxDepth, atmosConfig)
}

// NormalizeConfigImports converts a raw decoded `import:` value (as returned by viper.Get) into
// ConfigImports, accepting both the legacy string-list form and the new object form. This is the
// counterpart to schema.ConfigImportsDecodeHook for code paths that read the `import:` key as a
// raw Viper value (adapters processing nested imports, profile loading) rather than unmarshaling
// into a typed struct. A bare string becomes an optional (fail-open) import, preserving the
// historical warn-and-skip behavior for backward compatibility.
func NormalizeConfigImports(raw any, configs ...*schema.AtmosConfiguration) (schema.ConfigImports, error) {
	var config *schema.AtmosConfiguration
	if len(configs) > 0 {
		config = configs[0]
	}
	strict, err := u.ExplicitYAMLTags(config)
	if err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, nil
	}
	if typed, ok := raw.(schema.ConfigImports); ok {
		return typed, nil
	}

	items := rawImportItems(raw)
	result := make(schema.ConfigImports, 0, len(items))
	for _, item := range items {
		imp, ok, err := configImportFromItem(item, strict)
		if err != nil {
			return nil, err
		}
		if ok {
			result = append(result, imp)
		}
	}
	return result, nil
}

// rawImportItems coerces a raw decoded `import:` value into a slice of entries to normalize,
// accepting a []any (mixed strings/objects), a []string (legacy form), or a single scalar.
func rawImportItems(raw any) []any {
	switch v := raw.(type) {
	case []any:
		return v
	case []string:
		items := make([]any, 0, len(v))
		for _, s := range v {
			items = append(items, s)
		}
		return items
	default:
		return []any{v}
	}
}

// configImportFromItem converts one raw import entry (a bare string or a decoded object map) into
// a ConfigImport. A bare string becomes an optional (fail-open) import. The returned bool is false
// for an entry that is intentionally skipped (an empty string or an empty path). A decode failure
// returns errUtils.ErrInvalidConfigImport so a malformed required entry fails loudly instead of
// being silently dropped and treated as an optional omission.
func configImportFromItem(item any, strict ...bool) (schema.ConfigImport, bool, error) {
	if s, ok := item.(string); ok {
		if s == "" {
			return schema.ConfigImport{}, false, nil
		}
		return schema.ConfigImport{Path: s, SkipIfMissing: true}, true, nil
	}

	var imp schema.ConfigImport
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: getAtmosDecodeHookFunc(strict...),
		Result:     &imp,
	})
	if err != nil {
		return schema.ConfigImport{}, false, fmt.Errorf("%w: %w", errUtils.ErrInvalidConfigImport, err)
	}
	if err := decoder.Decode(item); err != nil {
		return schema.ConfigImport{}, false, fmt.Errorf("%w: %w", errUtils.ErrInvalidConfigImport, err)
	}
	if imp.Path == "" {
		return schema.ConfigImport{}, false, nil
	}
	return imp, true, nil
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

func isPolicyError(err error) bool { return guard.IsPolicyError(err) }
