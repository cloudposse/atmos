package downloader

import (
	"net/http"

	"github.com/cloudposse/atmos/pkg/perf"
)

// FetchMetadata records provenance from the download itself.
// HTTP cache headers are hints, not integrity checks; GitCommit identifies the checked-out content.
type FetchMetadata struct {
	GitCommit    string
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
