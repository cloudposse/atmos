package downloader

import (
	"context"
	"net/http"
	"sync"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/hashicorp/go-getter"
)

// detectorsMutex guards modifications to getter.Detectors.
var detectorsMutex sync.Mutex

type goGetterClient struct {
	client *getter.Client
}

// Get executes the download.
func (c *goGetterClient) Get() error {
	return c.client.Get()
}

type goGetterClientFactory struct {
	atmosConfig *schema.AtmosConfiguration
	retryConfig *schema.RetryConfig
	httpClient  *http.Client // Optional custom HTTP client for testing.
}

// NewClient creates a new `go-getter` client.
func (f *goGetterClientFactory) NewClient(ctx context.Context, src, dest string, mode ClientMode) (DownloadClient, error) {
	clientMode := getter.ClientModeAny
	registerCustomDetectors(f.atmosConfig, src)
	switch mode {
	case ClientModeAny:
		clientMode = getter.ClientModeAny
	case ClientModeDir:
		clientMode = getter.ClientModeDir
	case ClientModeFile:
		clientMode = getter.ClientModeFile
	}

	// Create HTTP getter with optional custom client.
	httpGetter := &getter.HttpGetter{}
	if f.httpClient != nil {
		httpGetter.Client = f.httpClient
	}

	client := &getter.Client{
		Ctx:             ctx,
		Src:             src,
		Dst:             dest,
		Mode:            clientMode,
		DisableSymlinks: false,
		Getters: map[string]getter.Getter{
			// Overriding 'git'.
			"git":   &CustomGitGetter{RetryConfig: f.retryConfig},
			"file":  &getter.FileGetter{},
			"hg":    &getter.HgGetter{},
			"http":  httpGetter,
			"https": httpGetter,
			// "s3": &getter.S3Getter{}, // add as needed.
			// "gcs": &getter.GCSGetter{},
		},
	}

	return &goGetterClient{client: client}, nil
}

// registerCustomDetectors prepends the custom detector so it runs before
// the built-in ones. Any code that calls go-getter should invoke this.
func registerCustomDetectors(atmosConfig *schema.AtmosConfiguration, src string) {
	detectorsMutex.Lock()
	defer detectorsMutex.Unlock()

	getter.Detectors = append(
		[]getter.Detector{
			&CustomGitDetector{atmosConfig: atmosConfig, source: src},
		},
		getter.Detectors...,
	)
}

// GoGetterOption configures the go-getter downloader.
type GoGetterOption func(*goGetterClientFactory)

// WithRetryConfig sets the retry configuration for git operations.
func WithRetryConfig(retryConfig *schema.RetryConfig) GoGetterOption {
	return func(f *goGetterClientFactory) {
		f.retryConfig = retryConfig
	}
}

// WithHTTPClient sets a custom HTTP client for HTTP/HTTPS requests.
// This is useful for testing with mock servers or custom transport configurations.
func WithHTTPClient(client *http.Client) GoGetterOption {
	return func(f *goGetterClientFactory) {
		f.httpClient = client
	}
}

// NewGoGetterDownloader creates a new go-getter based downloader.
func NewGoGetterDownloader(atmosConfig *schema.AtmosConfiguration, opts ...GoGetterOption) FileDownloader {
	factory := &goGetterClientFactory{
		atmosConfig: atmosConfig,
	}
	for _, opt := range opts {
		opt(factory)
	}
	return NewFileDownloader(factory)
}
