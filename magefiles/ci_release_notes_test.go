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

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func testParams(apiKey, ghToken string) *releasenotes.SummarizeParams {
	return &releasenotes.SummarizeParams{
		GHToken:      ghToken,
		OpenAIAPIKey: apiKey,
		Model:        "gpt-5.6-luna",
		Release:      releasenotes.ReleaseRef{Repo: "cloudposse/atmos", ID: "1"},
	}
}

func TestSummarizeNotes_SkipsWithoutAPIKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := releasenotes.NewMockHTTPClient(ctrl) // no .Do() expectation: must not be called.

	var stderr bytes.Buffer
	summarizeNotes(context.Background(), &stderr, client, testParams("", "gh-token"))
	assert.Contains(t, stderr.String(), "OPENAI_API_KEY not set")
}

func TestSummarizeNotes_SkipsWithoutGitHubToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := releasenotes.NewMockHTTPClient(ctrl) // no .Do() expectation: must not be called.

	var stderr bytes.Buffer
	summarizeNotes(context.Background(), &stderr, client, testParams("openai-key", ""))
	assert.Contains(t, stderr.String(), "GITHUB_TOKEN not set")
}

func TestSummarizeNotes_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := releasenotes.NewMockHTTPClient(ctrl)

	calls := 0
	client.EXPECT().Do(gomock.Any()).Times(3).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		calls++
		switch calls {
		case 1:
			return jsonResponse(http.StatusOK, `{"body":"<details>\n  <summary>feat: x @a (#1)</summary>\nbody\n</details>\n"}`), nil
		case 2:
			return jsonResponse(http.StatusOK, `{"choices":[{"message":{"content":"[{\"number\":1,\"summary\":\"Does x.\"}]"}}]}`), nil
		default:
			return jsonResponse(http.StatusOK, `{}`), nil
		}
	})

	var stderr bytes.Buffer
	summarizeNotes(context.Background(), &stderr, client, testParams("openai-key", "gh-token"))
	assert.Contains(t, stderr.String(), "release notes summarized")
}

func TestSummarizeNotes_FailureIsWarnedNotReturned(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := releasenotes.NewMockHTTPClient(ctrl)
	client.EXPECT().Do(gomock.Any()).Return(nil, assert.AnError)

	var stderr bytes.Buffer
	summarizeNotes(context.Background(), &stderr, client, testParams("openai-key", "gh-token"))
	assert.Contains(t, stderr.String(), "::warning::release notes summarization skipped")
}
