package downloader

import (
	"context"
	"net/http"
	neturl "net/url"
	"strings"

	"github.com/hashicorp/go-getter"

	"github.com/cloudposse/atmos/pkg/auth/broker"
	s3source "github.com/cloudposse/atmos/pkg/downloader/s3"
	"github.com/cloudposse/atmos/pkg/github"
	httpClient "github.com/cloudposse/atmos/pkg/http"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

type goGetterClient struct {
	client   *getter.Client
	metadata *metadataCapturingTransport
}

// Get executes the download.
func (c *goGetterClient) Get() error {
	return c.client.Get()
}

// Metadata returns provenance captured during this download, including the Git
// commit from the checkout before go-getter extracts a requested subdirectory.
func (c *goGetterClient) Metadata() FetchMetadata {
	var metadata FetchMetadata
	if c.metadata != nil {
		metadata = c.metadata.captured
	}
	if c.client != nil {
		if gitGetter, ok := c.client.Getters["git"].(*CustomGitGetter); ok {
			metadata.GitCommit = gitGetter.ResolvedCommit
		}
	}
	return metadata
}

type goGetterClientFactory struct {
	atmosConfig *schema.AtmosConfiguration
	progress    getter.ProgressTracker
	onRetry     func(int)
	retryConfig *schema.RetryConfig
	httpClient  *http.Client // Optional custom HTTP client for testing.
}

// NewClient creates a new `go-getter` client.
func (f *goGetterClientFactory) NewClient(ctx context.Context, src, dest string, mode ClientMode) (DownloadClient, error) {
	clientMode := getter.ClientModeAny

	// Run CustomGitDetector against a "git::"-forced, already-schemed source ourselves: see
	// applyForcedGitDetection's doc comment for why go-getter's own Detect() would otherwise
	// silently skip it, leaving token injection inconsistently applied depending on source shape.
	src = applyForcedGitDetection(ctx, f.atmosConfig, src)

	src, err := prepareOCISource(src)
	if err != nil {
		return nil, err
	}

	// Lazily provision ambient credential brokers (e.g., Atmos Pro github/sts) before the first
	// remote read so private repositories are reachable. Brokers export GIT_CONFIG_*/tokens into
	// the process env, which the git subprocess this client spawns inherits. Process-once and
	// gated (CI + configured), so local-only fetches and non-Pro repos pay nothing.
	remote := f.atmosConfig != nil && isRemoteSource(src)
	if remote {
		broker.EnsureCredentials(ctx, f.atmosConfig)
	}

	// When Atmos brokered a fresh token this process, a freshly minted GitHub token can
	// briefly 401 before GitHub propagates it. Let the git getter retry such auth failures
	// within a bounded window — only for remote, brokered reads, so static credentials still
	// fail fast.
	retryAuthErrors := remote && broker.HasBrokeredCredentials()

	switch mode {
	case ClientModeAny:
		clientMode = getter.ClientModeAny
	case ClientModeDir:
		clientMode = getter.ClientModeDir
	case ClientModeFile:
		clientMode = getter.ClientModeFile
	}

	// Create HTTP getter with optional custom client, always wrapped in a metadataCapturingTransport
	// so a caller can retrieve ETag/Last-Modified after Get() runs (see goGetterClient.Metadata).
	// Header capture must happen during the fetch itself -- there is no way to recover response
	// headers afterward from a staged directory. A caller-supplied test client always wins;
	// otherwise attach a GitHub token when one is available so http(s):// sources with an explicit
	// scheme (e.g. https://raw.githubusercontent.com/...) get authenticated requests too —
	// go-getter's Detect() only runs CustomGitDetector (which already handles token injection) for
	// scheme-less shorthand sources, never for URLs with a scheme.
	capture := &metadataCapturingTransport{}
	httpGetter := &getter.HttpGetter{}
	switch {
	case f.httpClient != nil:
		// Shallow-copy so the caller-owned client (used for testing) is never mutated in place;
		// wrap its existing transport rather than replace it.
		cloned := *f.httpClient
		capture.base = cloned.Transport
		cloned.Transport = capture
		httpGetter.Client = &cloned
	default:
		if token := github.GetGitHubTokenContext(ctx); token != "" {
			// Shallow-copy to preserve CheckRedirect (and any other fields set by
			// NewGitHubAuthenticatedHTTPClient) while wrapping its transport, so header capture
			// still observes the final response.
			cloned := *httpClient.NewGitHubAuthenticatedHTTPClient(token)
			capture.base = cloned.Transport
			cloned.Transport = capture
			httpGetter.Client = &cloned
		} else {
			// Neither a caller-supplied client nor a GitHub token: attach an explicit client
			// wrapping the capturing transport around Go's default transport, so ETag/Last-Modified
			// capture still works instead of falling back to go-getter's own internal default
			// client, which this wrapper could never observe.
			capture.base = http.DefaultTransport
			httpGetter.Client = &http.Client{Transport: capture}
		}
	}

	client := &getter.Client{
		Ctx:              ctx,
		Detectors:        customDetectors(ctx, f.atmosConfig, src),
		ProgressListener: f.progress,
		Src:              src,
		Dst:              dest,
		Mode:             clientMode,
		DisableSymlinks:  false,
		// Warn when an archive carries link entries, which go-getter extracts as empty files.
		Decompressors: linkAuditedDecompressors(),
		Getters: map[string]getter.Getter{
			// Overriding 'git'.
			"git":   &CustomGitGetter{RetryConfig: f.retryConfig, RetryAuthErrors: retryAuthErrors, OnRetry: f.onRetry},
			"file":  &getter.FileGetter{},
			"hg":    &getter.HgGetter{},
			"http":  httpGetter,
			"https": httpGetter,
			"s3":    s3source.NewGetter(ctx),
			"oci":   &OCIGetter{AtmosConfig: f.atmosConfig},
			// "gcs": &getter.GCSGetter{},
		},
	}

	return &goGetterClient{client: client, metadata: capture}, nil
}

// customDetectors creates a private detector list for each client. Global detectors are never mutated.
func customDetectors(ctx context.Context, atmosConfig *schema.AtmosConfiguration, src string) []getter.Detector {
	return append([]getter.Detector{&CustomGitDetector{ctx: ctx, atmosConfig: atmosConfig, source: src}}, getter.Detectors...)
}

// WithProgress reports downloaded bytes without writing to the terminal.
func WithProgress(progress getter.ProgressTracker) GoGetterOption {
	return func(f *goGetterClientFactory) { f.progress = progress }
}

// isRemoteSource reports whether a go-getter source refers to a remote location (so that
// credential brokers should be consulted). It recognizes explicit non-file schemes
// (https://, git::https://, ssh://), SCP-style git URLs (git@github.com:org/repo), and
// scheme-less shorthand for supported hosts (github.com/org/repo). Local and relative file
// paths return false so local-only fetches do not trigger provisioning.
func isRemoteSource(src string) bool {
	s := strings.TrimPrefix(src, GitPrefix)

	// OCI registries authenticate through go-containerregistry (docker keychain, ghcr.io token),
	// never through the git/GitHub credential brokers this check gates.
	if strings.HasPrefix(s, ociScheme) {
		return false
	}

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
	// Scheme-less shorthand carries no port of its own (a trailing ":port" would already have
	// been consumed by the SCP-style check above), so the same portless host is used for both
	// isSupportedHost parameters here.
	lowerHost := strings.ToLower(host)
	return isSupportedHost(lowerHost, lowerHost)
}

// applyForcedGitDetection re-runs CustomGitDetector against a "git::"-forced source whose inner
// URL already carries an explicit scheme (e.g. "git::https://github.com/org/repo.git").
//
// go-getter's own Detect() (detect.go) skips calling every registered Detector -- including
// CustomGitDetector -- whenever the un-forced source already parses with a non-empty URL scheme
// (its "u.Scheme != "" -> return early" check runs before any detector sees the source).
// CustomGitDetector therefore only ever runs, through go-getter's normal Client.Get() dispatch,
// for scheme-less shorthand sources like "github.com/org/repo" -- never for an explicitly-schemed
// "git::https://github.com/..." source, even though that is a documented, common way to force a
// git clone (see CLAUDE.md's go-getter composite-scheme example). The practical effect: token
// injection (and the default depth=1 shallow-clone query) silently never applies to that source
// shape, which looks like inconsistent/"sometimes skipped" injection depending on which form a
// stack manifest, vendor source, or scaffold source happens to use for the same host.
//
// This re-runs the SAME detector directly against the inner (scheme-stripped-of-"git::") URL,
// bypassing go-getter's early return, so a forced git:: GitHub/GitLab/Bitbucket source gets the
// same token injection a scheme-less shorthand source already receives. Only "git::"-forced
// sources are touched here -- a plain "https://github.com/..." source without forcing is routed
// to the http(s) getter instead (e.g. an archive/release download via isGitHubHTTPURL's handling
// in file_downloader.go), and must not be reinterpreted as a git clone target.
func applyForcedGitDetection(ctx context.Context, atmosConfig *schema.AtmosConfiguration, src string) string {
	inner, ok := strings.CutPrefix(src, GitPrefix)
	if !ok {
		return src
	}
	if u, err := neturl.Parse(inner); err != nil || u.Scheme == "" {
		// No explicit scheme: go-getter's own Detect() already runs every Detector for this (its
		// early return only triggers once a scheme is already present), so CustomGitDetector sees
		// it during the normal Client.Get() -> Detect() call. Nothing to do here.
		return src
	}
	result, detected, err := NewCustomGitDetectorContext(ctx, atmosConfig, inner).Detect(inner, "")
	if err != nil || !detected {
		return src
	}
	return result
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

// WithRetryObserver reports actual git retries without owning terminal output.
func WithRetryObserver(fn func(int)) GoGetterOption {
	return func(f *goGetterClientFactory) { f.onRetry = fn }
}
