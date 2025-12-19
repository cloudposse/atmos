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
func (l *LocalAdapter) Resolve(
	ctx context.Context,
	importPath string,
	basePath string,
	tempDir string,
	currentDepth int,
	maxDepth int,
) ([]config.ResolvedPaths, error) {
	defer perf.Track(nil, "adapters.LocalAdapter.Resolve")()

	if importPath == "" {
		return nil, errUtils.ErrImportPathRequired
	}

	// Make path absolute relative to basePath.
	resolvedPath := importPath
	if !filepath.IsAbs(importPath) {
		resolvedPath = filepath.Join(basePath, importPath)
	}

	// Log if import path is outside base directory (allowed but noteworthy).
	if !strings.HasPrefix(filepath.Clean(resolvedPath), filepath.Clean(basePath)) {
		log.Trace("Import path is outside of base directory",
			"importPath", resolvedPath,
			"basePath", basePath,
		)
	}

	// Search for matching config files.
	paths, err := config.SearchAtmosConfig(resolvedPath)
	if err != nil {
		log.Debug("failed to resolve local import path", "path", importPath, "err", err)
		return nil, errUtils.ErrResolveLocal
	}

	resolvedPaths := make([]config.ResolvedPaths, 0)

	// Process each matched file.
	for _, path := range paths {
		v := viper.New()
		v.SetConfigFile(path)
		v.SetConfigType("yaml")
		err := v.ReadInConfig()
		if err != nil {
			log.Debug("failed to load local config", "path", path, "error", err)
			continue
		}

		resolvedPaths = append(resolvedPaths, config.ResolvedPaths{
			FilePath:    path,
			ImportPaths: importPath,
			ImportType:  config.LOCAL,
		})

		// Check for nested imports.
		imports := v.GetStringSlice("import")
		importBasePath := v.GetString("base_path")
		if importBasePath == "" {
			importBasePath = basePath
		}

		// Recursively process nested imports.
		if len(imports) > 0 {
			nestedPaths, err := config.ProcessImportsFromAdapter(importBasePath, imports, tempDir, currentDepth+1, maxDepth)
			if err != nil {
				log.Debug("failed to process nested imports from", "path", path, "error", err)
				continue
			}
			resolvedPaths = append(resolvedPaths, nestedPaths...)
		}
	}

	return resolvedPaths, nil
}
