package atmos

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/toolchain/registry"
)

// Error definitions for the atmos registry package.
var (
	// ErrInvalidToolConfig indicates the tool configuration is malformed.
	ErrInvalidToolConfig = errors.New("invalid tool configuration")

	// ErrMissingRequiredField indicates a required field is missing from the config.
	ErrMissingRequiredField = errors.New("missing required field")
)

// init registers the Atmos registry constructor.
func init() {
	registry.RegisterAtmosRegistry(func(tools map[string]any) (registry.ToolRegistry, error) {
		return NewAtmosRegistry(tools)
	})
}

// AtmosRegistry implements inline tool definitions in atmos.yaml.
// Tools are defined directly in the configuration rather than fetched from external sources.
type AtmosRegistry struct {
	tools map[string]*registry.Tool // Cached tools indexed by "owner/repo".
}

// NewAtmosRegistry creates a new inline registry from tool definitions.
func NewAtmosRegistry(toolsConfig map[string]any) (*AtmosRegistry, error) {
	defer perf.Track(nil, "atmos.NewAtmosRegistry")()

	tools, err := parseToolDefinitions(toolsConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to parse tool definitions: %w", err)
	}

	return &AtmosRegistry{
		tools: tools,
	}, nil
}

// parseToolDefinitions converts raw tool config to Tool objects.
// This is a pure function for easier testing.
func parseToolDefinitions(toolsConfig map[string]any) (map[string]*registry.Tool, error) {
	defer perf.Track(nil, "atmos.parseToolDefinitions")()

	tools := make(map[string]*registry.Tool)

	for toolID, toolConfigRaw := range toolsConfig {
		// Parse tool identifier (owner/repo format).
		owner, repo, err := parseToolID(toolID)
		if err != nil {
			return nil, fmt.Errorf("invalid tool ID %q: %w", toolID, err)
		}

		// Convert tool config to map.
		toolConfig, ok := toolConfigRaw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%w: tool %q config must be a map, got %T", ErrInvalidToolConfig, toolID, toolConfigRaw)
		}

		// Parse tool from config.
		tool, err := parseToolConfig(owner, repo, toolConfig)
		if err != nil {
			return nil, fmt.Errorf("tool %q: %w", toolID, err)
		}

		tools[toolID] = tool
	}

	return tools, nil
}

// parseToolID splits a tool identifier into owner and repo.
// Pure function for testability.
func parseToolID(toolID string) (owner, repo string, err error) {
	defer perf.Track(nil, "atmos.parseToolID")()

	parts := strings.Split(toolID, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("%w: tool ID must be in 'owner/repo' format, got %q", ErrInvalidToolConfig, toolID)
	}

	owner = strings.TrimSpace(parts[0])
	repo = strings.TrimSpace(parts[1])

	if owner == "" || repo == "" {
		return "", "", fmt.Errorf("%w: owner and repo cannot be empty", ErrInvalidToolConfig)
	}

	return owner, repo, nil
}

// parseToolConfig converts a tool config map to a Tool object.
// Pure function for testability.
func parseToolConfig(owner, repo string, config map[string]any) (*registry.Tool, error) {
	defer perf.Track(nil, "atmos.parseToolConfig")()

	tool := &registry.Tool{
		RepoOwner: owner,
		RepoName:  repo,
		Name:      repo, // Default name is repo name.
		Registry:  "atmos-inline",
	}

	// Parse type (required).
	toolType, ok := config["type"].(string)
	if !ok || toolType == "" {
		return nil, fmt.Errorf("%w: 'type' field is required and must be a string", ErrMissingRequiredField)
	}
	tool.Type = toolType

	// Parse URL/asset template (required).
	url, ok := config["url"].(string)
	if !ok || url == "" {
		return nil, fmt.Errorf("%w: 'url' field is required and must be a string", ErrMissingRequiredField)
	}
	tool.URL = url
	// Asset is used for template rendering in BuildAssetURL for both types.
	tool.Asset = url

	// Parse optional fields.
	if format, ok := config["format"].(string); ok {
		tool.Format = format
	}

	if binaryName, ok := config["binary_name"].(string); ok {
		tool.BinaryName = binaryName
		tool.Name = binaryName
	}

	return tool, nil
}

// GetTool fetches tool metadata from inline definitions.
func (ar *AtmosRegistry) GetTool(owner, repo string) (*registry.Tool, error) {
	defer perf.Track(nil, "atmos.AtmosRegistry.GetTool")()

	key := fmt.Sprintf("%s/%s", owner, repo)
	tool, exists := ar.tools[key]
	if !exists {
		return nil, fmt.Errorf("%w: %s not found in inline registry", registry.ErrToolNotFound, key)
	}

	return tool, nil
}

// GetToolWithVersion fetches tool metadata and sets the version.
func (ar *AtmosRegistry) GetToolWithVersion(owner, repo, version string) (*registry.Tool, error) {
	defer perf.Track(nil, "atmos.AtmosRegistry.GetToolWithVersion")()

	tool, err := ar.GetTool(owner, repo)
	if err != nil {
		return nil, err
	}

	// Set version on the tool.
	tool.Version = version

	return tool, nil
}

// GetLatestVersion is not supported for inline registries.
// Inline registries don't track version information.
func (ar *AtmosRegistry) GetLatestVersion(owner, repo string) (string, error) {
	defer perf.Track(nil, "atmos.AtmosRegistry.GetLatestVersion")()

	return "", fmt.Errorf("%w: inline registries do not support version queries", registry.ErrNoVersionsFound)
}

// LoadLocalConfig is a no-op for inline registries.
func (ar *AtmosRegistry) LoadLocalConfig(configPath string) error {
	defer perf.Track(nil, "atmos.AtmosRegistry.LoadLocalConfig")()

	// Inline registries don't use local config.
	return nil
}

// Search searches tools in the inline registry.
func (ar *AtmosRegistry) Search(ctx context.Context, query string, opts ...registry.SearchOption) ([]*registry.Tool, error) {
	defer perf.Track(nil, "atmos.AtmosRegistry.Search")()

	var results []*registry.Tool
	queryLower := strings.ToLower(query)

	for _, tool := range ar.tools {
		// Simple substring matching on repo name and owner.
		if strings.Contains(strings.ToLower(tool.RepoName), queryLower) ||
			strings.Contains(strings.ToLower(tool.RepoOwner), queryLower) {
			results = append(results, tool)
		}
	}

	return results, nil
}

// ListAll returns all tools defined in the inline registry.
func (ar *AtmosRegistry) ListAll(ctx context.Context, opts ...registry.ListOption) ([]*registry.Tool, error) {
	defer perf.Track(nil, "atmos.AtmosRegistry.ListAll")()

	tools := make([]*registry.Tool, 0, len(ar.tools))
	for _, tool := range ar.tools {
		tools = append(tools, tool)
	}

	return tools, nil
}

// GetMetadata returns metadata about the inline registry.
func (ar *AtmosRegistry) GetMetadata(ctx context.Context) (*registry.RegistryMetadata, error) {
	defer perf.Track(nil, "atmos.AtmosRegistry.GetMetadata")()

	return &registry.RegistryMetadata{
		Name:      "atmos-inline",
		Type:      "atmos",
		Source:    "inline (atmos.yaml)",
		Priority:  0,
		ToolCount: len(ar.tools),
	}, nil
}
