package aqua

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	log "github.com/charmbracelet/log"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/toolchain/registry"
)

// Search searches for tools matching the query string.
// The query is matched against tool owner, repo, and description.
func (ar *AquaRegistry) Search(ctx context.Context, query string, opts ...registry.SearchOption) ([]*registry.Tool, error) {
	defer perf.Track(nil, "aqua.AquaRegistry.Search")()

	config := applySearchOptions(opts)
	allTools, err := ar.fetchToolsForSearch(ctx)
	if err != nil {
		return nil, err
	}

	results := ar.scoreAndSortResults(allTools, query)
	ar.lastSearchTotal = len(results)

	return paginateResults(results, config), nil
}

// applySearchOptions applies search options and returns the config.
func applySearchOptions(opts []registry.SearchOption) *registry.SearchConfig {
	config := &registry.SearchConfig{
		Limit: defaultSearchLimit,
	}
	for _, opt := range opts {
		opt(config)
	}
	return config
}

// fetchToolsForSearch fetches all tools for search.
func (ar *AquaRegistry) fetchToolsForSearch(ctx context.Context) ([]*registry.Tool, error) {
	listStart := time.Now()
	allTools, err := ar.ListAll(ctx, registry.WithListLimit(0))
	if err != nil {
		return nil, err
	}
	log.Debug("ListAll took", durationMetricKey, time.Since(listStart), "tools", len(allTools))
	return allTools, nil
}

// scoreAndSortResults scores and sorts tools by relevance.
func (ar *AquaRegistry) scoreAndSortResults(tools []*registry.Tool, query string) []scoredTool {
	queryLower := strings.ToLower(query)

	scoreStart := time.Now()
	var results []scoredTool
	for _, tool := range tools {
		score := ar.calculateRelevanceScore(tool, queryLower)
		if score > 0 {
			results = append(results, scoredTool{tool: tool, score: score})
		}
	}
	log.Debug("Scoring took", durationMetricKey, time.Since(scoreStart), "matches", len(results))

	sortStart := time.Now()
	sortResults(results)
	log.Debug("Sort took", durationMetricKey, time.Since(sortStart))

	return results
}

// paginateResults applies offset and limit to results.
func paginateResults(results []scoredTool, config *registry.SearchConfig) []*registry.Tool {
	start := config.Offset
	if start > len(results) {
		start = len(results)
	}

	end := start + config.Limit
	if config.Limit == 0 || end > len(results) {
		end = len(results)
	}

	filtered := make([]*registry.Tool, 0, end-start)
	for i := start; i < end; i++ {
		filtered = append(filtered, results[i].tool)
	}

	return filtered
}

// calculateRelevanceScore scores a tool based on query match.
func (ar *AquaRegistry) calculateRelevanceScore(tool *registry.Tool, queryLower string) int {
	repoLower := strings.ToLower(tool.RepoName)
	ownerLower := strings.ToLower(tool.RepoOwner)

	if repoLower == queryLower {
		return scoreExactRepoMatch
	}

	score := 0

	if strings.HasPrefix(repoLower, queryLower) {
		score += scoreRepoPrefixMatch
	} else if strings.Contains(repoLower, queryLower) {
		score += scoreRepoContainsMatch
	}

	if strings.HasPrefix(ownerLower, queryLower) {
		score += scoreOwnerPrefixMatch
	} else if strings.Contains(ownerLower, queryLower) {
		score += scoreOwnerContainsMatch
	}

	return score
}

// sortResults sorts scored tools by score (descending) then alphabetically.
func sortResults(results []scoredTool) {
	sort.Slice(results, func(i, j int) bool {
		if results[i].score != results[j].score {
			return results[i].score > results[j].score
		}
		return results[i].tool.RepoName < results[j].tool.RepoName
	})
}

// GetLastSearchTotal returns the total number of search results before pagination.
func (ar *AquaRegistry) GetLastSearchTotal() int {
	defer perf.Track(nil, "aqua.AquaRegistry.GetLastSearchTotal")()

	return ar.lastSearchTotal
}

// ListAll returns all tools available in the Aqua registry.
func (ar *AquaRegistry) ListAll(ctx context.Context, opts ...registry.ListOption) ([]*registry.Tool, error) {
	defer perf.Track(nil, "aqua.AquaRegistry.ListAll")()

	config := applyListOptions(opts)
	tools, err := ar.fetchRegistryIndex(ctx)
	if err != nil {
		return nil, err
	}

	if config.Sort == "name" {
		sortToolsByName(tools)
	}

	return paginateToolsList(tools, config), nil
}

// applyListOptions applies list options and returns the config.
func applyListOptions(opts []registry.ListOption) *registry.ListConfig {
	config := &registry.ListConfig{
		Limit: defaultListLimit,
		Sort:  "name",
	}
	for _, opt := range opts {
		opt(config)
	}
	return config
}

// paginateToolsList applies offset and limit to tools.
func paginateToolsList(tools []*registry.Tool, config *registry.ListConfig) []*registry.Tool {
	start := config.Offset
	if start > len(tools) {
		start = len(tools)
	}

	end := start + config.Limit
	if config.Limit == 0 || end > len(tools) {
		end = len(tools)
	}

	return tools[start:end]
}

// fetchRegistryIndex fetches the complete registry index from aqua-registry.
func (ar *AquaRegistry) fetchRegistryIndex(ctx context.Context) ([]*registry.Tool, error) {
	defer perf.Track(nil, "aqua.AquaRegistry.fetchRegistryIndex")()

	const cacheKey = "aqua-registry-index"
	const cacheTTL = 24 * time.Hour

	if tools := ar.tryGetCachedIndex(ctx, cacheKey); tools != nil {
		return tools, nil
	}

	return ar.fetchAndCacheIndex(ctx, cacheKey, cacheTTL)
}

// tryGetCachedIndex attempts to get the registry index from cache.
func (ar *AquaRegistry) tryGetCachedIndex(ctx context.Context, cacheKey string) []*registry.Tool {
	start := time.Now()
	cachedData, err := ar.cacheStore.Get(ctx, cacheKey)
	if err != nil {
		return nil
	}
	log.Debug("Cache read took", durationMetricKey, time.Since(start))

	parseStart := time.Now()
	tools, err := ar.parseIndexYAML(cachedData)
	log.Debug("Parse took", durationMetricKey, time.Since(parseStart), "tools", len(tools))
	if err != nil {
		log.Debug("Failed to parse cached index, fetching fresh", "error", err)
		return nil
	}

	log.Debug("Using cached registry index", "tool_count", len(tools))
	return tools
}

// fetchAndCacheIndex fetches the registry index from GitHub and caches it.
func (ar *AquaRegistry) fetchAndCacheIndex(ctx context.Context, cacheKey string, cacheTTL time.Duration) ([]*registry.Tool, error) {
	indexURL := "https://raw.githubusercontent.com/aquaproj/aqua-registry/main/registry.yaml"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, indexURL, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to create request: %w", registry.ErrHTTPRequest, err)
	}

	resp, err := ar.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to fetch registry index: %w", registry.ErrHTTPRequest, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: failed to fetch registry index (HTTP %d)", registry.ErrHTTPRequest, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to read registry index: %w", registry.ErrHTTPRequest, err)
	}

	tools, err := ar.parseIndexYAML(data)
	if err != nil {
		return nil, err
	}

	if err := ar.cacheStore.Set(ctx, cacheKey, data, cacheTTL); err != nil {
		log.Debug("Failed to cache registry index", "error", err)
	}

	log.Debug("Fetched registry index", "tool_count", len(tools))
	return tools, nil
}

// parseIndexYAML parses the aqua-registry registry.yaml format.
func (ar *AquaRegistry) parseIndexYAML(data []byte) ([]*registry.Tool, error) {
	defer perf.Track(nil, "aqua.AquaRegistry.parseIndexYAML")()

	var index indexFile
	if err := yaml.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("%w: failed to parse registry index: %w", registry.ErrRegistryParse, err)
	}

	return ar.convertPackagesToTools(index.Packages), nil
}

// indexFile represents the structure of the registry.yaml file.
type indexFile struct {
	Packages []indexPackage `yaml:"packages"`
}

// indexPackage represents a package in the registry index.
type indexPackage struct {
	Type      string `yaml:"type"`
	RepoOwner string `yaml:"repo_owner"`
	RepoName  string `yaml:"repo_name"`
	Name      string `yaml:"name"`
	Path      string `yaml:"path"`
}

// convertPackagesToTools converts index packages to Tool objects.
func (ar *AquaRegistry) convertPackagesToTools(packages []indexPackage) []*registry.Tool {
	tools := make([]*registry.Tool, 0, len(packages))
	for i := range packages {
		pkg := &packages[i]
		if pkg.Type == "" {
			continue
		}

		owner, repo := resolveOwnerRepo(pkg)
		tools = append(tools, &registry.Tool{
			RepoOwner: owner,
			RepoName:  repo,
			Type:      pkg.Type,
			Registry:  "aqua-public",
		})
	}
	return tools
}

// resolveOwnerRepo extracts owner and repo from a package.
func resolveOwnerRepo(pkg *indexPackage) (owner, repo string) {
	owner = pkg.RepoOwner
	repo = pkg.RepoName

	if pkg.Name != "" && owner == "" && repo == "" {
		owner, repo = parseNameField(pkg.Name)
	}

	if pkg.Type == "go_install" && pkg.Path != "" && owner == "" && repo == "" {
		owner, repo = parseGoPath(pkg.Path)
	}

	return owner, repo
}

// parseNameField parses owner/repo from a name field.
func parseNameField(name string) (owner, repo string) {
	parts := strings.SplitN(name, "/", 3)
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}
	if len(parts) == 1 {
		return "", name
	}
	return "", ""
}

// parseGoPath parses owner/repo from a Go module path.
func parseGoPath(path string) (owner, repo string) {
	parts := strings.Split(path, "/")
	if len(parts) < 3 {
		return "", ""
	}

	switch {
	case parts[0] == "golang.org" && len(parts) >= 2:
		return "golang", parts[1]
	case parts[0] == "github.com" && len(parts) >= 3:
		return parts[1], parts[2]
	default:
		if len(parts) > 1 {
			return parts[0], parts[1]
		}
		return parts[0], ""
	}
}

// sortToolsByName sorts tools alphabetically by repo name.
func sortToolsByName(tools []*registry.Tool) {
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].RepoName < tools[j].RepoName
	})
}

// GetMetadata returns metadata about the Aqua registry.
func (ar *AquaRegistry) GetMetadata(ctx context.Context) (*registry.RegistryMetadata, error) {
	defer perf.Track(nil, "aqua.AquaRegistry.GetMetadata")()

	tools, err := ar.ListAll(ctx, registry.WithListLimit(0))
	if err != nil {
		return nil, err
	}

	return &registry.RegistryMetadata{
		Name:        "aqua-public",
		Type:        "aqua",
		Source:      "https://github.com/aquaproj/aqua-registry",
		Priority:    defaultRegistryPriority,
		ToolCount:   len(tools),
		LastUpdated: time.Now(),
	}, nil
}
