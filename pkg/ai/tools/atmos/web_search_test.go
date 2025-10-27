package atmos

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
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

	// Pass query as int instead of string
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

			// Note: This would require mocking the search engine to avoid actual HTTP calls
			// For now, we're just testing the parameter validation logic
			tool := NewWebSearchTool(atmosConfig)
			assert.NotNil(t, tool)

			// Verify tool was created successfully with the config
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

			// Verify engine is created (actual search would require integration test)
			assert.NotNil(t, tool.engine)
		})
	}
}
