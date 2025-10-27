package web

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewDuckDuckGoEngine(t *testing.T) {
	engine := NewDuckDuckGoEngine()
	assert.NotNil(t, engine)
	assert.NotNil(t, engine.client)
	assert.Equal(t, DefaultHTTPTimeout, engine.client.Timeout)
}

func TestNewGoogleEngine(t *testing.T) {
	apiKey := "test-api-key"
	cseID := "test-cse-id"

	engine := NewGoogleEngine(apiKey, cseID)
	assert.NotNil(t, engine)
	assert.NotNil(t, engine.client)
	assert.Equal(t, apiKey, engine.apiKey)
	assert.Equal(t, cseID, engine.cseID)
	assert.Equal(t, DefaultHTTPTimeout, engine.client.Timeout)
}

func TestNormalizeMaxResults(t *testing.T) {
	tests := []struct {
		name        string
		maxResults  int
		defaultMax  int
		absoluteMax int
		expected    int
	}{
		{
			name:        "zero returns default",
			maxResults:  0,
			defaultMax:  10,
			absoluteMax: 50,
			expected:    10,
		},
		{
			name:        "negative returns default",
			maxResults:  -5,
			defaultMax:  10,
			absoluteMax: 50,
			expected:    10,
		},
		{
			name:        "within bounds unchanged",
			maxResults:  25,
			defaultMax:  10,
			absoluteMax: 50,
			expected:    25,
		},
		{
			name:        "exceeds max capped to absoluteMax",
			maxResults:  100,
			defaultMax:  10,
			absoluteMax: 50,
			expected:    50,
		},
		{
			name:        "exact max unchanged",
			maxResults:  50,
			defaultMax:  10,
			absoluteMax: 50,
			expected:    50,
		},
		{
			name:        "exact default unchanged",
			maxResults:  10,
			defaultMax:  10,
			absoluteMax: 50,
			expected:    10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeMaxResults(tt.maxResults, tt.defaultMax, tt.absoluteMax)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestParseDuckDuckGoURL(t *testing.T) {
	tests := []struct {
		name     string
		ddgURL   string
		expected string
	}{
		{
			name:     "URL with uddg parameter",
			ddgURL:   "//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com",
			expected: "https://example.com",
		},
		{
			name:     "URL with uddg parameter and other params",
			ddgURL:   "//duckduckgo.com/l/?kh=-1&uddg=https%3A%2F%2Ftest.com%2Fpath",
			expected: "https://test.com/path",
		},
		{
			name:     "URL without uddg parameter returns original",
			ddgURL:   "https://direct-link.com",
			expected: "https://direct-link.com",
		},
		{
			name:     "invalid URL returns original",
			ddgURL:   "not-a-url",
			expected: "not-a-url",
		},
		{
			name:     "empty URL returns empty",
			ddgURL:   "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDuckDuckGoURL(tt.ddgURL)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildGoogleAPIURL(t *testing.T) {
	apiKey := "test-api-key"
	cseID := "test-cse-id"
	query := "test query"
	maxResults := 10

	url := buildGoogleAPIURL(apiKey, cseID, query, maxResults)

	assert.Contains(t, url, "https://www.googleapis.com/customsearch/v1")
	assert.Contains(t, url, "key=test-api-key")
	assert.Contains(t, url, "cx=test-cse-id")
	assert.Contains(t, url, "q=test+query")
	assert.Contains(t, url, "num=10")
}

func TestConvertGoogleResults(t *testing.T) {
	items := []googleItem{
		{
			Title:   "Result 1",
			Link:    "https://example.com/1",
			Snippet: "Description 1",
		},
		{
			Title:   "Result 2",
			Link:    "https://example.com/2",
			Snippet: "Description 2",
		},
	}

	results := convertGoogleResults(items)

	assert.Len(t, results, 2)
	assert.Equal(t, "Result 1", results[0].Title)
	assert.Equal(t, "https://example.com/1", results[0].URL)
	assert.Equal(t, "Description 1", results[0].Description)
	assert.Equal(t, "Result 2", results[1].Title)
	assert.Equal(t, "https://example.com/2", results[1].URL)
	assert.Equal(t, "Description 2", results[1].Description)
}

func TestConvertGoogleResults_Empty(t *testing.T) {
	items := []googleItem{}
	results := convertGoogleResults(items)

	assert.NotNil(t, results)
	assert.Len(t, results, 0)
}

func TestNewSearchEngine_Google(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				WebSearch: schema.AIWebSearchSettings{
					GoogleAPIKey: "test-api-key",
					GoogleCSEID:  "test-cse-id",
				},
			},
		},
	}

	engine := NewSearchEngine(atmosConfig)
	assert.NotNil(t, engine)

	// Verify it's a Google engine by type assertion.
	googleEngine, ok := engine.(*GoogleEngine)
	assert.True(t, ok, "Expected GoogleEngine type")
	assert.Equal(t, "test-api-key", googleEngine.apiKey)
	assert.Equal(t, "test-cse-id", googleEngine.cseID)
}

func TestNewSearchEngine_DuckDuckGo_Default(t *testing.T) {
	// No config - should default to DuckDuckGo
	engine := NewSearchEngine(nil)
	assert.NotNil(t, engine)

	_, ok := engine.(*DuckDuckGoEngine)
	assert.True(t, ok, "Expected DuckDuckGoEngine type when no config provided")
}

func TestNewSearchEngine_DuckDuckGo_NoGoogleConfig(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			AI: schema.AISettings{
				WebSearch: schema.AIWebSearchSettings{
					Enabled: true,
					// No Google API key or CSE ID
				},
			},
		},
	}

	engine := NewSearchEngine(atmosConfig)
	assert.NotNil(t, engine)

	_, ok := engine.(*DuckDuckGoEngine)
	assert.True(t, ok, "Expected DuckDuckGoEngine type when Google config is missing")
}

func TestNewSearchEngine_DuckDuckGo_PartialGoogleConfig(t *testing.T) {
	tests := []struct {
		name      string
		apiKey    string
		cseID     string
		expectDDG bool
	}{
		{
			name:      "missing API key",
			apiKey:    "",
			cseID:     "test-cse-id",
			expectDDG: true,
		},
		{
			name:      "missing CSE ID",
			apiKey:    "test-api-key",
			cseID:     "",
			expectDDG: true,
		},
		{
			name:      "both present",
			apiKey:    "test-api-key",
			cseID:     "test-cse-id",
			expectDDG: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					AI: schema.AISettings{
						WebSearch: schema.AIWebSearchSettings{
							GoogleAPIKey: tt.apiKey,
							GoogleCSEID:  tt.cseID,
						},
					},
				},
			}

			engine := NewSearchEngine(atmosConfig)
			assert.NotNil(t, engine)

			_, isDDG := engine.(*DuckDuckGoEngine)
			if tt.expectDDG {
				assert.True(t, isDDG, "Expected DuckDuckGoEngine")
			} else {
				assert.False(t, isDDG, "Expected GoogleEngine")
			}
		})
	}
}

// TestDuckDuckGoEngine_Search_Integration is an integration test that requires internet.
// It's skipped by default.
func TestDuckDuckGoEngine_Search_Integration(t *testing.T) {
	t.Skip("Skipping integration test that requires internet connection")

	engine := NewDuckDuckGoEngine()
	ctx := context.Background()

	response, err := engine.Search(ctx, "golang", 5)
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, "golang", response.Query)
	assert.GreaterOrEqual(t, response.Count, 1)
	assert.LessOrEqual(t, response.Count, 5)

	if len(response.Results) > 0 {
		assert.NotEmpty(t, response.Results[0].Title)
		assert.NotEmpty(t, response.Results[0].URL)
	}
}

// TestGoogleEngine_Search_Integration is an integration test that requires internet and API keys.
// It's skipped by default.
func TestGoogleEngine_Search_Integration(t *testing.T) {
	t.Skip("Skipping integration test that requires internet connection and API credentials")

	// These would need to be set via environment variables or test config
	apiKey := "YOUR_GOOGLE_API_KEY"
	cseID := "YOUR_GOOGLE_CSE_ID"

	engine := NewGoogleEngine(apiKey, cseID)
	ctx := context.Background()

	response, err := engine.Search(ctx, "golang", 5)
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, "golang", response.Query)
	assert.GreaterOrEqual(t, response.Count, 1)
	assert.LessOrEqual(t, response.Count, 5)

	if len(response.Results) > 0 {
		assert.NotEmpty(t, response.Results[0].Title)
		assert.NotEmpty(t, response.Results[0].URL)
		assert.NotEmpty(t, response.Results[0].Description)
	}
}
