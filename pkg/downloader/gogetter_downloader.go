package downloader

import (
	"context"
	"net/http"
	"strings"
	"sync"

	"github.com/hashicorp/go-getter"

	"github.com/cloudposse/atmos/pkg/auth/broker"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
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

	// Lazily provision ambient credential brokers (e.g., Atmos Pro github/sts) before the first
	// remote read so private repositories are reachable. Brokers export GIT_CONFIG_*/tokens into
	// the process env, which the git subprocess this client spawns inherits. Process-once and
	// gated (CI + configured), so local-only fetches and non-Pro repos pay nothing.
	if f.atmosConfig != nil && isRemoteSource(src) {
		broker.EnsureCredentials(ctx, f.atmosConfig)
	}

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

// isRemoteSource reports whether a go-getter source refers to a remote location (so that
// credential brokers should be consulted). It recognizes explicit non-file schemes
// (https://, git::https://, ssh://), SCP-style git URLs (git@github.com:org/repo), and
// scheme-less shorthand for supported hosts (github.com/org/repo). Local and relative file
// paths return false so local-only fetches do not trigger provisioning.
func isRemoteSource(src string) bool {
	s := strings.TrimPrefix(src, GitPrefix)

	if i := strings.Index(s, schemeSeparator); i >= 0 {
		return !strings.EqualFold(s[:i], "file")
	}

	if _, rewritten := rewriteSCPURL(s); rewritten {
		return true
	}

	// Scheme-less shorthand such as "github.com/org/repo[//subdir]".
	host := s
	if idx := strings.IndexAny(host, "/?#"); idx >= 0 {
		host = host[:idx]
	}
	if idx := strings.IndexByte(host, ':'); idx >= 0 {
		host = host[:idx]
	}
	return isSupportedHost(strings.ToLower(host))
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
	defer perf.Track(atmosConfig, "pkg.downloader.NewGoGetterDownloader")()

	factory := &goGetterClientFactory{
		atmosConfig: atmosConfig,
	}
	for _, opt := range opts {
		opt(factory)
	}
	return NewFileDownloader(factory)
}
