package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// DefaultHTTPTimeout is the default timeout for HTTP requests.
	DefaultHTTPTimeout = 30 * time.Second
	// DefaultMaxResults is the default maximum number of search results.
	DefaultMaxResults = 10
	// DuckDuckGoMaxResults is the maximum number of results for DuckDuckGo.
	DuckDuckGoMaxResults = 50
	// GoogleMaxResults is the maximum number of results for Google CSE API.
	GoogleMaxResults = 10
)

// SearchResult represents a single search result.
type SearchResult struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
}

// SearchResponse contains search results.
type SearchResponse struct {
	Query   string         `json:"query"`
	Results []SearchResult `json:"results"`
	Count   int            `json:"count"`
}

// SearchEngine represents a search engine implementation.
type SearchEngine interface {
	Search(ctx context.Context, query string, maxResults int) (*SearchResponse, error)
}

// DuckDuckGoEngine implements search using DuckDuckGo HTML.
type DuckDuckGoEngine struct {
	client *http.Client
}

// NewDuckDuckGoEngine creates a new DuckDuckGo search engine.
func NewDuckDuckGoEngine() *DuckDuckGoEngine {
	defer perf.Track(nil, "web.NewDuckDuckGoEngine")()

	return &DuckDuckGoEngine{
		client: &http.Client{
			Timeout: DefaultHTTPTimeout,
		},
	}
}

// Search performs a DuckDuckGo HTML search.
func (e *DuckDuckGoEngine) Search(ctx context.Context, query string, maxResults int) (*SearchResponse, error) {
	defer perf.Track(nil, "web.DuckDuckGoEngine.Search")()

	maxResults = normalizeMaxResults(maxResults, DefaultMaxResults, DuckDuckGoMaxResults)

	// Build search URL.
	searchURL := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))

	// Execute HTTP request and get document.
	doc, err := e.fetchAndParse(ctx, searchURL)
	if err != nil {
		return nil, err
	}

	// Extract results from HTML.
	results := extractDuckDuckGoResults(doc, maxResults)

	return &SearchResponse{
		Query:   query,
		Results: results,
		Count:   len(results),
	}, nil
}

// fetchAndParse performs HTTP request and parses HTML response.
func (e *DuckDuckGoEngine) fetchAndParse(ctx context.Context, searchURL string) (*goquery.Document, error) {
	// Create request.
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, wrapSearchError(errUtils.ErrWebSearchFailed, err)
	}

	// Set headers to mimic browser.
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Atmos/1.0; +https://atmos.tools)")

	// Execute request.
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, wrapSearchError(errUtils.ErrWebSearchFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", errUtils.ErrWebSearchFailed, resp.StatusCode)
	}

	// Parse HTML.
	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, wrapSearchError(errUtils.ErrWebSearchParseFailed, err)
	}

	return doc, nil
}

// extractDuckDuckGoResults extracts search results from DuckDuckGo HTML.
func extractDuckDuckGoResults(doc *goquery.Document, maxResults int) []SearchResult {
	var results []SearchResult
	doc.Find(".result").Each(func(i int, s *goquery.Selection) {
		if len(results) >= maxResults {
			return
		}

		// Extract title and URL.
		titleLink := s.Find(".result__a")
		title := strings.TrimSpace(titleLink.Text())
		href, exists := titleLink.Attr("href")

		// Extract description.
		description := strings.TrimSpace(s.Find(".result__snippet").Text())

		if exists && title != "" {
			// Parse DuckDuckGo redirect URL to get actual URL.
			actualURL := parseDuckDuckGoURL(href)

			results = append(results, SearchResult{
				Title:       title,
				URL:         actualURL,
				Description: description,
			})
		}
	})

	return results
}

// parseDuckDuckGoURL extracts the actual URL from DuckDuckGo's redirect URL.
func parseDuckDuckGoURL(ddgURL string) string {
	// DuckDuckGo uses URLs like: //duckduckgo.com/l/?uddg=https%3A%2F%2Fexample.com
	parsedURL, err := url.Parse(ddgURL)
	if err != nil {
		return ddgURL
	}

	// Get the uddg parameter which contains the actual URL.
	if uddg := parsedURL.Query().Get("uddg"); uddg != "" {
		return uddg
	}

	return ddgURL
}

// wrapSearchError wraps an error with a search-related error.
func wrapSearchError(baseErr, err error) error {
	return fmt.Errorf("%w: %w", baseErr, err)
}

// GoogleEngine implements search using Google Custom Search API.
type GoogleEngine struct {
	client *http.Client
	apiKey string
	cseID  string
}

// NewGoogleEngine creates a new Google Custom Search engine.
func NewGoogleEngine(apiKey, cseID string) *GoogleEngine {
	defer perf.Track(nil, "web.NewGoogleEngine")()

	return &GoogleEngine{
		client: &http.Client{
			Timeout: DefaultHTTPTimeout,
		},
		apiKey: apiKey,
		cseID:  cseID,
	}
}

// Search performs a Google Custom Search.
func (e *GoogleEngine) Search(ctx context.Context, query string, maxResults int) (*SearchResponse, error) {
	defer perf.Track(nil, "web.GoogleEngine.Search")()

	maxResults = normalizeMaxResults(maxResults, DefaultMaxResults, GoogleMaxResults)

	// Build API URL.
	apiURL := buildGoogleAPIURL(e.apiKey, e.cseID, query, maxResults)

	// Execute API request and parse response.
	items, err := e.fetchGoogleResults(ctx, apiURL)
	if err != nil {
		return nil, err
	}

	// Convert to SearchResults.
	results := convertGoogleResults(items)

	return &SearchResponse{
		Query:   query,
		Results: results,
		Count:   len(results),
	}, nil
}

// buildGoogleAPIURL constructs the Google Custom Search API URL.
func buildGoogleAPIURL(apiKey, cseID, query string, maxResults int) string {
	return fmt.Sprintf(
		"https://www.googleapis.com/customsearch/v1?key=%s&cx=%s&q=%s&num=%d",
		apiKey,
		cseID,
		url.QueryEscape(query),
		maxResults,
	)
}

// fetchGoogleResults performs the API request and returns the items.
func (e *GoogleEngine) fetchGoogleResults(ctx context.Context, apiURL string) ([]googleItem, error) {
	// Create request.
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, wrapSearchError(errUtils.ErrWebSearchFailed, err)
	}

	// Execute request.
	resp, err := e.client.Do(req)
	if err != nil {
		return nil, wrapSearchError(errUtils.ErrWebSearchFailed, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%w: HTTP %d: %s", errUtils.ErrWebSearchFailed, resp.StatusCode, string(body))
	}

	// Parse JSON response.
	var apiResp struct {
		Items []googleItem `json:"items"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, wrapSearchError(errUtils.ErrWebSearchParseFailed, err)
	}

	return apiResp.Items, nil
}

// googleItem represents a single item from Google Custom Search API.
type googleItem struct {
	Title   string `json:"title"`
	Link    string `json:"link"`
	Snippet string `json:"snippet"`
}

// convertGoogleResults converts Google API items to SearchResults.
func convertGoogleResults(items []googleItem) []SearchResult {
	results := make([]SearchResult, 0, len(items))
	for _, item := range items {
		results = append(results, SearchResult{
			Title:       item.Title,
			URL:         item.Link,
			Description: item.Snippet,
		})
	}
	return results
}

// normalizeMaxResults ensures maxResults is within valid bounds.
func normalizeMaxResults(maxResults, defaultMax, absoluteMax int) int {
	if maxResults <= 0 {
		return defaultMax
	}
	if maxResults > absoluteMax {
		return absoluteMax
	}
	return maxResults
}

// NewSearchEngine creates a search engine based on configuration.
func NewSearchEngine(atmosConfig *schema.AtmosConfiguration) SearchEngine {
	defer perf.Track(atmosConfig, "web.NewSearchEngine")()

	// Check for Google Custom Search configuration.
	if atmosConfig != nil &&
		atmosConfig.Settings.AI.WebSearch.GoogleAPIKey != "" &&
		atmosConfig.Settings.AI.WebSearch.GoogleCSEID != "" {
		return NewGoogleEngine(
			atmosConfig.Settings.AI.WebSearch.GoogleAPIKey,
			atmosConfig.Settings.AI.WebSearch.GoogleCSEID,
		)
	}

	// Default to DuckDuckGo (no API key required).
	return NewDuckDuckGoEngine()
}
