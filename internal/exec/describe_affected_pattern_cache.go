package exec

import (
	"fmt"
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
//
//nolint:revive // Error handling for missing directories requires additional lines
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
		errMsg := diags.Err().Error()
		// If the directory doesn't exist, treat as empty (component may be deleted or not yet created).
		// This is valid in affected detection when tracking changes over time.
		if strings.Contains(errMsg, "does not exist") || strings.Contains(errMsg, "Failed to read directory") {
			c.mu.Lock()
			c.modulePatterns[component] = []string{}
			c.mu.Unlock()
			return []string{}, nil
		}

		// For other errors (syntax errors, permission issues, etc.), return error.
		// This prevents false negatives from caching incorrect empty results.
		return nil, fmt.Errorf("%w at %s: %v", errUtils.ErrFailedToLoadTerraformModule, componentPathAbs, diags.Err())
	}

	if terraformConfiguration == nil {
		// No modules found (successful load with no modules), cache empty slice.
		c.mu.Lock()
		c.modulePatterns[component] = []string{}
		c.mu.Unlock()
		return []string{}, nil
	}

	// Pre-compute ALL module patterns ONCE.
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

	// Store in cache with write lock.
	c.mu.Lock()
	c.modulePatterns[component] = patterns
	c.mu.Unlock()

	return patterns, nil
}

// Clear clears the cache (useful for testing).
func (c *componentPathPatternCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.patterns = make(map[string]string)
	c.modulePatterns = make(map[string][]string)
}
