package config

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// testLocalAdapter is a minimal local adapter for testing.
// It implements the same logic as adapters.LocalAdapter but without
// importing the adapters package to avoid circular imports.
type testLocalAdapter struct{}

func (l *testLocalAdapter) Schemes() []string {
	return nil
}

func (l *testLocalAdapter) Resolve(
	_ context.Context,
	importPath string,
	basePath string,
	tempDir string,
	currentDepth int,
	maxDepth int,
) ([]ResolvedPaths, error) {
	if importPath == "" {
		return nil, errUtils.ErrImportPathRequired
	}

	// Make path absolute relative to basePath.
	resolvedPath := importPath
	if !filepath.IsAbs(importPath) {
		resolvedPath = filepath.Join(basePath, importPath)
	}

	// Log if import path is outside base directory.
	if !strings.HasPrefix(filepath.Clean(resolvedPath), filepath.Clean(basePath)) {
		log.Trace("Import path is outside of base directory",
			"importPath", resolvedPath,
			"basePath", basePath,
		)
	}

	// Search for matching config files.
	paths, err := SearchAtmosConfig(resolvedPath)
	if err != nil {
		log.Debug("failed to resolve local import path", "path", importPath, "err", err)
		return nil, errUtils.ErrResolveLocal
	}

	resolvedPaths := make([]ResolvedPaths, 0)

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

		resolvedPaths = append(resolvedPaths, ResolvedPaths{
			FilePath:    path,
			ImportPaths: importPath,
			ImportType:  LOCAL,
		})

		// Check for nested imports.
		imports := v.GetStringSlice("import")
		importBasePath := v.GetString("base_path")
		if importBasePath == "" {
			importBasePath = basePath
		}

		// Recursively process nested imports.
		if len(imports) > 0 {
			nestedPaths, err := ProcessImportsFromAdapter(importBasePath, imports, tempDir, currentDepth+1, maxDepth)
			if err != nil {
				log.Debug("failed to process nested imports from", "path", path, "error", err)
				continue
			}
			resolvedPaths = append(resolvedPaths, nestedPaths...)
		}
	}

	return resolvedPaths, nil
}

// setupTestAdapters registers minimal test adapters for unit testing.
// This must be called at the start of tests that use processImports.
func setupTestAdapters() {
	ResetImportAdapterRegistry()
	SetDefaultAdapter(&testLocalAdapter{})
}
