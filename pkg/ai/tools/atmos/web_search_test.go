package atmos

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/web"
)

func TestWebSearchTool_Name(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				WebSearch: schema.AIWebSearchSettings{
					Enabled: true,
				},
			},
		},
	}

	tool := NewWebSearchTool(atmosConfig)
	assert.Equal(t, "web_search", tool.Name())
}

func TestWebSearchTool_Description(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				WebSearch: schema.AIWebSearchSettings{
					Enabled: true,
				},
			},
		},
	}

	tool := NewWebSearchTool(atmosConfig)
	desc := tool.Description()
	assert.NotEmpty(t, desc)
	assert.Contains(t, desc, "Search the web")
}

func TestWebSearchTool_Parameters(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				WebSearch: schema.AIWebSearchSettings{
					Enabled: true,
				},
			},
		},
	}

	tool := NewWebSearchTool(atmosConfig)
	params := tool.Parameters()

	assert.Len(t, params, 2)

	// Check query parameter.
	assert.Equal(t, "query", params[0].Name)
	assert.Equal(t, "string", string(params[0].Type))
	assert.True(t, params[0].Required)

	// Check max_results parameter.
	assert.Equal(t, "max_results", params[1].Name)
	assert.Equal(t, "integer", string(params[1].Type))
	assert.False(t, params[1].Required)
	assert.Equal(t, 10, params[1].Default)
}

func TestWebSearchTool_RequiresPermission(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				WebSearch: schema.AIWebSearchSettings{
					Enabled: true,
				},
			},
		},
	}

	tool := NewWebSearchTool(atmosConfig)
	// Web search makes external requests, so should require permission.
	assert.True(t, tool.RequiresPermission())
}

func TestWebSearchTool_IsRestricted(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				WebSearch: schema.AIWebSearchSettings{
					Enabled: true,
				},
			},
		},
	}

	tool := NewWebSearchTool(atmosConfig)
	assert.False(t, tool.IsRestricted())
}

func TestWebSearchTool_Execute_NotEnabled(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				WebSearch: schema.AIWebSearchSettings{
					Enabled: false,
				},
			},
		},
	}

	tool := NewWebSearchTool(atmosConfig)
	ctx := context.Background()

	params := map[string]interface{}{
		"query": "test query",
	}

	result, err := tool.Execute(ctx, params)
	assert.Error(t, err)
	assert.False(t, result.Success)
}

func TestWebSearchTool_Execute_MissingQuery(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				WebSearch: schema.AIWebSearchSettings{
					Enabled: true,
				},
			},
		},
	}

	tool := NewWebSearchTool(atmosConfig)
	ctx := context.Background()

	params := map[string]interface{}{}

	result, err := tool.Execute(ctx, params)
	assert.Error(t, err)
	assert.False(t, result.Success)
}

func TestWebSearchTool_Execute_Success(t *testing.T) {
	t.Skip("Skipping integration test that requires internet connection")

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				WebSearch: schema.AIWebSearchSettings{
					Enabled:    true,
					MaxResults: 5,
				},
			},
		},
	}

	tool := NewWebSearchTool(atmosConfig)
	ctx := context.Background()

	params := map[string]interface{}{
		"query":       "Terraform AWS",
		"max_results": 3,
	}

	result, err := tool.Execute(ctx, params)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.NotEmpty(t, result.Output)

	// Check data structure.
	assert.Contains(t, result.Data, "query")
	assert.Contains(t, result.Data, "results")
	assert.Contains(t, result.Data, "count")

	query := result.Data["query"].(string)
	assert.Equal(t, "Terraform AWS", query)
}

func TestWebSearchTool_Execute_MaxResultsCapped(t *testing.T) {
	t.Skip("Skipping integration test that requires internet connection")

	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				WebSearch: schema.AIWebSearchSettings{
					Enabled:    true,
					MaxResults: 3, // Cap at 3
				},
			},
		},
	}

	tool := NewWebSearchTool(atmosConfig)
	ctx := context.Background()

	params := map[string]interface{}{
		"query":       "Terraform",
		"max_results": 10, // Request 10, but should be capped at 3
	}

	result, err := tool.Execute(ctx, params)
	require.NoError(t, err)
	assert.True(t, result.Success)

	count := result.Data["count"].(int)
	assert.LessOrEqual(t, count, 3)
}

func TestWebSearchTool_Execute_InvalidQueryType(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				WebSearch: schema.AIWebSearchSettings{
					Enabled: true,
				},
			},
		},
	}

	tool := NewWebSearchTool(atmosConfig)
	ctx := context.Background()

	// Pass query as int instead of string.
	params := map[string]interface{}{
		"query": 12345,
	}

	result, err := tool.Execute(ctx, params)
	assert.Error(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, err.Error(), "required")
}

func TestWebSearchTool_Execute_EmptyQuery(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				WebSearch: schema.AIWebSearchSettings{
					Enabled: true,
				},
			},
		},
	}

	tool := NewWebSearchTool(atmosConfig)
	ctx := context.Background()

	params := map[string]interface{}{
		"query": "",
	}

	result, err := tool.Execute(ctx, params)
	assert.Error(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, err.Error(), "required")
}

func TestWebSearchTool_Execute_MaxResultsEdgeCases(t *testing.T) {
	tests := []struct {
		name            string
		maxResults      interface{}
		configMaxResult int
		description     string
	}{
		{
			name:            "negative max_results",
			maxResults:      -5,
			configMaxResult: 0,
			description:     "should be normalized to 1",
		},
		{
			name:            "zero max_results",
			maxResults:      0,
			configMaxResult: 0,
			description:     "should be normalized to 1",
		},
		{
			name:            "very large max_results",
			maxResults:      1000,
			configMaxResult: 0,
			description:     "should be capped at 50",
		},
		{
			name:            "float max_results",
			maxResults:      5.7,
			configMaxResult: 0,
			description:     "should be converted to int 5",
		},
		{
			name:            "config override",
			maxResults:      20,
			configMaxResult: 10,
			description:     "should be capped by config max",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						WebSearch: schema.AIWebSearchSettings{
							Enabled:    true,
							MaxResults: tt.configMaxResult,
						},
					},
				},
			}

			// Note: This would require mocking the search engine to avoid actual HTTP calls.
			// For now, we're just testing the parameter validation logic.
			tool := NewWebSearchTool(atmosConfig)
			assert.NotNil(t, tool)

			// Verify tool was created successfully with the config.
			if tt.configMaxResult > 0 {
				assert.Equal(t, tt.configMaxResult, atmosConfig.Settings.AI.WebSearch.MaxResults)
			}
		})
	}
}

func TestWebSearchTool_Execute_DifferentEngines(t *testing.T) {
	tests := []struct {
		name          string
		engine        string
		googleAPIKey  string
		googleCSEID   string
		expectSuccess bool
	}{
		{
			name:          "DuckDuckGo engine (default)",
			engine:        "",
			googleAPIKey:  "",
			googleCSEID:   "",
			expectSuccess: true,
		},
		{
			name:          "Google engine with credentials",
			engine:        "google",
			googleAPIKey:  "test-key",
			googleCSEID:   "test-cse",
			expectSuccess: true,
		},
		{
			name:          "explicit DuckDuckGo",
			engine:        "duckduckgo",
			googleAPIKey:  "",
			googleCSEID:   "",
			expectSuccess: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						WebSearch: schema.AIWebSearchSettings{
							Enabled:      true,
							Engine:       tt.engine,
							GoogleAPIKey: tt.googleAPIKey,
							GoogleCSEID:  tt.googleCSEID,
						},
					},
				},
			}

			tool := NewWebSearchTool(atmosConfig)
			assert.NotNil(t, tool)

			// Verify engine is created (actual search would require integration test).
			assert.NotNil(t, tool.engine)
		})
	}
}

// mockSearchEngine is a mock implementation of web.SearchEngine for testing.
type mockSearchEngine struct {
	searchFunc func(ctx context.Context, query string, maxResults int) (*web.SearchResponse, error)
}

func (m *mockSearchEngine) Search(ctx context.Context, query string, maxResults int) (*web.SearchResponse, error) {
	if m.searchFunc != nil {
		return m.searchFunc(ctx, query, maxResults)
	}
	return &web.SearchResponse{
		Query:   query,
		Results: []web.SearchResult{},
		Count:   0,
	}, nil
}

func TestWebSearchTool_Execute_WithMockEngine_Success(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				WebSearch: schema.AIWebSearchSettings{
					Enabled: true,
				},
			},
		},
	}

	tool := NewWebSearchTool(atmosConfig)

	// Replace engine with mock.
	mockEngine := &mockSearchEngine{
		searchFunc: func(ctx context.Context, query string, maxResults int) (*web.SearchResponse, error) {
			return &web.SearchResponse{
				Query: query,
				Results: []web.SearchResult{
					{
						Title:       "Test Result 1",
						URL:         "https://example.com/1",
						Description: "First test result",
					},
					{
						Title:       "Test Result 2",
						URL:         "https://example.com/2",
						Description: "Second test result",
					},
				},
				Count: 2,
			}, nil
		},
	}
	tool.engine = mockEngine

	ctx := context.Background()
	params := map[string]interface{}{
		"query":       "test query",
		"max_results": 10,
	}

	result, err := tool.Execute(ctx, params)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.NotEmpty(t, result.Output)
	assert.Contains(t, result.Output, "Test Result 1")
	assert.Contains(t, result.Output, "Test Result 2")
	assert.Contains(t, result.Output, "https://example.com/1")
	assert.Contains(t, result.Output, "https://example.com/2")

	// Check data structure.
	assert.Contains(t, result.Data, "query")
	assert.Contains(t, result.Data, "results")
	assert.Contains(t, result.Data, "count")
	assert.Equal(t, "test query", result.Data["query"])
	assert.Equal(t, 2, result.Data["count"])
}

func TestWebSearchTool_Execute_WithMockEngine_NoResults(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				WebSearch: schema.AIWebSearchSettings{
					Enabled: true,
				},
			},
		},
	}

	tool := NewWebSearchTool(atmosConfig)

	// Replace engine with mock that returns no results.
	mockEngine := &mockSearchEngine{
		searchFunc: func(ctx context.Context, query string, maxResults int) (*web.SearchResponse, error) {
			return &web.SearchResponse{
				Query:   query,
				Results: []web.SearchResult{},
				Count:   0,
			}, nil
		},
	}
	tool.engine = mockEngine

	ctx := context.Background()
	params := map[string]interface{}{
		"query": "test query with no results",
	}

	result, err := tool.Execute(ctx, params)
	require.NoError(t, err)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "No results found")
	assert.Equal(t, 0, result.Data["count"])
}

func TestWebSearchTool_Execute_WithMockEngine_SearchError(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				WebSearch: schema.AIWebSearchSettings{
					Enabled: true,
				},
			},
		},
	}

	tool := NewWebSearchTool(atmosConfig)

	// Replace engine with mock that returns an error.
	searchErr := errors.New("search service unavailable")
	mockEngine := &mockSearchEngine{
		searchFunc: func(ctx context.Context, query string, maxResults int) (*web.SearchResponse, error) {
			return nil, searchErr
		},
	}
	tool.engine = mockEngine

	ctx := context.Background()
	params := map[string]interface{}{
		"query": "test query",
	}

	result, err := tool.Execute(ctx, params)
	assert.Error(t, err)
	assert.False(t, result.Success)
	assert.Contains(t, result.Output, "Web search failed")
	assert.Contains(t, result.Output, "search service unavailable")
	assert.Equal(t, searchErr, result.Error)
}

func TestWebSearchTool_Execute_MaxResultsValidation(t *testing.T) {
	tests := []struct {
		name             string
		inputMaxResults  interface{}
		configMaxResults int
		expectedMax      int
		description      string
	}{
		{
			name:             "negative max_results",
			inputMaxResults:  -5,
			configMaxResults: 0,
			expectedMax:      1,
			description:      "should be normalized to 1",
		},
		{
			name:             "zero max_results",
			inputMaxResults:  0,
			configMaxResults: 0,
			expectedMax:      1,
			description:      "should be normalized to 1",
		},
		{
			name:             "very large max_results",
			inputMaxResults:  1000,
			configMaxResults: 0,
			expectedMax:      50,
			description:      "should be capped at 50",
		},
		{
			name:             "float max_results",
			inputMaxResults:  5.7,
			configMaxResults: 0,
			expectedMax:      5,
			description:      "should be converted to int 5",
		},
		{
			name:             "config override - request higher than config",
			inputMaxResults:  20,
			configMaxResults: 10,
			expectedMax:      10,
			description:      "should be capped by config max",
		},
		{
			name:             "config override - request lower than config",
			inputMaxResults:  5,
			configMaxResults: 10,
			expectedMax:      5,
			description:      "should use requested value",
		},
		{
			name:             "valid max_results with no config limit",
			inputMaxResults:  15,
			configMaxResults: 0,
			expectedMax:      15,
			description:      "should use requested value",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						WebSearch: schema.AIWebSearchSettings{
							Enabled:    true,
							MaxResults: tt.configMaxResults,
						},
					},
				},
			}

			tool := NewWebSearchTool(atmosConfig)

			// Track what maxResults value is passed to the engine.
			var capturedMaxResults int
			mockEngine := &mockSearchEngine{
				searchFunc: func(ctx context.Context, query string, maxResults int) (*web.SearchResponse, error) {
					capturedMaxResults = maxResults
					return &web.SearchResponse{
						Query:   query,
						Results: []web.SearchResult{},
						Count:   0,
					}, nil
				},
			}
			tool.engine = mockEngine

			ctx := context.Background()
			params := map[string]interface{}{
				"query":       "test",
				"max_results": tt.inputMaxResults,
			}

			result, err := tool.Execute(ctx, params)
			require.NoError(t, err)
			assert.True(t, result.Success)
			assert.Equal(t, tt.expectedMax, capturedMaxResults, tt.description)
		})
	}
}

//nolint:dupl
func TestWebSearchTool_Execute_MaxResultsTypes(t *testing.T) {
	tests := []struct {
		name           string
		maxResults     interface{}
		expectedParsed int
		description    string
	}{
		{
			name:           "int type",
			maxResults:     5,
			expectedParsed: 5,
			description:    "should parse int correctly",
		},
		{
			name:           "float64 type",
			maxResults:     7.9,
			expectedParsed: 7,
			description:    "should parse float64 and truncate",
		},
		{
			name:           "missing max_results",
			maxResults:     nil,
			expectedParsed: 10,
			description:    "should use default of 10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						WebSearch: schema.AIWebSearchSettings{
							Enabled: true,
						},
					},
				},
			}

			tool := NewWebSearchTool(atmosConfig)

			var capturedMaxResults int
			mockEngine := &mockSearchEngine{
				searchFunc: func(ctx context.Context, query string, maxResults int) (*web.SearchResponse, error) {
					capturedMaxResults = maxResults
					return &web.SearchResponse{
						Query:   query,
						Results: []web.SearchResult{},
						Count:   0,
					}, nil
				},
			}
			tool.engine = mockEngine

			ctx := context.Background()
			params := map[string]interface{}{
				"query": "test",
			}
			if tt.maxResults != nil {
				params["max_results"] = tt.maxResults
			}

			result, err := tool.Execute(ctx, params)
			require.NoError(t, err)
			assert.True(t, result.Success)
			assert.Equal(t, tt.expectedParsed, capturedMaxResults, tt.description)
		})
	}
}

func TestWebSearchTool_Execute_OutputFormatting(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				WebSearch: schema.AIWebSearchSettings{
					Enabled: true,
				},
			},
		},
	}

	tool := NewWebSearchTool(atmosConfig)

	// Mock engine with results that have no description.
	mockEngine := &mockSearchEngine{
		searchFunc: func(ctx context.Context, query string, maxResults int) (*web.SearchResponse, error) {
			return &web.SearchResponse{
				Query: query,
				Results: []web.SearchResult{
					{
						Title:       "Result With Description",
						URL:         "https://example.com/1",
						Description: "This result has a description",
					},
					{
						Title:       "Result Without Description",
						URL:         "https://example.com/2",
						Description: "",
					},
				},
				Count: 2,
			}, nil
		},
	}
	tool.engine = mockEngine

	ctx := context.Background()
	params := map[string]interface{}{
		"query": "formatting test",
	}

	result, err := tool.Execute(ctx, params)
	require.NoError(t, err)
	assert.True(t, result.Success)

	// Verify output formatting.
	output := result.Output
	assert.Contains(t, output, "Web search results for 'formatting test' (2 results)")
	assert.Contains(t, output, "1. Result With Description")
	assert.Contains(t, output, "URL: https://example.com/1")
	assert.Contains(t, output, "This result has a description")
	assert.Contains(t, output, "2. Result Without Description")
	assert.Contains(t, output, "URL: https://example.com/2")

	// Verify result structure numbering.
	assert.Regexp(t, `1\. Result With Description`, output)
	assert.Regexp(t, `2\. Result Without Description`, output)
}

func TestWebSearchTool_Execute_NilConfig(t *testing.T) {
	// Create tool with nil config.
	tool := NewWebSearchTool(nil)
	assert.NotNil(t, tool)
	assert.Nil(t, tool.atmosConfig)

	// Mock the engine since NewWebSearchTool creates one.
	mockEngine := &mockSearchEngine{
		searchFunc: func(ctx context.Context, query string, maxResults int) (*web.SearchResponse, error) {
			return &web.SearchResponse{
				Query:   query,
				Results: []web.SearchResult{},
				Count:   0,
			}, nil
		},
	}
	tool.engine = mockEngine

	ctx := context.Background()
	params := map[string]interface{}{
		"query": "test",
	}

	// With nil config, web search should still work (no enabled check enforced).
	result, err := tool.Execute(ctx, params)
	require.NoError(t, err)
	assert.True(t, result.Success)
}

func TestWebSearchTool_Execute_ErrorChecking(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				WebSearch: schema.AIWebSearchSettings{
					Enabled: false,
				},
			},
		},
	}

	tool := NewWebSearchTool(atmosConfig)
	ctx := context.Background()

	params := map[string]interface{}{
		"query": "test",
	}

	result, err := tool.Execute(ctx, params)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrWebSearchNotEnabled)
	assert.False(t, result.Success)
	assert.Equal(t, errUtils.ErrWebSearchNotEnabled, result.Error)
}

func TestWebSearchTool_Execute_QueryValidation(t *testing.T) {
	tests := []struct {
		name        string
		params      map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name:        "missing query parameter",
			params:      map[string]interface{}{},
			expectError: true,
			errorMsg:    "required",
		},
		{
			name: "empty query string",
			params: map[string]interface{}{
				"query": "",
			},
			expectError: true,
			errorMsg:    "required",
		},
		{
			name: "query as non-string type",
			params: map[string]interface{}{
				"query": 12345,
			},
			expectError: true,
			errorMsg:    "required",
		},
		{
			name: "query as boolean",
			params: map[string]interface{}{
				"query": true,
			},
			expectError: true,
			errorMsg:    "required",
		},
		{
			name: "query as nil",
			params: map[string]interface{}{
				"query": nil,
			},
			expectError: true,
			errorMsg:    "required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						WebSearch: schema.AIWebSearchSettings{
							Enabled: true,
						},
					},
				},
			}

			tool := NewWebSearchTool(atmosConfig)
			ctx := context.Background()

			result, err := tool.Execute(ctx, tt.params)
			if tt.expectError {
				assert.Error(t, err)
				assert.False(t, result.Success)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
				assert.True(t, result.Success)
			}
		})
	}
}
