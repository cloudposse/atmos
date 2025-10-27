package atmos

import (
	"context"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	perf "github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/web"
)

// WebSearchTool performs web searches to gather information.
type WebSearchTool struct {
	atmosConfig *schema.AtmosConfiguration
	engine      web.SearchEngine
}

// NewWebSearchTool creates a new web search tool.
func NewWebSearchTool(atmosConfig *schema.AtmosConfiguration) *WebSearchTool {
	defer perf.Track(atmosConfig, "pkg.ai.tools.atmos.NewWebSearchTool")()

	return &WebSearchTool{
		atmosConfig: atmosConfig,
		engine:      web.NewSearchEngine(atmosConfig),
	}
}

// Name returns the tool name.
func (t *WebSearchTool) Name() string {
	return "web_search"
}

// Description returns the tool description.
func (t *WebSearchTool) Description() string {
	return "Search the web for information using DuckDuckGo or Google Custom Search. Returns titles, URLs, and descriptions of search results. Use this to find current information, documentation, examples, or answers to questions that require up-to-date knowledge."
}

// Parameters returns the tool parameters.
func (t *WebSearchTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "query",
			Description: "The search query to execute (e.g., 'Terraform AWS VPC module', 'latest Kubernetes version', 'how to configure Atmos stacks')",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "max_results",
			Description: "Maximum number of search results to return (1-50, default: 10)",
			Type:        tools.ParamTypeInt,
			Required:    false,
			Default:     10,
		},
	}
}

// Execute runs the tool.
func (t *WebSearchTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	defer perf.Track(t.atmosConfig, "pkg.ai.tools.atmos.WebSearchTool.Execute")()

	// Check if web search is enabled.
	if t.atmosConfig != nil && !t.atmosConfig.Settings.AI.WebSearch.Enabled {
		return &tools.Result{
			Success: false,
			Error:   errUtils.ErrWebSearchNotEnabled,
		}, errUtils.ErrWebSearchNotEnabled
	}

	// Extract query parameter (required).
	query, ok := params["query"].(string)
	if !ok || query == "" {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: query", errUtils.ErrAIToolParameterRequired),
		}, fmt.Errorf("%w: query", errUtils.ErrAIToolParameterRequired)
	}

	// Extract max_results parameter (optional).
	maxResults := 10
	if mr, ok := params["max_results"].(float64); ok {
		maxResults = int(mr)
	} else if mr, ok := params["max_results"].(int); ok {
		maxResults = mr
	}

	// Validate max_results.
	if maxResults < 1 {
		maxResults = 1
	}
	if maxResults > 50 {
		maxResults = 50
	}

	// Override max_results from configuration if set.
	if t.atmosConfig != nil && t.atmosConfig.Settings.AI.WebSearch.MaxResults > 0 {
		if maxResults > t.atmosConfig.Settings.AI.WebSearch.MaxResults {
			maxResults = t.atmosConfig.Settings.AI.WebSearch.MaxResults
		}
	}

	// Execute search.
	response, err := t.engine.Search(ctx, query, maxResults)
	if err != nil {
		return &tools.Result{
			Success: false,
			Output:  fmt.Sprintf("Web search failed: %v", err),
			Error:   err,
		}, err
	}

	// Format output.
	var output strings.Builder
	output.WriteString(fmt.Sprintf("Web search results for '%s' (%d results):\n\n", query, response.Count))

	if response.Count == 0 {
		output.WriteString("No results found.\n")
	} else {
		for i, result := range response.Results {
			output.WriteString(fmt.Sprintf("%d. %s\n", i+1, result.Title))
			output.WriteString(fmt.Sprintf("   URL: %s\n", result.URL))
			if result.Description != "" {
				output.WriteString(fmt.Sprintf("   %s\n", result.Description))
			}
			output.WriteString("\n")
		}
	}

	return &tools.Result{
		Success: true,
		Output:  output.String(),
		Data: map[string]interface{}{
			"query":   query,
			"results": response.Results,
			"count":   response.Count,
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *WebSearchTool) RequiresPermission() bool {
	// Web search makes external HTTP requests, so require permission by default.
	return true
}

// IsRestricted returns true if this tool is always restricted.
func (t *WebSearchTool) IsRestricted() bool {
	return false
}
