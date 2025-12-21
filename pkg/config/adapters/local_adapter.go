package adapters

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// LocalAdapter handles local filesystem imports.
// It is registered as the default adapter and handles paths without URL schemes.
type LocalAdapter struct{}

// Schemes returns nil because LocalAdapter is the default fallback.
// It handles any path that doesn't match other adapters' schemes.
func (l *LocalAdapter) Schemes() []string {
	return nil
}

// Resolve processes a local filesystem import path.
//
//nolint:revive // argument-limit: matches ImportAdapter interface signature.
func (l *LocalAdapter) Resolve(
	_ context.Context, importPath, basePath, tempDir string, currentDepth, maxDepth int,
) ([]config.ResolvedPaths, error) {
	defer perf.Track(nil, "adapters.LocalAdapter.Resolve")()

	if importPath == "" {
		return nil, errUtils.ErrImportPathRequired
	}

	resolvedPath := l.resolveImportPath(importPath, basePath)
	paths, err := config.SearchAtmosConfig(resolvedPath)
	if err != nil {
		log.Debug("failed to resolve local import path", "path", importPath, "err", err)
		return nil, errUtils.ErrResolveLocal
	}

	resolvedPaths := make([]config.ResolvedPaths, 0)
	for _, path := range paths {
		v := viper.New()
		v.SetConfigFile(path)
		v.SetConfigType("yaml")
		if err := v.ReadInConfig(); err != nil {
			log.Debug("failed to load local config", "path", path, "error", err)
			continue
		}

		resolvedPaths = append(resolvedPaths, config.ResolvedPaths{
			FilePath: path, ImportPaths: importPath, ImportType: config.LOCAL,
		})

		nestedPaths := l.processNestedImports(v, nestedImportParams{
			basePath: basePath, tempDir: tempDir, currentDepth: currentDepth, maxDepth: maxDepth, path: path,
		})
		resolvedPaths = append(resolvedPaths, nestedPaths...)
	}

	return resolvedPaths, nil
}

// resolveImportPath converts a relative import path to absolute.
func (l *LocalAdapter) resolveImportPath(importPath, basePath string) string {
	resolvedPath := importPath
	if !filepath.IsAbs(importPath) {
		resolvedPath = filepath.Join(basePath, importPath)
	}
	if !strings.HasPrefix(filepath.Clean(resolvedPath), filepath.Clean(basePath)) {
		log.Trace("Import path is outside of base directory", "importPath", resolvedPath, "basePath", basePath)
	}
	return resolvedPath
}

// nestedImportParams holds parameters for processing nested imports.
type nestedImportParams struct {
	basePath, tempDir      string
	currentDepth, maxDepth int
	path                   string
}

// processNestedImports handles nested import statements in a config file.
func (l *LocalAdapter) processNestedImports(v *viper.Viper, p nestedImportParams) []config.ResolvedPaths {
	imports := v.GetStringSlice("import")
	if len(imports) == 0 {
		return nil
	}
	importBasePath := v.GetString("base_path")
	if importBasePath == "" {
		importBasePath = p.basePath
	}
	nestedPaths, err := config.ProcessImportsFromAdapter(importBasePath, imports, p.tempDir, p.currentDepth+1, p.maxDepth)
	if err != nil {
		log.Debug("failed to process nested imports from", "path", p.path, "error", err)
		return nil
	}
	return nestedPaths
}
