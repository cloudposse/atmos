package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// getStackFilePatterns returns a slice of possible file patterns for a given base path.
func getStackFilePatterns(basePath string, includeTemplates bool) []string {
	patterns := []string{
		basePath + u.YamlFileExtension,
		basePath + u.YmlFileExtension,
	}

	if includeTemplates {
		patterns = append(
			patterns,
			basePath+u.YamlTemplateExtension,
			basePath+u.YmlTemplateExtension,
		)
	}

	return patterns
}

// matchesStackFilePattern checks if a file path matches any of the valid stack file patterns.
func matchesStackFilePattern(filePath, stackName string) bool {
	defer perf.Track(nil, "config.matchesStackFilePattern")()

	// Always include template files for normal operations (imports, etc.)
	patterns := getStackFilePatterns(stackName, true)
	for _, pattern := range patterns {
		if strings.HasSuffix(filePath, pattern) {
			return true
		}
	}
	return false
}

func getConfigFilePatterns(path string, forGlobMatch bool) []string {
	defer perf.Track(nil, "config.getConfigFilePatterns")()

	if path == "" {
		return []string{}
	}
	ext := filepath.Ext(path)
	if ext != "" {
		return []string{path}
	}

	// include template files for normal operations
	patterns := getStackFilePatterns(path, true)
	if !forGlobMatch {
		// For direct file search, include the exact path without extension
		patterns = append([]string{path}, patterns...)
	}

	return patterns
}

func SearchConfigFile(configPath string, atmosConfig *schema.AtmosConfiguration) (string, error) {
	// If path already has an extension, verify it exists
	if ext := filepath.Ext(configPath); ext != "" {
		if _, err := os.Stat(configPath); err == nil {
			return configPath, nil
		}
		return "", fmt.Errorf("%w: %s", errSpecifiedConfigNotFound, configPath)
	}

	dir := filepath.Dir(configPath)
	base := filepath.Base(configPath)

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("error reading directory %s: %w", dir, err)
	}

	// Create a map of existing files for quick lookup
	fileMap := make(map[string]bool)
	for _, entry := range entries {
		if !entry.IsDir() {
			fileMap[entry.Name()] = true
		}
	}

	// Try all patterns in order
	patterns := getConfigFilePatterns(base, false)
	for _, pattern := range patterns {
		if fileMap[pattern] {
			return filepath.Join(dir, pattern), nil
		}
	}

	return "", fmt.Errorf("%w '%s' ('%s' + '%s')", errImportMatchNotFound, configPath, dir, base)
}
