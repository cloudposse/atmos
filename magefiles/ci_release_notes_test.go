//go:build mage

package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/internal/ci/releasenotes"
)

// jsonResponse is a 200 OK with the given body; the non-200 paths are
// covered in internal/ci/releasenotes, not here.
func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     http.StatusText(http.StatusOK),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func testParams(apiKey, ghToken string) *releasenotes.SummarizeParams {
	return &releasenotes.SummarizeParams{
		GHToken:      ghToken,
		OpenAIAPIKey: apiKey,
		Model:        defaultSummaryModel,
		Release:      releasenotes.ReleaseRef{Repo: "cloudposse/atmos", ID: "1"},
	}
}

// fakeAPIs answers the skeleton release, one PR, the model, and the update.
func fakeAPIs(t *testing.T) (releasenotes.HTTPClient, map[string]int) {
	ctrl := gomock.NewController(t)
	client := releasenotes.NewMockHTTPClient(ctrl)
	calls := map[string]int{}
	client.EXPECT().Do(gomock.Any()).AnyTimes().DoAndReturn(func(req *http.Request) (*http.Response, error) {
		url := req.URL.String()
		switch {
		case req.Method == http.MethodGet && strings.Contains(url, "/releases/"):
			calls["get-release"]++
			return jsonResponse(`{"body":"- feat: x @a (#1)\n"}`), nil
		case req.Method == http.MethodGet && strings.Contains(url, "/pulls/1"):
			calls["get-pr"]++
			return jsonResponse(`{"body":"Does x in detail."}`), nil
		case req.Method == http.MethodPost:
			calls["openai"]++
			return jsonResponse(`{"choices":[{"message":{"content":"[{\"number\":1,\"summary\":\"Does x.\"}]"}}]}`), nil
		case req.Method == http.MethodPatch:
			calls["patch"]++
			return jsonResponse(`{}`), nil
		}
		t.Fatalf("unexpected request %s %s", req.Method, url)
		return nil, nil
	})
	return client, calls
}

func TestSummarizeNotes_SkipsWithoutGitHubToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := releasenotes.NewMockHTTPClient(ctrl) // no .Do() expectation: must not be called.

	var stderr bytes.Buffer
	summarizeNotes(context.Background(), &stderr, client, testParams("openai-key", ""))
	assert.Contains(t, stderr.String(), "GITHUB_TOKEN not set")
}

func TestSummarizeNotes_WithoutAPIKeyStillRewritesWithFallback(t *testing.T) {
	client, calls := fakeAPIs(t)

	var stderr bytes.Buffer
	summarizeNotes(context.Background(), &stderr, client, testParams("", "gh-token"))
	assert.Contains(t, stderr.String(), "OPENAI_API_KEY not set")
	assert.Contains(t, stderr.String(), "release notes rewritten (1 entries")
	assert.Contains(t, stderr.String(), "fallback summaries")
	assert.Equal(t, 0, calls["openai"])
	assert.Equal(t, 1, calls["patch"])
}

func TestSummarizeNotes_WithAPIKeyUsesModel(t *testing.T) {
	client, calls := fakeAPIs(t)

	var stderr bytes.Buffer
	summarizeNotes(context.Background(), &stderr, client, testParams("openai-key", "gh-token"))
	assert.Contains(t, stderr.String(), "release notes rewritten (1 entries")
	assert.Contains(t, stderr.String(), "model summaries")
	assert.Equal(t, 1, calls["openai"])
	assert.Equal(t, 1, calls["patch"])
}

func TestSummarizeNotes_DryRunPrintsBodyAndSkipsUpdate(t *testing.T) {
	client, calls := fakeAPIs(t)

	var stdout, stderr bytes.Buffer
	params := testParams("openai-key", "gh-token")
	params.DryRun = true
	params.Output = &stdout
	summarizeNotes(context.Background(), &stderr, client, params)
	assert.Contains(t, stdout.String(), "Does x.")
	assert.Contains(t, stderr.String(), "dry run: release not updated")
	assert.Equal(t, 0, calls["patch"])
}

func TestSummarizeNotes_FailureIsWarnedNotReturned(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := releasenotes.NewMockHTTPClient(ctrl)
	client.EXPECT().Do(gomock.Any()).Return(nil, assert.AnError)

	var stderr bytes.Buffer
	summarizeNotes(context.Background(), &stderr, client, testParams("openai-key", "gh-token"))
	assert.Contains(t, stderr.String(), "::warning::release notes summarization skipped")
}
