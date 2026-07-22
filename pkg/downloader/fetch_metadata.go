package downloader

import (
	"net/http"

	"github.com/cloudposse/atmos/pkg/perf"
)

// FetchMetadata is best-effort HTTP cache metadata captured during a fetch, for provenance only.
// It is never treated as a stronger identity than a real digest/checksum -- see ResolvedArtifact's
// own doc comment ("Cache metadata deliberately has no role in integrity verification").
type FetchMetadata struct {
	ETag         string
	LastModified string
}

// metadataCapturingTransport wraps an http.RoundTripper, recording the ETag/Last-Modified headers
// of the last response it sees. Scoped to a single fetch (one goGetterClient instance owns one of
// these) -- not intended for a shared/long-lived http.Client across multiple fetches.
type metadataCapturingTransport struct {
	base     http.RoundTripper
	captured FetchMetadata
}

// RoundTrip implements http.RoundTripper interface.
func (t *metadataCapturingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	defer perf.Track(nil, "downloader.metadataCapturingTransport.RoundTrip")()

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err == nil && resp != nil {
		if etag := resp.Header.Get("ETag"); etag != "" {
			t.captured.ETag = etag
		}
		if modified := resp.Header.Get("Last-Modified"); modified != "" {
			t.captured.LastModified = modified
		}
	}
	return resp, err
}
