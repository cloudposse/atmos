package s3

import (
	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Defaults for streaming uploads.
const (
	// DefaultPartSize is the single-request threshold and multipart part size. S3 requires every
	// part except the last to be at least 5 MiB.
	DefaultPartSize int64 = 8 << 20

	// DefaultConcurrency is the number of multipart parts uploaded at once.
	DefaultConcurrency = 4

	// MaxParts is the S3 limit on parts in one multipart upload.
	MaxParts = 10000
)

// ConditionalMode selects how the store treats conditional writes.
type ConditionalMode int

// Conditional modes.
const (
	// ConditionalAuto sends If-None-Match/If-Match and trusts the endpoint. An endpoint that rejects
	// the headers fails the operation with ErrBlobConditionalUnsupported.
	ConditionalAuto ConditionalMode = iota

	// ConditionalProbe is like Auto, but first proves once per store that the endpoint enforces
	// If-None-Match (see [Store.ProbeConditionalWrites]). Use it for third-party endpoints that
	// might silently ignore the headers.
	ConditionalProbe

	// ConditionalDisabled never sends conditions: any conditional operation fails clearly.
	ConditionalDisabled
)

// settings holds the resolved configuration of a Store.
type settings struct {
	bucket      string
	prefix      string
	tags        map[string]string
	partSize    int64
	concurrency int
	conditional ConditionalMode
	opaqueKeys  bool
	client      ClientConfig
	api         API
}

// Option configures a [Store].
type Option func(*settings)

// WithPrefix prefixes every key with prefix and a slash. Leading and trailing slashes are trimmed.
func WithPrefix(prefix string) Option {
	defer perf.Track(nil, "blob.s3.WithPrefix")()

	return func(s *settings) { s.prefix = trimSlashes(prefix) }
}

// WithRegion sets the AWS region.
func WithRegion(region string) Option {
	defer perf.Track(nil, "blob.s3.WithRegion")()

	return func(s *settings) { s.client.Region = region }
}

// WithEndpoint overrides the S3 endpoint, for example an emulator. Path-style addressing is used
// unless [WithPathStyle] says otherwise.
func WithEndpoint(url string) Option {
	defer perf.Track(nil, "blob.s3.WithEndpoint")()

	return func(s *settings) { s.client.EndpointURL = url }
}

// WithPathStyle forces path-style (true) or virtual-hosted (false) addressing.
func WithPathStyle(pathStyle bool) Option {
	defer perf.Track(nil, "blob.s3.WithPathStyle")()

	return func(s *settings) { s.client.PathStyle = aws.Bool(pathStyle) }
}

// WithIdentity authenticates through an Atmos identity. Credentials resolve lazily on first use and
// an unresolvable identity fails closed.
func WithIdentity(name string, resolver AuthResolver) Option {
	defer perf.Track(nil, "blob.s3.WithIdentity")()

	return func(s *settings) {
		s.client.Identity = name
		s.client.Resolver = resolver
	}
}

// WithHTTPClient replaces the SDK HTTP client.
func WithHTTPClient(client aws.HTTPClient) Option {
	defer perf.Track(nil, "blob.s3.WithHTTPClient")()

	return func(s *settings) { s.client.HTTPClient = client }
}

// WithTags sets default object tags copied onto every object the store writes. Tags passed to an
// individual Put override these.
func WithTags(tags map[string]string) Option {
	defer perf.Track(nil, "blob.s3.WithTags")()

	return func(s *settings) { s.tags = cloneTags(tags) }
}

// WithPartSize sets the multipart part size and the single-request threshold. Real S3 requires at
// least 5 MiB for every part but the last; smaller sizes are for emulators and tests.
func WithPartSize(size int64) Option {
	defer perf.Track(nil, "blob.s3.WithPartSize")()

	return func(s *settings) {
		if size > 0 {
			s.partSize = size
		}
	}
}

// WithConcurrency bounds the multipart parts uploaded at once. Memory use is at most
// (concurrency + 1) part buffers.
func WithConcurrency(workers int) Option {
	defer perf.Track(nil, "blob.s3.WithConcurrency")()

	return func(s *settings) {
		if workers > 0 {
			s.concurrency = workers
		}
	}
}

// WithConditionalMode selects how conditional writes are enforced; see [ConditionalMode].
func WithConditionalMode(mode ConditionalMode) Option {
	defer perf.Track(nil, "blob.s3.WithConditionalMode")()

	return func(s *settings) { s.conditional = mode }
}

// WithOpaqueKeys accepts any non-empty key up to the S3 limit instead of requiring portable
// relative keys. The legacy CI adapter uses it to keep its historical key patterns.
func WithOpaqueKeys() Option {
	defer perf.Track(nil, "blob.s3.WithOpaqueKeys")()

	return func(s *settings) { s.opaqueKeys = true }
}

// WithAPI injects a ready S3 client (or a test double), skipping lazy client construction.
func WithAPI(api API) Option {
	defer perf.Track(nil, "blob.s3.WithAPI")()

	return func(s *settings) { s.api = api }
}

func cloneTags(tags map[string]string) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	out := make(map[string]string, len(tags))
	for k, v := range tags {
		out[k] = v
	}
	return out
}

func trimSlashes(s string) string {
	for len(s) > 0 && s[0] == '/' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
