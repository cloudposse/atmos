//go:build mage

package release

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/internal/ci/releasenotes"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
)

func TestDraftNotesAreFormattedOnceFromOriginalPullRequest(t *testing.T) {
	p := preparedFixture()
	p.Entries[0].Summary = "Fix `config` & paths @contributor (#42)"
	var draft releasenotes.ReleaseContent
	c := milestoneHTTP(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&draft))
		githubtest.JSON(t, w, githubRelease{ID: 1, Tag: p.Tag, Commit: p.Commit, Draft: true})
	})
	_, err := c.createDraft(p)
	require.NoError(t, err)

	client := releasenotes.NewMockHTTPClient(gomock.NewController(t))
	prReads := 0
	var published releasenotes.ReleaseContent
	client.EXPECT().Do(gomock.Any()).AnyTimes().DoAndReturn(func(r *http.Request) (*http.Response, error) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/releases/1"):
			body, marshalErr := json.Marshal(draft)
			require.NoError(t, marshalErr)
			return jsonResponse(string(body)), nil
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/pulls/42"):
			prReads++
			return jsonResponse(`{"body":"Original description for the summary."}`), nil
		case r.Method == http.MethodPatch:
			require.NoError(t, json.NewDecoder(r.Body).Decode(&published))
			body, marshalErr := json.Marshal(published)
			require.NoError(t, marshalErr)
			return jsonResponse(string(body)), nil
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
			return nil, nil
		}
	})
	_, err = releasenotes.SummarizeRelease(context.Background(), client, testParams("", "gh-token"))
	require.NoError(t, err)
	require.Contains(t, published.Body, "<summary>Fix <code>config</code> &amp; paths @contributor (#42)</summary>")
	require.Equal(t, 1, prReads, "summarize the original PR, not a previously condensed body")
	require.Contains(t, published.Body, "- Original description for the summary.")
	require.Equal(t, p.Tag, published.TagName)
}
