//go:build mage

package release

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/internal/ci/releasenotes"
)

func TestReleasePreviewUsesAISummariesBeforeSizeCheck(t *testing.T) {
	f := newPreviewFixture()
	for i := 1; i <= 60; i++ {
		f.issues[1] = append(f.issues[1], previewIssue{Number: i, PullRequest: json.RawMessage(`{}`)})
		pr := planPull(i, "patch", nil)
		pr.Title = fmt.Sprintf("Change %d", i)
		pr.Body = strings.Repeat("Original description. ", 100) + "<!-- hidden comment -->"
		pr.Merged = i == 1
		if i == 2 {
			pr.State = previewClosed
		}
		f.pulls[i] = pr
	}
	c := f.client(t)
	c.summaryParams = releasenotes.SummarizeParams{OpenAIAPIKey: "test-key", Model: "test-model"}
	client := releasenotes.NewMockHTTPClient(gomock.NewController(t))
	c.summaryHTTP = client
	client.EXPECT().Do(gomock.Any()).Times(1).DoAndReturn(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, "https://api.openai.com/v1/chat/completions", req.URL.String())
		require.Equal(t, "Bearer test-key", req.Header.Get("Authorization"))
		prompt, err := io.ReadAll(req.Body)
		require.NoError(t, err)
		require.Contains(t, string(prompt), `"model":"test-model"`)
		require.Contains(t, string(prompt), "Original description.")
		require.NotContains(t, string(prompt), "hidden comment")
		summaries := make([]map[string]any, 60)
		for i := range summaries {
			summaries[i] = map[string]any{"number": i + 1, "summary": fmt.Sprintf("Condensed change %d.", i+1)}
		}
		raw, err := json.Marshal(summaries)
		require.NoError(t, err)
		reply, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(raw)}}}})
		require.NoError(t, err)
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(reply)))}, nil
	})
	require.NoError(t, c.previews())
	body := createdPreviewBody(t, f)
	require.Less(t, len(body), previewBodyLimit)
	require.NotContains(t, body, "test-key")
	for i := 1; i <= 60; i++ {
		require.Contains(t, body, fmt.Sprintf("- #%d — Change %d", i, i))
		require.Contains(t, body, fmt.Sprintf("Condensed change %d.", i))
	}
	require.NotContains(t, body, "Original description.")
	require.Contains(t, body, "### 🐛 Bug Fixes")
	require.Contains(t, body, "(merged)")
	require.Contains(t, body, "(closed, unmerged — blocks release)")
	require.Contains(t, body, "(open)")
	slices.Reverse(f.issues[1])
	c.summaryParams.OpenAIAPIKey = "rotated-key"
	fresh := &milestoneClient{Client: c.Client, summaryHTTP: client, summaryParams: c.summaryParams}
	require.NoError(t, fresh.previews())
	require.Len(t, f.writes, 1, "unchanged inputs reuse the saved preview without another model call")
}

func TestReleasePreviewSummaryFailuresLeaveExistingIssueUntouched(t *testing.T) {
	for _, tc := range []struct {
		name, content, want string
		status              int
	}{
		{name: "API failure", status: http.StatusServiceUnavailable, content: `{"error":"unavailable"}`, want: "OpenAI API request failed"},
		{name: "wrong PR", status: http.StatusOK, content: `{"choices":[{"message":{"content":"[{\"number\":99,\"summary\":\"Wrong PR.\"}]"}}]}`, want: "summary does not match entry"},
		{name: "oversized AI summary", status: http.StatusOK, content: previewSummaryResponse(t, strings.Repeat("x", previewBodyLimit)), want: "exceeds issue body limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newPreviewFixture()
			existing := ownedPreview(90, previewOpen)
			f.issues[1] = []previewIssue{existing, {Number: 42, PullRequest: json.RawMessage(`{}`)}}
			f.pulls[42] = planPull(42, "patch", nil)
			c := f.client(t)
			c.summaryParams = releasenotes.SummarizeParams{OpenAIAPIKey: "test-key", Model: "test-model"}
			client := releasenotes.NewMockHTTPClient(gomock.NewController(t))
			c.summaryHTTP = client
			client.EXPECT().Do(gomock.Any()).Return(&http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.content))}, nil)
			require.ErrorContains(t, c.previews(), tc.want)
			require.Empty(t, f.writes)
			require.Empty(t, f.mutations)
			require.Equal(t, existing, f.issues[1][0])
		})
	}
}

func previewSummaryResponse(t *testing.T, summary string) string {
	t.Helper()
	raw, err := json.Marshal([]map[string]any{{"number": 42, "summary": summary}})
	require.NoError(t, err)
	reply, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": string(raw)}}}})
	require.NoError(t, err)
	return string(reply)
}

func TestReleasePreviewEmptyMilestoneSkipsAI(t *testing.T) {
	f := newPreviewFixture()
	c := f.client(t)
	c.summaryParams.OpenAIAPIKey = "test-key"
	c.summaryHTTP = releasenotes.NewMockHTTPClient(gomock.NewController(t))
	require.NoError(t, c.previews())
	require.Contains(t, createdPreviewBody(t, f), "No pull requests assigned yet.")
}

func TestReleasePreviewWorkflowSuppliesSummaryKeyToTrustedCode(t *testing.T) {
	var wf pipelineWorkflow
	pipelineYAML(t, ".github/workflows/release-milestones.yml", &wf)
	require.Equal(t, "release-notes", wf.Jobs["previews"].Environment, "release notes must use a dedicated environment without publishing credentials")
	steps := wf.Jobs["previews"].Steps
	checkout := pipelineUses(t, steps, "actions/checkout")
	require.Equal(t, "${{ github.event.repository.default_branch }}", checkout.With["ref"])
	preview := pipelineUses(t, steps, "./.github/actions/release-preview")
	require.Equal(t, "${{ secrets.OPENAI_API_KEY }}", preview.With["openai-api-key"])
	action := pipelineAction(t, "release-preview")
	run := action[pipelineRunIndex(t, action, "release:previews")]
	require.Equal(t, "${{ inputs.openai-api-key }}", run.Env["OPENAI_API_KEY"])
	require.Equal(t, "${{ inputs.github-token }}", run.Env["GH_TOKEN"])
}
