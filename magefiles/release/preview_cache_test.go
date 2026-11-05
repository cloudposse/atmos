//go:build mage

package release

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/internal/ci/releasenotes"
)

func TestReleasePreviewRefreshesChangedInputs(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*previewFixture, *milestoneClient)
		want   string
	}{
		{name: "description", change: func(f *previewFixture, _ *milestoneClient) {
			pr := f.pulls[42]
			pr.Body = "New behavior."
			f.pulls[42] = pr
		}},
		{name: "title", change: func(f *previewFixture, _ *milestoneClient) {
			pr := f.pulls[42]
			pr.Title = "New title"
			f.pulls[42] = pr
		}, want: "New title"},
		{name: "closed PR", change: func(f *previewFixture, _ *milestoneClient) {
			pr := f.pulls[42]
			pr.State = previewClosed
			f.pulls[42] = pr
		}, want: "closed, unmerged"},
		{name: "merged PR", change: func(f *previewFixture, _ *milestoneClient) { pr := f.pulls[42]; pr.Merged = true; f.pulls[42] = pr }, want: "(merged)"},
		{name: "category", change: func(f *previewFixture, _ *milestoneClient) {
			pr := f.pulls[42]
			pr.Labels[0].Name = "minor"
			f.pulls[42] = pr
		}, want: "### ✨ Features"},
		{name: "model", change: func(_ *previewFixture, c *milestoneClient) { c.summaryParams.Model = "another-model" }},
		{name: "version", change: func(f *previewFixture, _ *milestoneClient) { f.milestones[0].Title = "1.235.0" }, want: "Upcoming release v1.235.0"},
		{name: "milestone closure", change: func(f *previewFixture, _ *milestoneClient) { f.milestones[0].State = previewClosed }, want: milestoneClosedMarker},
		{name: "edited saved body", change: func(f *previewFixture, _ *milestoneClient) {
			f.issues[1][1].Body = strings.ReplaceAll(f.issues[1][1].Body, "Original AI note.", "Edited note.")
		}},
		{name: "oversized saved body", change: func(f *previewFixture, _ *milestoneClient) {
			f.issues[1][1].Body += strings.Repeat("x", previewBodyLimit)
		}},
		{name: "missing marker", change: func(f *previewFixture, _ *milestoneClient) {
			body := f.issues[1][1].Body
			f.issues[1][1].Body = body[:strings.LastIndex(body, previewCachePrefix)]
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, c, client := previewCacheFixture(t)
			expectPreviewSummary(t, client, "Original AI note.")
			require.NoError(t, c.previews())
			tc.change(f, c)
			expectPreviewSummary(t, client, "Updated AI note.")
			require.NoError(t, c.previews())
			body := createdPreviewBody(t, f)
			require.Contains(t, body, "Updated AI note.")
			require.NotContains(t, body, "Original AI note.")
			if tc.want != "" {
				require.Contains(t, body, tc.want)
			}
			require.Len(t, f.writes, 2)
			require.NoError(t, c.previews())
			require.Len(t, f.writes, 2, "refreshed preview must also be reusable")
		})
	}
}

func TestReleasePreviewRefreshesSummaryModeAndMembership(t *testing.T) {
	f, c, client := previewCacheFixture(t)
	c.summaryParams.OpenAIAPIKey = ""
	require.NoError(t, c.previews())
	require.Contains(t, createdPreviewBody(t, f), "Original description.")
	c.summaryParams.OpenAIAPIKey = "test-key"
	expectPreviewSummary(t, client, "AI note.")
	require.NoError(t, c.previews())
	require.Contains(t, createdPreviewBody(t, f), "AI note.")
	// Removing the only PR must replace the saved preview without calling the model.
	f.issues[1] = f.issues[1][1:]
	require.NoError(t, c.previews())
	require.Contains(t, createdPreviewBody(t, f), "No pull requests assigned yet.")
	require.NotContains(t, createdPreviewBody(t, f), "- #42")
	f.issues[1] = append(f.issues[1], previewIssue{Number: 42, PullRequest: json.RawMessage(`{}`)})
	expectPreviewSummary(t, client, "Added back.")
	require.NoError(t, c.previews())
	require.Contains(t, createdPreviewBody(t, f), "Added back.")
	require.Len(t, f.writes, 4)
}

func TestReleasePreviewCachePreservesManualClosureAndRepairsTitle(t *testing.T) {
	f, c, client := previewCacheFixture(t)
	expectPreviewSummary(t, client, "AI note.")
	require.NoError(t, c.previews())
	body := createdPreviewBody(t, f)
	f.issues[1][1].State = previewClosed
	f.issues[1][1].Title = "Edited title"
	require.NoError(t, c.previews())
	require.Equal(t, body, createdPreviewBody(t, f))
	require.Equal(t, previewClosed, f.issues[1][1].State)
	require.Equal(t, "Release v1.234.0", f.issues[1][1].Title)
	require.Len(t, f.writes, 2)
	require.Equal(t, []string{"pin I_90", "unpin I_90"}, f.mutations)
}

func previewCacheFixture(t *testing.T) (*previewFixture, *milestoneClient, *releasenotes.MockHTTPClient) {
	t.Helper()
	f := newPreviewFixture()
	f.issues[1] = []previewIssue{{Number: 42, PullRequest: json.RawMessage(`{}`)}}
	pr := planPull(42, "patch", nil)
	pr.Body = "Original description."
	pr.Merged = false
	pr.State = previewOpen
	f.pulls[42] = pr
	c := f.client(t)
	c.summaryParams = releasenotes.SummarizeParams{OpenAIAPIKey: "test-key", Model: "test-model"}
	client := releasenotes.NewMockHTTPClient(gomock.NewController(t))
	c.summaryHTTP = client
	return f, c, client
}

func expectPreviewSummary(t *testing.T, client *releasenotes.MockHTTPClient, summary string) {
	t.Helper()
	client.EXPECT().Do(gomock.Any()).Return(&http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(previewSummaryResponse(t, summary)))}, nil)
}
