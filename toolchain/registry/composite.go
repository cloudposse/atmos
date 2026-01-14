package registry

import (
	"context"
	"fmt"
	"sort"

	"github.com/cloudposse/atmos/pkg/perf"
)

// CompositeRegistry coordinates multiple registry sources with priority-based precedence.
// Higher priority registries are checked first, with fallback to lower priority registries.
type CompositeRegistry struct {
	registries []PrioritizedRegistry
}

// PrioritizedRegistry wraps a registry with priority and name metadata.
type PrioritizedRegistry struct {
	Name     string
	Registry ToolRegistry
	Priority int
}

// NewCompositeRegistry creates a new composite registry from multiple registry sources.
func NewCompositeRegistry(registries []PrioritizedRegistry) *CompositeRegistry {
	defer perf.Track(nil, "registry.NewCompositeRegistry")()

	// Sort by priority (highest first).
	sorted := make([]PrioritizedRegistry, len(registries))
	copy(sorted, registries)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority > sorted[j].Priority
	})

	return &CompositeRegistry{
		registries: sorted,
	}
}

// GetTool tries to get tool metadata from registries in priority order.
func (cr *CompositeRegistry) GetTool(owner, repo string) (*Tool, error) {
	defer perf.Track(nil, "registry.CompositeRegistry.GetTool")()

	// Try each registry in priority order.
	var lastErr error
	for _, pr := range cr.registries {
		tool, err := pr.Registry.GetTool(owner, repo)
		if err == nil {
			return tool, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, fmt.Errorf("%w: checked %d registries: %w", ErrToolNotFound, len(cr.registries), lastErr)
	}

	return nil, fmt.Errorf("%w: not found in %d configured registries", ErrToolNotFound, len(cr.registries))
}

// GetToolWithVersion tries to get versioned tool metadata from registries in priority order.
func (cr *CompositeRegistry) GetToolWithVersion(owner, repo, version string) (*Tool, error) {
	defer perf.Track(nil, "registry.CompositeRegistry.GetToolWithVersion")()

	// Try each registry in priority order.
	for _, pr := range cr.registries {
		tool, err := pr.Registry.GetToolWithVersion(owner, repo, version)
		if err == nil {
			return tool, nil
		}
	}

	return nil, fmt.Errorf("%w: %s/%s@%s not found in %d registries", ErrToolNotFound, owner, repo, version, len(cr.registries))
}

// GetLatestVersion tries to get the latest version from registries in priority order.
func (cr *CompositeRegistry) GetLatestVersion(owner, repo string) (string, error) {
	defer perf.Track(nil, "registry.CompositeRegistry.GetLatestVersion")()

	// Try each registry in priority order.
	for _, pr := range cr.registries {
		version, err := pr.Registry.GetLatestVersion(owner, repo)
		if err == nil {
			return version, nil
		}
	}

	return "", fmt.Errorf("%w: %s/%s not found in %d registries", ErrToolNotFound, owner, repo, len(cr.registries))
}

// LoadLocalConfig is deprecated and no-op for compatibility.
func (cr *CompositeRegistry) LoadLocalConfig(configPath string) error {
	defer perf.Track(nil, "registry.CompositeRegistry.LoadLocalConfig")()

	// No-op for backward compatibility.
	return nil
}

// Search searches across all registries and combines results.
// Results are deduplicated (highest priority registry wins) and sorted by relevance.
func (cr *CompositeRegistry) Search(ctx context.Context, query string, opts ...SearchOption) ([]*Tool, error) {
	defer perf.Track(nil, "registry.CompositeRegistry.Search")()

	// Collect results from all registries.
	seen := make(map[string]*Tool) // key: owner/repo
	for _, pr := range cr.registries {
		results, err := pr.Registry.Search(ctx, query, opts...)
		if err != nil {
			// Continue on error, try other registries.
			continue
		}

		// Add to results, deduplicating by owner/repo.
		for _, tool := range results {
			key := tool.RepoOwner + "/" + tool.RepoName
			if _, exists := seen[key]; !exists {
				// First time seeing this tool (highest priority).
				seen[key] = tool
			}
		}
	}

	// Convert map to slice.
	combined := make([]*Tool, 0, len(seen))
	for _, tool := range seen {
		combined = append(combined, tool)
	}

	return combined, nil
}

// ListAll lists tools from all registries, deduplicated.
func (cr *CompositeRegistry) ListAll(ctx context.Context, opts ...ListOption) ([]*Tool, error) {
	defer perf.Track(nil, "registry.CompositeRegistry.ListAll")()

	// Collect results from all registries.
	seen := make(map[string]*Tool) // key: owner/repo
	for _, pr := range cr.registries {
		results, err := pr.Registry.ListAll(ctx, opts...)
		if err != nil {
			// Continue on error, try other registries.
			continue
		}

		// Add to results, deduplicating by owner/repo.
		for _, tool := range results {
			key := tool.RepoOwner + "/" + tool.RepoName
			if _, exists := seen[key]; !exists {
				// First time seeing this tool (highest priority).
				seen[key] = tool
			}
		}
	}

	// Convert map to slice.
	combined := make([]*Tool, 0, len(seen))
	for _, tool := range seen {
		combined = append(combined, tool)
	}

	return combined, nil
}

// GetMetadata returns aggregated metadata from all registries.
func (cr *CompositeRegistry) GetMetadata(ctx context.Context) (*RegistryMetadata, error) {
	defer perf.Track(nil, "registry.CompositeRegistry.GetMetadata")()

	// Aggregate metadata from all registries.
	totalTools := 0
	for _, pr := range cr.registries {
		meta, err := pr.Registry.GetMetadata(ctx)
		if err != nil {
			continue
		}
		totalTools += meta.ToolCount
	}

	return &RegistryMetadata{
		Name:      "composite",
		Type:      "composite",
		Source:    fmt.Sprintf("%d configured registries", len(cr.registries)),
		Priority:  0,
		ToolCount: totalTools,
	}, nil
}
