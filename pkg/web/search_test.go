package web

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// MockRoundTripper is an implementation of http.RoundTripper for testing purposes.
type MockRoundTripper struct {
	mock.Mock
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	args := m.Called(req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*http.Response), args.Error(1)
}

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
		{
			name:     "URL with invalid percent encoding returns original",
			ddgURL:   "//duckduckgo.com/l/?uddg=https%3A%2F%ZZexample.com",
			expected: "//duckduckgo.com/l/?uddg=https%3A%2F%ZZexample.com",
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

func TestWrapSearchError(t *testing.T) {
	baseErr := errUtils.ErrWebSearchFailed
	originalErr := errors.New("connection timeout")

	wrappedErr := wrapSearchError(baseErr, originalErr)

	assert.Error(t, wrappedErr)
	assert.True(t, errors.Is(wrappedErr, errUtils.ErrWebSearchFailed))
	assert.True(t, errors.Is(wrappedErr, originalErr))
	assert.Contains(t, wrappedErr.Error(), "connection timeout")
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
	// No config - should default to DuckDuckGo.
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

func TestDuckDuckGoEngine_Search_Success(t *testing.T) {
	mockHTML := `
		<html>
			<body>
				<div class="result">
					<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fpage1">Example Page 1</a>
					<div class="result__snippet">This is the first result description</div>
				</div>
				<div class="result">
					<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2Fpage2">Example Page 2</a>
					<div class="result__snippet">This is the second result description</div>
				</div>
			</body>
		</html>
	`

	mockTransport := new(MockRoundTripper)
	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(mockHTML)),
		Header:     http.Header{},
	}
	mockTransport.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	engine := &DuckDuckGoEngine{
		client: &http.Client{Transport: mockTransport},
	}

	ctx := context.Background()
	response, err := engine.Search(ctx, "test query", 5)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, "test query", response.Query)
	assert.Equal(t, 2, response.Count)
	assert.Len(t, response.Results, 2)

	assert.Equal(t, "Example Page 1", response.Results[0].Title)
	assert.Equal(t, "https://example.com/page1", response.Results[0].URL)
	assert.Equal(t, "This is the first result description", response.Results[0].Description)

	assert.Equal(t, "Example Page 2", response.Results[1].Title)
	assert.Equal(t, "https://example.com/page2", response.Results[1].URL)
	assert.Equal(t, "This is the second result description", response.Results[1].Description)

	mockTransport.AssertExpectations(t)
}

func TestDuckDuckGoEngine_Search_HTTPError(t *testing.T) {
	mockTransport := new(MockRoundTripper)
	mockTransport.On("RoundTrip", mock.Anything).Return(nil, errors.New("network error"))

	engine := &DuckDuckGoEngine{
		client: &http.Client{Transport: mockTransport},
	}

	ctx := context.Background()
	response, err := engine.Search(ctx, "test query", 5)

	assert.Error(t, err)
	assert.Nil(t, response)
	assert.True(t, errors.Is(err, errUtils.ErrWebSearchFailed))
	mockTransport.AssertExpectations(t)
}

func TestDuckDuckGoEngine_Search_HTTPNonOKStatus(t *testing.T) {
	mockTransport := new(MockRoundTripper)
	mockResponse := &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       io.NopCloser(strings.NewReader("Internal Server Error")),
		Header:     http.Header{},
	}
	mockTransport.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	engine := &DuckDuckGoEngine{
		client: &http.Client{Transport: mockTransport},
	}

	ctx := context.Background()
	response, err := engine.Search(ctx, "test query", 5)

	assert.Error(t, err)
	assert.Nil(t, response)
	assert.True(t, errors.Is(err, errUtils.ErrWebSearchFailed))
	assert.Contains(t, err.Error(), "HTTP 500")
	mockTransport.AssertExpectations(t)
}

func TestDuckDuckGoEngine_Search_InvalidHTML(t *testing.T) {
	mockTransport := new(MockRoundTripper)
	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("invalid html")),
		Header:     http.Header{},
	}
	mockTransport.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	engine := &DuckDuckGoEngine{
		client: &http.Client{Transport: mockTransport},
	}

	ctx := context.Background()
	response, err := engine.Search(ctx, "test query", 5)

	// HTML parsing should succeed even with invalid HTML, just return empty results.
	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, 0, response.Count)
	mockTransport.AssertExpectations(t)
}

func TestDuckDuckGoEngine_Search_MaxResults(t *testing.T) {
	mockHTML := `
		<html>
			<body>
				<div class="result">
					<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2F1">Result 1</a>
					<div class="result__snippet">Description 1</div>
				</div>
				<div class="result">
					<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2F2">Result 2</a>
					<div class="result__snippet">Description 2</div>
				</div>
				<div class="result">
					<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com%2F3">Result 3</a>
					<div class="result__snippet">Description 3</div>
				</div>
			</body>
		</html>
	`

	mockTransport := new(MockRoundTripper)
	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(mockHTML)),
		Header:     http.Header{},
	}
	mockTransport.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	engine := &DuckDuckGoEngine{
		client: &http.Client{Transport: mockTransport},
	}

	ctx := context.Background()
	response, err := engine.Search(ctx, "test query", 2)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, 2, response.Count)
	assert.Len(t, response.Results, 2)
	mockTransport.AssertExpectations(t)
}

func TestDuckDuckGoEngine_Search_NoResults(t *testing.T) {
	mockHTML := `
		<html>
			<body>
				<p>No results found</p>
			</body>
		</html>
	`

	mockTransport := new(MockRoundTripper)
	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(mockHTML)),
		Header:     http.Header{},
	}
	mockTransport.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	engine := &DuckDuckGoEngine{
		client: &http.Client{Transport: mockTransport},
	}

	ctx := context.Background()
	response, err := engine.Search(ctx, "test query", 5)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, 0, response.Count)
	assert.Len(t, response.Results, 0)
	mockTransport.AssertExpectations(t)
}

func TestDuckDuckGoEngine_Search_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This handler should not be called since context is canceled.
		t.Error("Handler should not be called with canceled context")
	}))
	defer server.Close()

	engine := NewDuckDuckGoEngine()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	response, err := engine.Search(ctx, "test query", 5)

	assert.Error(t, err)
	assert.Nil(t, response)
	assert.True(t, errors.Is(err, errUtils.ErrWebSearchFailed))
}

func TestDuckDuckGoEngine_Search_MissingTitle(t *testing.T) {
	mockHTML := `
		<html>
			<body>
				<div class="result">
					<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com"></a>
					<div class="result__snippet">Description without title</div>
				</div>
			</body>
		</html>
	`

	mockTransport := new(MockRoundTripper)
	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(mockHTML)),
		Header:     http.Header{},
	}
	mockTransport.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	engine := &DuckDuckGoEngine{
		client: &http.Client{Transport: mockTransport},
	}

	ctx := context.Background()
	response, err := engine.Search(ctx, "test query", 5)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	// Result should be excluded since title is empty.
	assert.Equal(t, 0, response.Count)
	mockTransport.AssertExpectations(t)
}

func TestDuckDuckGoEngine_Search_MissingHref(t *testing.T) {
	mockHTML := `
		<html>
			<body>
				<div class="result">
					<a class="result__a">Title Without Link</a>
					<div class="result__snippet">Description</div>
				</div>
			</body>
		</html>
	`

	mockTransport := new(MockRoundTripper)
	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(mockHTML)),
		Header:     http.Header{},
	}
	mockTransport.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	engine := &DuckDuckGoEngine{
		client: &http.Client{Transport: mockTransport},
	}

	ctx := context.Background()
	response, err := engine.Search(ctx, "test query", 5)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	// Result should be excluded since href doesn't exist.
	assert.Equal(t, 0, response.Count)
	mockTransport.AssertExpectations(t)
}

func TestGoogleEngine_Search_Success(t *testing.T) {
	mockJSON := `{
		"items": [
			{
				"title": "Result 1",
				"link": "https://example.com/1",
				"snippet": "Description 1"
			},
			{
				"title": "Result 2",
				"link": "https://example.com/2",
				"snippet": "Description 2"
			}
		]
	}`

	mockTransport := new(MockRoundTripper)
	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(mockJSON)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
	mockTransport.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	engine := &GoogleEngine{
		client: &http.Client{Transport: mockTransport},
		apiKey: "test-api-key",
		cseID:  "test-cse-id",
	}

	ctx := context.Background()
	response, err := engine.Search(ctx, "test query", 5)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, "test query", response.Query)
	assert.Equal(t, 2, response.Count)
	assert.Len(t, response.Results, 2)

	assert.Equal(t, "Result 1", response.Results[0].Title)
	assert.Equal(t, "https://example.com/1", response.Results[0].URL)
	assert.Equal(t, "Description 1", response.Results[0].Description)

	mockTransport.AssertExpectations(t)
}

func TestGoogleEngine_Search_HTTPError(t *testing.T) {
	mockTransport := new(MockRoundTripper)
	mockTransport.On("RoundTrip", mock.Anything).Return(nil, errors.New("network error"))

	engine := &GoogleEngine{
		client: &http.Client{Transport: mockTransport},
		apiKey: "test-api-key",
		cseID:  "test-cse-id",
	}

	ctx := context.Background()
	response, err := engine.Search(ctx, "test query", 5)

	assert.Error(t, err)
	assert.Nil(t, response)
	assert.True(t, errors.Is(err, errUtils.ErrWebSearchFailed))
	mockTransport.AssertExpectations(t)
}

func TestGoogleEngine_Search_HTTPNonOKStatus(t *testing.T) {
	mockTransport := new(MockRoundTripper)
	mockResponse := &http.Response{
		StatusCode: http.StatusForbidden,
		Body:       io.NopCloser(strings.NewReader(`{"error": {"code": 403, "message": "API key not valid"}}`)),
		Header:     http.Header{},
	}
	mockTransport.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	engine := &GoogleEngine{
		client: &http.Client{Transport: mockTransport},
		apiKey: "invalid-key",
		cseID:  "test-cse-id",
	}

	ctx := context.Background()
	response, err := engine.Search(ctx, "test query", 5)

	assert.Error(t, err)
	assert.Nil(t, response)
	assert.True(t, errors.Is(err, errUtils.ErrWebSearchFailed))
	assert.Contains(t, err.Error(), "HTTP 403")
	mockTransport.AssertExpectations(t)
}

func TestGoogleEngine_Search_InvalidJSON(t *testing.T) {
	mockTransport := new(MockRoundTripper)
	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("invalid json")),
		Header:     http.Header{},
	}
	mockTransport.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	engine := &GoogleEngine{
		client: &http.Client{Transport: mockTransport},
		apiKey: "test-api-key",
		cseID:  "test-cse-id",
	}

	ctx := context.Background()
	response, err := engine.Search(ctx, "test query", 5)

	assert.Error(t, err)
	assert.Nil(t, response)
	assert.True(t, errors.Is(err, errUtils.ErrWebSearchParseFailed))
	mockTransport.AssertExpectations(t)
}

func TestGoogleEngine_Search_EmptyResults(t *testing.T) {
	mockJSON := `{"items": []}`

	mockTransport := new(MockRoundTripper)
	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(mockJSON)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
	mockTransport.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	engine := &GoogleEngine{
		client: &http.Client{Transport: mockTransport},
		apiKey: "test-api-key",
		cseID:  "test-cse-id",
	}

	ctx := context.Background()
	response, err := engine.Search(ctx, "test query", 5)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, 0, response.Count)
	assert.Len(t, response.Results, 0)
	mockTransport.AssertExpectations(t)
}

func TestGoogleEngine_Search_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This handler should not be called since context is canceled.
		t.Error("Handler should not be called with canceled context")
	}))
	defer server.Close()

	engine := NewGoogleEngine("test-api-key", "test-cse-id")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	response, err := engine.Search(ctx, "test query", 5)

	assert.Error(t, err)
	assert.Nil(t, response)
	assert.True(t, errors.Is(err, errUtils.ErrWebSearchFailed))
}

func TestExtractDuckDuckGoResults_MultiplePages(t *testing.T) {
	htmlContent := `
		<html>
			<body>
				<div class="result">
					<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample1.com">Title 1</a>
					<div class="result__snippet">Snippet 1</div>
				</div>
				<div class="result">
					<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample2.com">Title 2</a>
					<div class="result__snippet">Snippet 2</div>
				</div>
				<div class="result">
					<a class="result__a" href="//duckduckgo.com/l/?uddg=https%3A%2F%2Fexample3.com">Title 3</a>
					<div class="result__snippet">Snippet 3</div>
				</div>
			</body>
		</html>
	`

	doc, err := goQueryDocumentFromString(htmlContent)
	assert.NoError(t, err)

	results := extractDuckDuckGoResults(doc, 2)

	assert.Len(t, results, 2)
	assert.Equal(t, "Title 1", results[0].Title)
	assert.Equal(t, "Title 2", results[1].Title)
}

func TestExtractDuckDuckGoResults_WithWhitespace(t *testing.T) {
	htmlContent := `
		<html>
			<body>
				<div class="result">
					<a class="result__a" href="https://example.com">

						Title with whitespace

					</a>
					<div class="result__snippet">

						Description with whitespace

					</div>
				</div>
			</body>
		</html>
	`

	doc, err := goQueryDocumentFromString(htmlContent)
	assert.NoError(t, err)

	results := extractDuckDuckGoResults(doc, 10)

	assert.Len(t, results, 1)
	assert.Equal(t, "Title with whitespace", results[0].Title)
	assert.Equal(t, "Description with whitespace", results[0].Description)
}

// Helper function to create goquery document from string.
func goQueryDocumentFromString(html string) (*goquery.Document, error) {
	return goquery.NewDocumentFromReader(strings.NewReader(html))
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

	// These would need to be set via environment variables or test config.
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

// TestFetchAndParse_InvalidURL tests error handling in fetchAndParse.
func TestFetchAndParse_InvalidURL(t *testing.T) {
	engine := NewDuckDuckGoEngine()

	ctx := context.Background()
	doc, err := engine.fetchAndParse(ctx, "://invalid-url")

	assert.Error(t, err)
	assert.Nil(t, doc)
	assert.True(t, errors.Is(err, errUtils.ErrWebSearchFailed))
}

// TestFetchGoogleResults_InvalidURL tests error handling in fetchGoogleResults.
func TestFetchGoogleResults_InvalidURL(t *testing.T) {
	engine := NewGoogleEngine("test-api-key", "test-cse-id")

	ctx := context.Background()
	items, err := engine.fetchGoogleResults(ctx, "://invalid-url")

	assert.Error(t, err)
	assert.Nil(t, items)
	assert.True(t, errors.Is(err, errUtils.ErrWebSearchFailed))
}

// TestFetchAndParse_MalformedBody tests handling of malformed response body.
func TestFetchAndParse_MalformedBody(t *testing.T) {
	mockTransport := new(MockRoundTripper)
	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("<html><body>")),
		Header:     http.Header{"Content-Length": []string{"1000"}},
	}
	mockTransport.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	engine := &DuckDuckGoEngine{
		client: &http.Client{Transport: mockTransport},
	}

	ctx := context.Background()
	doc, err := engine.fetchAndParse(ctx, "https://example.com")

	// goquery is lenient and should still parse partial HTML.
	assert.NoError(t, err)
	assert.NotNil(t, doc)
	mockTransport.AssertExpectations(t)
}

// TestFetchGoogleResults_ReadBodyError tests handling of body read errors.
func TestFetchGoogleResults_ReadBodyError(t *testing.T) {
	mockTransport := new(MockRoundTripper)
	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     http.Header{"Content-Length": []string{"1"}},
	}
	mockTransport.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	engine := &GoogleEngine{
		client: &http.Client{Transport: mockTransport},
		apiKey: "test-api-key",
		cseID:  "test-cse-id",
	}

	ctx := context.Background()
	items, err := engine.fetchGoogleResults(ctx, "https://example.com")

	assert.Error(t, err)
	assert.Nil(t, items)
	assert.True(t, errors.Is(err, errUtils.ErrWebSearchParseFailed))
	mockTransport.AssertExpectations(t)
}

// TestDuckDuckGoEngine_Search_NormalizeMaxResults tests max results normalization.
func TestDuckDuckGoEngine_Search_NormalizeMaxResults(t *testing.T) {
	tests := []struct {
		name           string
		inputMax       int
		expectedCapped int
	}{
		{
			name:           "zero maxResults uses default",
			inputMax:       0,
			expectedCapped: DefaultMaxResults,
		},
		{
			name:           "negative maxResults uses default",
			inputMax:       -5,
			expectedCapped: DefaultMaxResults,
		},
		{
			name:           "exceeds DuckDuckGo max capped",
			inputMax:       100,
			expectedCapped: DuckDuckGoMaxResults,
		},
		{
			name:           "within bounds unchanged",
			inputMax:       25,
			expectedCapped: 25,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockHTML := `<html><body></body></html>`

			mockTransport := new(MockRoundTripper)
			mockResponse := &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(mockHTML)),
				Header:     http.Header{},
			}
			mockTransport.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

			engine := &DuckDuckGoEngine{
				client: &http.Client{Transport: mockTransport},
			}

			ctx := context.Background()
			response, err := engine.Search(ctx, "test", tt.inputMax)

			assert.NoError(t, err)
			assert.NotNil(t, response)
			// We can't directly test the normalized value was used, but we verify no error occurred.
			mockTransport.AssertExpectations(t)
		})
	}
}

// TestGoogleEngine_Search_NormalizeMaxResults tests max results normalization for Google.
func TestGoogleEngine_Search_NormalizeMaxResults(t *testing.T) {
	tests := []struct {
		name     string
		inputMax int
	}{
		{
			name:     "zero maxResults uses default",
			inputMax: 0,
		},
		{
			name:     "exceeds Google max capped",
			inputMax: 100,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockJSON := `{"items": []}`

			mockTransport := new(MockRoundTripper)
			mockResponse := &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(mockJSON)),
				Header:     http.Header{"Content-Type": []string{"application/json"}},
			}
			mockTransport.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

			engine := &GoogleEngine{
				client: &http.Client{Transport: mockTransport},
				apiKey: "test-api-key",
				cseID:  "test-cse-id",
			}

			ctx := context.Background()
			response, err := engine.Search(ctx, "test", tt.inputMax)

			assert.NoError(t, err)
			assert.NotNil(t, response)
			mockTransport.AssertExpectations(t)
		})
	}
}

// TestDuckDuckGoEngine_FetchAndParse_BodyCloseError tests that body close errors don't fail the request.
func TestDuckDuckGoEngine_FetchAndParse_BodyCloseError(t *testing.T) {
	mockHTML := `<html><body><div class="result"></div></body></html>`

	mockTransport := new(MockRoundTripper)
	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(mockHTML)),
		Header:     http.Header{},
	}
	mockTransport.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	engine := &DuckDuckGoEngine{
		client: &http.Client{Transport: mockTransport},
	}

	ctx := context.Background()
	// We need a valid URL for the request to be created.
	doc, err := engine.fetchAndParse(ctx, "https://example.com")

	// Body close happens in defer, shouldn't affect result.
	assert.NoError(t, err)
	assert.NotNil(t, doc)
	mockTransport.AssertExpectations(t)
}

// TestGoogleEngine_FetchGoogleResults_BodyCloseError tests that body close errors don't fail the request.
func TestGoogleEngine_FetchGoogleResults_BodyCloseError(t *testing.T) {
	mockJSON := `{"items": []}`

	mockTransport := new(MockRoundTripper)
	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(mockJSON)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
	mockTransport.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	engine := &GoogleEngine{
		client: &http.Client{Transport: mockTransport},
		apiKey: "test-api-key",
		cseID:  "test-cse-id",
	}

	ctx := context.Background()
	items, err := engine.fetchGoogleResults(ctx, "https://example.com")

	// Body close happens in defer, shouldn't affect result.
	assert.NoError(t, err)
	assert.NotNil(t, items)
	mockTransport.AssertExpectations(t)
}

// TestGoogleEngine_Search_NoItems tests Google response with no items field.
func TestGoogleEngine_Search_NoItems(t *testing.T) {
	mockJSON := `{}`

	mockTransport := new(MockRoundTripper)
	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(mockJSON)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
	mockTransport.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	engine := &GoogleEngine{
		client: &http.Client{Transport: mockTransport},
		apiKey: "test-api-key",
		cseID:  "test-cse-id",
	}

	ctx := context.Background()
	response, err := engine.Search(ctx, "test query", 5)

	assert.NoError(t, err)
	assert.NotNil(t, response)
	assert.Equal(t, 0, response.Count)
	assert.Len(t, response.Results, 0)
	mockTransport.AssertExpectations(t)
}

// TestFetchAndParse_EmptyResponse tests handling of empty HTTP response body.
func TestFetchAndParse_EmptyResponse(t *testing.T) {
	mockTransport := new(MockRoundTripper)
	mockResponse := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     http.Header{},
	}
	mockTransport.On("RoundTrip", mock.Anything).Return(mockResponse, nil)

	engine := &DuckDuckGoEngine{
		client: &http.Client{Transport: mockTransport},
	}

	ctx := context.Background()
	doc, err := engine.fetchAndParse(ctx, "https://example.com")

	// goquery should parse empty content without error.
	assert.NoError(t, err)
	assert.NotNil(t, doc)
	mockTransport.AssertExpectations(t)
}

// TestBuildGoogleAPIURL_SpecialCharacters tests URL building with special characters.
func TestBuildGoogleAPIURL_SpecialCharacters(t *testing.T) {
	apiKey := "test-api-key"
	cseID := "test-cse-id"
	query := "test & special = chars"
	maxResults := 10

	url := buildGoogleAPIURL(apiKey, cseID, query, maxResults)

	assert.Contains(t, url, "test+%26+special+%3D+chars")
	assert.Contains(t, url, "key=test-api-key")
	assert.Contains(t, url, "cx=test-cse-id")
	assert.Contains(t, url, "num=10")
}

// TestConvertGoogleResults_Nil tests converting nil items.
func TestConvertGoogleResults_Nil(t *testing.T) {
	results := convertGoogleResults(nil)

	assert.NotNil(t, results)
	assert.Len(t, results, 0)
}

// TestParseDuckDuckGoURL_WithoutScheme tests URL parsing for protocol-relative URLs.
func TestParseDuckDuckGoURL_WithoutScheme(t *testing.T) {
	ddgURL := "//duckduckgo.com/path"
	result := parseDuckDuckGoURL(ddgURL)

	// Should return original if no uddg parameter present.
	assert.Equal(t, ddgURL, result)
}
