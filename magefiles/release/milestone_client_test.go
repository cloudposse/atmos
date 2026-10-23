//go:build mage

package release

import (
	"context"
	"net/http"
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
	"github.com/stretchr/testify/require"
)

func milestoneHTTP(t *testing.T, handler http.HandlerFunc) *milestoneClient {
	t.Helper()
	githubtest.Server(t, handler)
	c, err := newMilestoneClient(context.Background(), githubtest.Repo)
	require.NoError(t, err)
	c.watcher = stubReleaseWatcher{}
	return c
}

func stableRelease(tag string) map[string]any {
	return map[string]any{"tag_name": tag, "published_at": "2026-10-01T00:00:00Z"}
}

func policyPR(label string, milestone *Milestone) ReleasePR {
	pr := ReleasePR{Number: 42, State: "open", Milestone: milestone}
	pr.Labels = append(pr.Labels, struct {
		Name string `json:"name"`
	}{label})
	return pr
}
