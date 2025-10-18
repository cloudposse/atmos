package exec

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// componentPathPatternCache caches computed path patterns to avoid repeated string concatenation.
// Also caches Terraform module patterns to avoid expensive tfconfig.LoadModule() calls.
type componentPathPatternCache struct {
	patterns       map[string]string   // component:type -> pattern
	modulePatterns map[string][]string // component -> []module patterns
	mu             sync.RWMutex
}

// newComponentPathPatternCache creates a new pattern cache.
func newComponentPathPatternCache() *componentPathPatternCache {
	return &componentPathPatternCache{
		patterns:       make(map[string]string),
		modulePatterns: make(map[string][]string),
	}
}

// getComponentPathPattern returns the cached pattern for a component, or computes and caches it.
func (c *componentPathPatternCache) getComponentPathPattern(
	component string,
	componentType string,
	atmosConfig *schema.AtmosConfiguration,
) (string, error) {
	// Create cache key.
	cacheKey := component + ":" + componentType

	// Check cache with read lock.
	c.mu.RLock()
	if pattern, ok := c.patterns[cacheKey]; ok {
		c.mu.RUnlock()
		return pattern, nil
	}
	c.mu.RUnlock()

	// Compute pattern (not in cache).
	var componentPath string
	switch componentType {
	case cfg.TerraformComponentType:
		componentPath = filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath, component)
	case cfg.HelmfileComponentType:
		componentPath = filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Helmfile.BasePath, component)
	case cfg.PackerComponentType:
		componentPath = filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Packer.BasePath, component)
	default:
		// Unknown component type - return pattern without caching.
		return "", fmt.Errorf("%w: %s", errUtils.ErrUnsupportedComponentType, componentType)
	}

	componentPathAbs, err := filepath.Abs(componentPath)
	if err != nil {
		return "", err
	}

	pattern := componentPathAbs + "/**"

	// Store in cache with write lock.
	c.mu.Lock()
	c.patterns[cacheKey] = pattern
	c.mu.Unlock()

	return pattern, nil
}

// getTerraformModulePatterns returns cached module patterns for a terraform component.
// This caches the expensive tfconfig.LoadModule() result and pattern computation.
func (c *componentPathPatternCache) getTerraformModulePatterns(
	component string,
	atmosConfig *schema.AtmosConfiguration,
) ([]string, error) {
	// Check cache with read lock.
	c.mu.RLock()
	if patterns, ok := c.modulePatterns[component]; ok {
		c.mu.RUnlock()
		return patterns, nil
	}
	c.mu.RUnlock()

	// Compute patterns (not in cache).
	componentPath := filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath, component)
	componentPathAbs, err := filepath.Abs(componentPath)
	if err != nil {
		return nil, err
	}

	// Load Terraform configuration (expensive operation).
	terraformConfiguration, diags := tfconfig.LoadModule(componentPathAbs)
	if diags.HasErrors() {
		if shouldCacheEmptyPatterns(diags.Err()) {
			c.cacheEmptyPatterns(component)
			return []string{}, nil
		}
		return nil, errors.Join(errUtils.ErrFailedToLoadTerraformModule, diags.Err())
	}

	if terraformConfiguration == nil {
		// No modules found (successful load with no modules), cache empty slice.
		c.cacheEmptyPatterns(component)
		return []string{}, nil
	}

	// Pre-compute ALL module patterns ONCE.
	patterns := computeModulePatternsFromConfig(terraformConfiguration)

	// Store in cache with write lock.
	c.mu.Lock()
	c.modulePatterns[component] = patterns
	c.mu.Unlock()

	return patterns, nil
}

// shouldCacheEmptyPatterns determines if the error indicates a missing directory that should cache empty patterns.
func shouldCacheEmptyPatterns(diagErr error) bool {
	// Try structured error detection first (most robust).
	if errors.Is(diagErr, os.ErrNotExist) || errors.Is(diagErr, fs.ErrNotExist) {
		return true
	}

	// Fallback to error message inspection for cases where tfconfig doesn't wrap errors properly.
	// This handles missing subdirectory modules (e.g., ./modules/security-group referenced in main.tf
	// but the directory doesn't exist). Such missing paths are valid in affected detection—components
	// or their modules may be deleted or not yet created when tracking changes over time.
	errMsg := diagErr.Error()
	return strings.Contains(errMsg, "does not exist") || strings.Contains(errMsg, "Failed to read directory")
}

// cacheEmptyPatterns caches an empty pattern slice for a component.
func (c *componentPathPatternCache) cacheEmptyPatterns(component string) {
	c.mu.Lock()
	c.modulePatterns[component] = []string{}
	c.mu.Unlock()
}

// computeModulePatternsFromConfig computes module patterns from a terraform configuration.
func computeModulePatternsFromConfig(terraformConfiguration *tfconfig.Module) []string {
	patterns := make([]string, 0, len(terraformConfiguration.ModuleCalls))
	for _, moduleConfig := range terraformConfiguration.ModuleCalls {
		// Skip remote modules (from terraform registry).
		if moduleConfig.Version != "" {
			continue
		}

		modulePath := filepath.Join(filepath.Dir(moduleConfig.Pos.Filename), moduleConfig.Source)
		modulePathAbs, err := filepath.Abs(modulePath)
		if err != nil {
			continue
		}

		patterns = append(patterns, modulePathAbs+"/**")
	}
	return patterns
}

// Clear clears the cache (useful for testing).
func (c *componentPathPatternCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.patterns = make(map[string]string)
	c.modulePatterns = make(map[string][]string)
}
