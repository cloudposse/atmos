package config

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/hashicorp/go-getter"
	"github.com/spf13/viper"
)

// import Resolved Paths
type ResolvedPaths struct {
	filePath    string // path to the resolved config file
	importPaths string // import path from atmos config
}

func (cl *ConfigLoader) downloadRemoteConfig(url string, tempDir string) (string, error) {
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

func (cl *ConfigLoader) processImports(importPaths []string, tempDir string, currentDepth, maxDepth int) (resolvedPaths []ResolvedPaths, err error) {
	if tempDir == "" {
		return nil, fmt.Errorf("tempDir required to process imports")
	}
	if currentDepth > maxDepth {
		return nil, fmt.Errorf("maximum import depth of %d exceeded", maxDepth)
	}

	for _, importPath := range importPaths {
		if importPath == "" {
			continue
		}

		if isRemoteImport(importPath) {
			// Handle remote imports
			paths, err := cl.processRemoteImport(importPath, tempDir, currentDepth, maxDepth)
			if err != nil {
				log.Debug("failed to process remote import", "path", importPath, "error", err)
				continue
			}
			resolvedPaths = append(resolvedPaths, paths...)
		} else {
			// Handle local imports
			paths, err := cl.processLocalImport(importPath, tempDir, currentDepth, maxDepth)
			if err != nil {
				log.Debug("failed to process local import", "path", importPath, "error", err)
				continue
			}
			resolvedPaths = append(resolvedPaths, paths...)
		}
	}

	return resolvedPaths, nil
}

// Helper to determine if the import path is remote
func isRemoteImport(importPath string) bool {
	return strings.HasPrefix(importPath, "http://") || strings.HasPrefix(importPath, "https://")
}

// Process remote imports
func (cl *ConfigLoader) processRemoteImport(importPath, tempDir string, currentDepth, maxDepth int) ([]ResolvedPaths, error) {
	parsedURL, err := url.Parse(importPath)
	if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return nil, fmt.Errorf("unsupported URL '%s': %v", importPath, err)
	}

	tempFile, err := cl.downloadRemoteConfig(importPath, tempDir)
	if err != nil {
		return nil, fmt.Errorf("failed to download remote config '%s': %v", importPath, err)
	}

	v := viper.New()
	v.SetConfigType("yaml")
	err = cl.loadConfigFileViber(cl.atmosConfig, tempFile, v)
	if err != nil {
		return nil, fmt.Errorf("failed to load remote config '%s': %v", importPath, err)
	}

	var importedConfig schema.AtmosConfiguration
	if err := v.Unmarshal(&importedConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal remote config '%s': %v", importPath, err)
	}

	resolvedPaths := make([]ResolvedPaths, 0)
	resolvedPaths = append(resolvedPaths, ResolvedPaths{
		filePath:    tempFile,
		importPaths: importPath,
	})

	// Recursively process imports from the remote file
	if importedConfig.Import != nil {
		nestedPaths, err := cl.processImports(importedConfig.Import, tempDir, currentDepth+1, maxDepth)
		if err != nil {
			return nil, fmt.Errorf("failed to process nested imports from '%s': %v", importPath, err)
		}
		resolvedPaths = append(resolvedPaths, nestedPaths...)
	}

	return resolvedPaths, nil
}

// Process local imports
func (cl *ConfigLoader) processLocalImport(importPath, tempDir string, currentDepth, maxDepth int) ([]ResolvedPaths, error) {
	basePath, err := filepath.Abs(cl.atmosConfig.BasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve base path: %v", err)
	}

	localPath := filepath.Join(basePath, importPath)
	if !strings.HasPrefix(filepath.Clean(localPath), filepath.Clean(basePath)) {
		log.Warn("Import path is outside of base directory",
			"importPath", importPath,
			"basePath", basePath,
		)
	}
	paths, err := cl.SearchAtmosConfigFileDir(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve local import path '%s': %v", importPath, err)
	}

	resolvedPaths := make([]ResolvedPaths, 0)
	for _, path := range paths {
		resolvedPaths = append(resolvedPaths, ResolvedPaths{
			filePath:    path,
			importPaths: importPath,
		})
	}

	// Load the local configuration file to check for further imports
	for _, path := range paths {
		v := viper.New()
		v.SetConfigType("yaml")
		err := cl.loadConfigFileViber(cl.atmosConfig, path, v)
		if err != nil {
			log.Debug("failed to load local config", "path", path, "error", err)
			continue
		}

		var importedConfig schema.AtmosConfiguration
		if err := v.Unmarshal(&importedConfig); err != nil {
			log.Debug("failed to unmarshal local config", "path", path, "error", err)
			continue
		}

		// Recursively process imports from the local file
		if importedConfig.Import != nil {
			nestedPaths, err := cl.processImports(importedConfig.Import, tempDir, currentDepth+1, maxDepth)
			if err != nil {
				log.Debug("failed to process nested imports from", "path", path, "error", err)
				continue
			}
			resolvedPaths = append(resolvedPaths, nestedPaths...)
		}
	}

	return resolvedPaths, nil
}
