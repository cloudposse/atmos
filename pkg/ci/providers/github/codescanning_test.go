package github

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

func TestWithCategory(t *testing.T) {
	const sarif = `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"checkov"}}}]}`

	t.Run("injects automationDetails.id into each run", func(t *testing.T) {
		out := withCategory([]byte(sarif), "atmos/test/bucket")
		var doc map[string]any
		require.NoError(t, json.Unmarshal(out, &doc))
		runs := doc["runs"].([]any)
		run := runs[0].(map[string]any)
		details := run["automationDetails"].(map[string]any)
		assert.Equal(t, "atmos/test/bucket", details["id"])
	})

	t.Run("empty category returns input unchanged", func(t *testing.T) {
		out := withCategory([]byte(sarif), "")
		assert.Equal(t, sarif, string(out))
	})

	t.Run("unparseable input returns unchanged", func(t *testing.T) {
		bad := []byte("not json")
		assert.Equal(t, bad, withCategory(bad, "cat"))
	})
}

func TestGzipBase64RoundTrip(t *testing.T) {
	original := []byte(`{"version":"2.1.0"}`)
	encoded, err := gzipBase64(original)
	require.NoError(t, err)

	gz, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	r, err := gzip.NewReader(bytes.NewReader(gz))
	require.NoError(t, err)
	got, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, original, got)
}

// captureTransport records the last request body and returns a canned 202.
type captureTransport struct{ body []byte }

func (c *captureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		c.body, _ = io.ReadAll(req.Body)
	}
	return &http.Response{
		StatusCode: http.StatusAccepted,
		Body:       io.NopCloser(bytes.NewBufferString(`{"id":"a1","url":"https://api.github.com/x"}`)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func TestProvider_ReportSARIF_UploadsGzippedCategorizedSARIF(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_REPOSITORY", "acme/widgets")
	t.Setenv("GITHUB_SHA", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")

	ct := &captureTransport{}
	p := NewProviderWithClient(NewClientWithHTTPClient(&http.Client{Transport: ct}))

	const sarif = `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"checkov"}},"results":[]}]}`
	err := p.ReportSARIF(context.Background(), provider.SARIFReport{Body: []byte(sarif), Category: "atmos/bucket"})
	require.NoError(t, err)

	// Decode the upload payload: { commit_sha, ref, sarif: <gzip+base64> }.
	var payload struct {
		CommitSHA string `json:"commit_sha"`
		Sarif     string `json:"sarif"`
	}
	require.NoError(t, json.Unmarshal(ct.body, &payload))
	assert.NotEmpty(t, payload.CommitSHA)

	gz, err := base64.StdEncoding.DecodeString(payload.Sarif)
	require.NoError(t, err)
	r, err := gzip.NewReader(bytes.NewReader(gz))
	require.NoError(t, err)
	raw, err := io.ReadAll(r)
	require.NoError(t, err)

	var doc map[string]any
	require.NoError(t, json.Unmarshal(raw, &doc))
	run := doc["runs"].([]any)[0].(map[string]any)
	details := run["automationDetails"].(map[string]any)
	assert.Equal(t, "atmos/bucket", details["id"], "category must be stamped into the uploaded SARIF")
}
