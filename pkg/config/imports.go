package config

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/hashicorp/go-getter"
	"github.com/spf13/viper"
)

func (cl *ConfigLoader) downloadRemoteConfig(url string, tempDir string) (string, error) {
	// uniq name for the temp file
	fileName := fmt.Sprintf("atmos-%d.yaml", time.Now().UnixNano())
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
		os.RemoveAll(tempDir)
		return "", fmt.Errorf("failed to download remote config: %w", err)
	}
	return tempFile, nil
}
func (cl *ConfigLoader) processImports(importPaths []string, tempDir string, currentDepth, maxDepth int) (resolvedPaths []string, err error) {
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
				u.LogWarning(cl.atmosConfig, fmt.Sprintf("failed to process remote import '%s': %v", importPath, err))
				continue
			}
			resolvedPaths = append(resolvedPaths, paths...)
		} else {
			// Handle local imports
			paths, err := cl.processLocalImport(importPath, tempDir, currentDepth, maxDepth)
			if err != nil {
				u.LogWarning(cl.atmosConfig, fmt.Sprintf("failed to process local import '%s': %v", importPath, err))
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
func (cl *ConfigLoader) processRemoteImport(importPath, tempDir string, currentDepth, maxDepth int) ([]string, error) {
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
	found, _, err := cl.loadConfigFile(cl.atmosConfig, tempFile, v)
	if err != nil || !found {
		return nil, fmt.Errorf("failed to load remote config '%s': %v", importPath, err)
	}

	var importedConfig schema.AtmosConfiguration
	if err := v.Unmarshal(&importedConfig); err != nil {
		return nil, fmt.Errorf("failed to unmarshal remote config '%s': %v", importPath, err)
	}

	resolvedPaths := []string{tempFile}

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
func (cl *ConfigLoader) processLocalImport(importPath, tempDir string, currentDepth, maxDepth int) ([]string, error) {
	basePath, err := filepath.Abs(cl.atmosConfig.BasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve base path: %v", err)
	}

	localPath := filepath.Join(basePath, importPath)
	paths, err := cl.SearchAtmosConfigFileDir(localPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve local import path '%s': %v", importPath, err)
	}

	resolvedPaths := paths

	// Load the local configuration file to check for further imports
	for _, path := range paths {
		v := viper.New()
		v.SetConfigType("yaml")
		found, _, err := cl.loadConfigFile(cl.atmosConfig, path, v)
		if err != nil || !found {
			u.LogWarning(cl.atmosConfig, fmt.Sprintf("failed to load local config '%s': %v", path, err))
			continue
		}

		var importedConfig schema.AtmosConfiguration
		if err := v.Unmarshal(&importedConfig); err != nil {
			u.LogWarning(cl.atmosConfig, fmt.Sprintf("failed to unmarshal local config '%s': %v", path, err))
			continue
		}

		// Recursively process imports from the local file
		if importedConfig.Import != nil {
			nestedPaths, err := cl.processImports(importedConfig.Import, tempDir, currentDepth+1, maxDepth)
			if err != nil {
				u.LogWarning(cl.atmosConfig, fmt.Sprintf("failed to process nested imports from '%s': %v", path, err))
				continue
			}
			resolvedPaths = append(resolvedPaths, nestedPaths...)
		}
	}

	return resolvedPaths, nil
}
