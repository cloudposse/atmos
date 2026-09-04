package releasenotes

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestSummarizeRelease(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockHTTPClient(ctrl)

	calls := 0
	client.EXPECT().Do(gomock.Any()).Times(3).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		calls++
		switch calls {
		case 1: // GetReleaseBody.
			assert.Equal(t, http.MethodGet, req.Method)
			assert.Contains(t, req.URL.String(), "/releases/123")
			return jsonResponse(http.StatusOK, `{"body":"<details>\n  <summary>feat: x @a (#1)</summary>\nbody\n</details>\n"}`), nil
		case 2: // Summarize.
			assert.Equal(t, "https://api.openai.com/v1/chat/completions", req.URL.String())
			return jsonResponse(http.StatusOK, `{"choices":[{"message":{"content":"[{\"number\":1,\"summary\":\"Does x.\"}]"}}]}`), nil
		case 3: // UpdateReleaseBody.
			assert.Equal(t, http.MethodPatch, req.Method)
			body, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			assert.Contains(t, string(body), "Does x.")
			return jsonResponse(http.StatusOK, `{}`), nil
		default:
			t.Fatalf("unexpected call %d", calls)
			return nil, nil
		}
	})

	err := SummarizeRelease(context.Background(), client, &SummarizeParams{GHToken: "gh-token", OpenAIAPIKey: "openai-key", Model: "gpt-4o-mini", Release: ReleaseRef{Repo: "cloudposse/atmos", ID: "123"}})
	require.NoError(t, err)
}

func TestSummarizeRelease_DryRunWritesBodyWithoutUpdating(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockHTTPClient(ctrl)

	calls := 0
	// Exactly two calls: the release GET and the OpenAI request. A third
	// (the PATCH) would fail the Times(2) expectation.
	client.EXPECT().Do(gomock.Any()).Times(2).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return jsonResponse(http.StatusOK, `{"body":"<details>\n  <summary>feat: x @a (#1)</summary>\nbody\n</details>\n"}`), nil
		}
		assert.NotEqual(t, http.MethodPatch, req.Method)
		return jsonResponse(http.StatusOK, `{"choices":[{"message":{"content":"[{\"number\":1,\"summary\":\"Does x.\"}]"}}]}`), nil
	})

	var out bytes.Buffer
	err := SummarizeRelease(context.Background(), client, &SummarizeParams{
		GHToken: "gh-token", OpenAIAPIKey: "openai-key", Model: "gpt-4o-mini",
		Release: ReleaseRef{Repo: "cloudposse/atmos", ID: "123"},
		DryRun:  true, Output: &out,
	})
	require.NoError(t, err)
	assert.Contains(t, out.String(), "Does x.")
	assert.Contains(t, out.String(), "#1")
}

func TestSummarizeRelease_DryRunWithNilOutputDiscards(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockHTTPClient(ctrl)

	calls := 0
	client.EXPECT().Do(gomock.Any()).Times(2).DoAndReturn(func(_ *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return jsonResponse(http.StatusOK, `{"body":"<details>\n  <summary>feat: x @a (#1)</summary>\nbody\n</details>\n"}`), nil
		}
		return jsonResponse(http.StatusOK, `{"choices":[{"message":{"content":"[{\"number\":1,\"summary\":\"Does x.\"}]"}}]}`), nil
	})

	err := SummarizeRelease(context.Background(), client, &SummarizeParams{
		GHToken: "gh-token", OpenAIAPIKey: "openai-key", Model: "gpt-4o-mini",
		Release: ReleaseRef{Repo: "cloudposse/atmos", ID: "123"},
		DryRun:  true,
	})
	require.NoError(t, err)
}

func TestSummarizeRelease_PropagatesGetReleaseError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockHTTPClient(ctrl)
	client.EXPECT().Do(gomock.Any()).Return(nil, assert.AnError)

	err := SummarizeRelease(context.Background(), client, &SummarizeParams{GHToken: "gh-token", OpenAIAPIKey: "openai-key", Model: "gpt-4o-mini", Release: ReleaseRef{Repo: "cloudposse/atmos", ID: "123"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get release")
}

func TestSummarizeRelease_PropagatesParseError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockHTTPClient(ctrl)
	client.EXPECT().Do(gomock.Any()).Return(jsonResponse(http.StatusOK, `{"body":"no PR entries here"}`), nil) //nolint:bodyclose // closed by the code under test, not this fixture.

	err := SummarizeRelease(context.Background(), client, &SummarizeParams{GHToken: "gh-token", OpenAIAPIKey: "openai-key", Model: "gpt-4o-mini", Release: ReleaseRef{Repo: "cloudposse/atmos", ID: "123"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse release")
}

func TestSummarizeRelease_PropagatesSummarizeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockHTTPClient(ctrl)

	calls := 0
	client.EXPECT().Do(gomock.Any()).Times(2).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return jsonResponse(http.StatusOK, `{"body":"<details>\n  <summary>feat: x @a (#1)</summary>\nbody\n</details>\n"}`), nil
		}
		return jsonResponse(http.StatusTooManyRequests, `{"error":"rate limited"}`), nil
	})

	err := SummarizeRelease(context.Background(), client, &SummarizeParams{GHToken: "gh-token", OpenAIAPIKey: "openai-key", Model: "gpt-4o-mini", Release: ReleaseRef{Repo: "cloudposse/atmos", ID: "123"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OpenAI returned")
}

// Sanity check that RenderBody's output is itself round-trippable by
// ParseDraftedBody's *summary line* extraction (the bullet still carries the
// title/author/number release-drafter produced, just condensed) - guards
// against a future rewrite of RenderBody accidentally dropping the PR
// metadata a human reads release notes for.
func TestRenderBody_PreservesSummaryMetadata(t *testing.T) {
	entries := []PREntry{{Summary: "feat: add widget @alice (#101)", Number: 101}}
	got, err := RenderBody(entries, []string{"Adds a widget."})
	require.NoError(t, err)
	assert.True(t, strings.Contains(got, "@alice") && strings.Contains(got, "#101"))
}
