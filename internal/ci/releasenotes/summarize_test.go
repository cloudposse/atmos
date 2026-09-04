package releasenotes

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// skeletonBody is what release-drafter writes with .github/auto-release.yml.
const skeletonBody = "- feat: add widget @alice (#101)\n\n## 🚀 Enhancements\n\n- fix: correct gizmo @bob (#102)\n"

const widgetPRBody = "Adds a widget.\n\n" +
	"<!-- This is an auto-generated comment: release notes by coderabbit.ai -->\n" +
	"## Summary by CodeRabbit\n\n* **New Features**\n  * A widget.\n" +
	"<!-- end of auto-generated comment: release notes by coderabbit.ai -->\n"

func testParams() *SummarizeParams {
	return &SummarizeParams{GHToken: "gh-token", OpenAIAPIKey: "openai-key", Model: "gpt-4o-mini", Release: ReleaseRef{Repo: "cloudposse/atmos", ID: "123"}}
}

// apiScript fakes the three APIs the pipeline touches and records what it
// saw, so tests assert on behavior (which calls happened, what was written)
// rather than call order.
type apiScript struct {
	t            *testing.T
	releaseBody  string
	prBodies     map[string]string
	openAIReply  string
	openAIPrompt string
	patched      string
	calls        map[string]int
}

func newAPIScript(t *testing.T, releaseBody string) *apiScript {
	return &apiScript{
		t:           t,
		releaseBody: releaseBody,
		prBodies:    map[string]string{"101": widgetPRBody, "102": "Fixes the gizmo."},
		openAIReply: `[{"number":101,"summary":"Adds a widget."},{"number":102,"summary":"Fixes the gizmo."}]`,
		calls:       map[string]int{},
	}
}

func (s *apiScript) do(req *http.Request) (*http.Response, error) {
	url := req.URL.String()
	switch {
	case req.Method == http.MethodGet && strings.Contains(url, "/releases/"):
		s.calls["get-release"]++
		return jsonResponse(http.StatusOK, `{"body":`+jsonString(s.releaseBody)+`}`), nil
	case req.Method == http.MethodGet && strings.Contains(url, "/pulls/"):
		s.calls["get-pr"]++
		n := url[strings.LastIndex(url, "/")+1:]
		body, ok := s.prBodies[n]
		if !ok {
			return jsonResponse(http.StatusNotFound, `{"message":"Not Found"}`), nil
		}
		return jsonResponse(http.StatusOK, `{"body":`+jsonString(body)+`}`), nil
	case req.Method == http.MethodPost && strings.Contains(url, "openai.com"):
		s.calls["openai"]++
		raw, err := io.ReadAll(req.Body)
		require.NoError(s.t, err)
		s.openAIPrompt = string(raw)
		return jsonResponse(http.StatusOK, `{"choices":[{"message":{"content":`+jsonString(s.openAIReply)+`}}]}`), nil
	case req.Method == http.MethodPatch && strings.Contains(url, "/releases/"):
		s.calls["patch"]++
		var payload struct {
			Body string `json:"body"`
		}
		require.NoError(s.t, json.NewDecoder(req.Body).Decode(&payload))
		s.patched = payload.Body
		return jsonResponse(http.StatusOK, `{}`), nil
	}
	s.t.Fatalf("unexpected request %s %s", req.Method, url)
	return nil, nil
}

func jsonString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func scriptedClient(t *testing.T, s *apiScript) HTTPClient {
	ctrl := gomock.NewController(t)
	client := NewMockHTTPClient(ctrl)
	client.EXPECT().Do(gomock.Any()).AnyTimes().DoAndReturn(s.do)
	return client
}

func TestSummarizeRelease_SkeletonFetchesPRBodiesAndRewritesAsDetails(t *testing.T) {
	s := newAPIScript(t, skeletonBody)
	res, err := SummarizeRelease(context.Background(), scriptedClient(t, s), testParams())
	require.NoError(t, err)

	assert.Equal(t, Result{Entries: 2, AI: true, Chars: res.Chars}, res)
	assert.Equal(t, 2, s.calls["get-pr"], "one PR fetch per skeleton entry")
	assert.Equal(t, 1, s.calls["openai"])
	assert.Equal(t, 1, s.calls["patch"])

	// The model sees the author's description, not CodeRabbit's block.
	assert.Contains(t, s.openAIPrompt, "Adds a widget.")
	assert.NotContains(t, s.openAIPrompt, "Summary by CodeRabbit")

	// The release gets the org template's shape with the summaries inside.
	assert.Contains(t, s.patched, "<details>\n\n<summary>feat: add widget @alice (#101)</summary>\n\n- Adds a widget.\n\n</details>")
	assert.Contains(t, s.patched, "## 🚀 Enhancements")
	assert.Contains(t, s.patched, "<details>\n\n<summary>fix: correct gizmo @bob (#102)</summary>\n\n- Fixes the gizmo.\n\n</details>")
	assert.Positive(t, res.Chars)
}

func TestSummarizeRelease_WithoutKeyUsesCodeRabbitOrTruncatedDescription(t *testing.T) {
	s := newAPIScript(t, skeletonBody)
	p := testParams()
	p.OpenAIAPIKey = ""

	res, err := SummarizeRelease(context.Background(), scriptedClient(t, s), p)
	require.NoError(t, err)

	assert.False(t, res.AI)
	assert.Equal(t, 0, s.calls["openai"], "no model call without a key")
	assert.Equal(t, 1, s.calls["patch"])
	// #101 has a CodeRabbit summary: that is the fallback, heading dropped.
	assert.Contains(t, s.patched, "<summary>feat: add widget @alice (#101)</summary>\n\n* **New Features**\n  * A widget.\n\n</details>")
	assert.NotContains(t, s.patched, "Summary by CodeRabbit")
	// #102 has none: its (short) description is used as-is.
	assert.Contains(t, s.patched, "<summary>fix: correct gizmo @bob (#102)</summary>\n\n- Fixes the gizmo.\n\n</details>")
}

func TestSummarizeRelease_OrgTemplateBodyNeedsNoPRFetch(t *testing.T) {
	full := "<details>\n  <summary>feat: add widget @alice (#101)</summary>\nAdds a widget.\n</details>\n\n" +
		"## 🚀 Enhancements\n\n<details>\n  <summary>fix: correct gizmo @bob (#102)</summary>\nFixes the gizmo.\n</details>\n"
	s := newAPIScript(t, full)

	res, err := SummarizeRelease(context.Background(), scriptedClient(t, s), testParams())
	require.NoError(t, err)
	assert.Equal(t, 2, res.Entries)
	assert.Equal(t, 0, s.calls["get-pr"], "bodies already present in the draft")
	assert.Equal(t, 1, s.calls["openai"])
}

func TestSummarizeRelease_DryRunWritesBodyWithoutUpdating(t *testing.T) {
	s := newAPIScript(t, skeletonBody)
	p := testParams()
	p.DryRun = true
	var out bytes.Buffer
	p.Output = &out

	_, err := SummarizeRelease(context.Background(), scriptedClient(t, s), p)
	require.NoError(t, err)
	assert.Equal(t, 0, s.calls["patch"])
	assert.Contains(t, out.String(), "Adds a widget.")
	assert.Contains(t, out.String(), "(#102)")
}

func TestSummarizeRelease_DryRunWithNilOutputDiscards(t *testing.T) {
	s := newAPIScript(t, skeletonBody)
	p := testParams()
	p.DryRun = true

	_, err := SummarizeRelease(context.Background(), scriptedClient(t, s), p)
	require.NoError(t, err)
	assert.Equal(t, 0, s.calls["patch"])
}

func TestSummarizeRelease_DegradesToBulletsWhenStillTooLong(t *testing.T) {
	s := newAPIScript(t, skeletonBody)
	huge := strings.Repeat("x", maxReleaseBodyChars/2+1)
	s.openAIReply = `[{"number":101,"summary":"` + huge + `"},{"number":102,"summary":"` + huge + `"}]`

	res, err := SummarizeRelease(context.Background(), scriptedClient(t, s), testParams())
	require.NoError(t, err)
	assert.True(t, res.Degraded)
	assert.LessOrEqual(t, res.Chars, maxReleaseBodyChars)
	assert.NotContains(t, s.patched, "<details>")
	assert.Contains(t, s.patched, "- feat: add widget @alice (#101)")
	assert.Contains(t, s.patched, "## 🚀 Enhancements")
	assert.Contains(t, s.patched, "- fix: correct gizmo @bob (#102)")
}

func TestSummarizeRelease_PropagatesPRFetchError(t *testing.T) {
	s := newAPIScript(t, skeletonBody)
	delete(s.prBodies, "102")

	_, err := SummarizeRelease(context.Background(), scriptedClient(t, s), testParams())
	require.Error(t, err)
	assert.ErrorIs(t, err, errGitHubRequestFailed)
	assert.Contains(t, err.Error(), "PR #102")
}

func TestSummarizeRelease_PropagatesGetReleaseError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockHTTPClient(ctrl)
	client.EXPECT().Do(gomock.Any()).Return(nil, assert.AnError)

	_, err := SummarizeRelease(context.Background(), client, testParams())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get release")
}

func TestSummarizeRelease_PropagatesParseError(t *testing.T) {
	s := newAPIScript(t, "no PR entries here")

	_, err := SummarizeRelease(context.Background(), scriptedClient(t, s), testParams())
	require.Error(t, err)
	assert.ErrorIs(t, err, errNoEntries)
	assert.Contains(t, err.Error(), "parse release")
}

func TestSummarizeRelease_PropagatesSummarizeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	client := NewMockHTTPClient(ctrl)
	s := newAPIScript(t, skeletonBody)
	client.EXPECT().Do(gomock.Any()).AnyTimes().DoAndReturn(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodPost {
			return jsonResponse(http.StatusTooManyRequests, `{"error":"rate limited"}`), nil
		}
		return s.do(req)
	})

	_, err := SummarizeRelease(context.Background(), client, testParams())
	require.Error(t, err)
	assert.ErrorIs(t, err, errOpenAIRequestFailed)
	assert.Equal(t, 0, s.calls["patch"], "a failed summarization must not touch the release")
}

// Sanity check that RenderBody's output keeps the PR metadata a human reads
// release notes for - guards against a rewrite of RenderBody dropping it.
func TestRenderBody_PreservesSummaryMetadata(t *testing.T) {
	entries := []PREntry{{Summary: "feat: add widget @alice (#101)", Number: 101}}
	got, err := RenderBody(entries, []string{"Adds a widget."})
	require.NoError(t, err)
	assert.True(t, strings.Contains(got, "@alice") && strings.Contains(got, "#101"))
}
