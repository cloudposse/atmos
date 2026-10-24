package github

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
)

func TestWithCategory(t *testing.T) {
	const sarif = `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"checkov"}}}]}`

	t.Run("injects automationDetails.id into each run", func(t *testing.T) {
		out := withCategory([]byte(sarif), "checkov")
		var doc map[string]any
		require.NoError(t, json.Unmarshal(out, &doc))
		runs := doc["runs"].([]any)
		run := runs[0].(map[string]any)
		details := run["automationDetails"].(map[string]any)
		assert.Equal(t, "checkov/", details["id"])
	})

	t.Run("keeps explicit category id unchanged", func(t *testing.T) {
		out := withCategory([]byte(sarif), "checkov/run-1")
		var doc map[string]any
		require.NoError(t, json.Unmarshal(out, &doc))
		run := doc["runs"].([]any)[0].(map[string]any)
		details := run["automationDetails"].(map[string]any)
		assert.Equal(t, "checkov/run-1", details["id"])
	})

	t.Run("empty category returns input unchanged", func(t *testing.T) {
		out := withCategory([]byte(sarif), "")
		assert.Equal(t, sarif, string(out))
	})

	t.Run("unparseable input returns unchanged", func(t *testing.T) {
		bad := []byte("not json")
		assert.Equal(t, bad, withCategory(bad, "cat"))
	})

	t.Run("runs not an array returns unchanged", func(t *testing.T) {
		bad := []byte(`{"runs":{"not":"an array"}}`)
		assert.Equal(t, bad, withCategory(bad, "cat"))
	})

	t.Run("non-map run entries are skipped", func(t *testing.T) {
		// First run is a bare string (skipped via continue); second is a real run
		// and must still receive the stamped automationDetails.id.
		in := []byte(`{"runs":["not-a-map",{"tool":{"driver":{"name":"kics"}}}]}`)
		out := withCategory(in, "kics")
		var doc map[string]any
		require.NoError(t, json.Unmarshal(out, &doc))
		runs := doc["runs"].([]any)
		assert.Equal(t, "not-a-map", runs[0], "non-map run left untouched")
		details := runs[1].(map[string]any)["automationDetails"].(map[string]any)
		assert.Equal(t, "kics/", details["id"])
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
	err := p.ReportSARIF(context.Background(), provider.SARIFReport{Body: []byte(sarif), Category: "checkov"})
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
	assert.Equal(t, "checkov/", details["id"], "category must be stamped into the uploaded SARIF")
}

// With a usable client but no repository/commit context (GITHUB_REPOSITORY and
// GITHUB_SHA unset), ReportSARIF must fail the precondition check rather than
// attempt an unanchored upload.
func TestProvider_ReportSARIF_MissingRepoContext(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_REPOSITORY", "")
	t.Setenv("GITHUB_SHA", "")

	p := NewProviderWithClient(NewClientWithHTTPClient(&http.Client{Transport: &captureTransport{}}))
	err := p.ReportSARIF(context.Background(), provider.SARIFReport{Body: []byte(`{}`), Category: "checkov"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCISARIFUploadFailed)
}

// errTransport fails every request, standing in for a rejected Code Scanning
// upload (e.g. missing Advanced Security or token scope).
type errTransport struct{}

func (errTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("upload rejected")
}

// A failed UploadSarif call is wrapped in ErrCISARIFUploadFailed.
func TestProvider_ReportSARIF_WrapsUploadFailure(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	t.Setenv("GITHUB_REPOSITORY", "acme/widgets")
	t.Setenv("GITHUB_SHA", "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef")

	p := NewProviderWithClient(NewClientWithHTTPClient(&http.Client{Transport: errTransport{}}))
	const sarif = `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"checkov"}}}]}`
	err := p.ReportSARIF(context.Background(), provider.SARIFReport{Body: []byte(sarif), Category: "checkov"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCISARIFUploadFailed)
}

func TestProvider_ReportSARIF_WrapsClientInitFailure(t *testing.T) {
	t.Setenv("ATMOS_CI_GITHUB_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")

	p := NewProvider()
	err := p.ReportSARIF(context.Background(), provider.SARIFReport{Body: []byte(`{}`)})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCISARIFUploadFailed)
	assert.True(t, errors.Is(err, errUtils.ErrGitHubTokenNotFound), "keeps underlying client init error matchable")
}
