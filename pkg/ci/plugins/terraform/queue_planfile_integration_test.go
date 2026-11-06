package terraform

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	ghprovider "github.com/cloudposse/atmos/pkg/ci/providers/github"
)

// Exercise the real GitHub resolver and real bundled local storage together.
// API fixtures model active and rebuilt queues; no Atmos Pro configuration or
// credentials participate in plan selection.
func TestQueuePlanfileResolutionWithGitHubProvider(t *testing.T) {
	for _, active := range []bool{true, false} {
		t.Run(fmt.Sprintf("active_entry=%t", active), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/repos/owner/repo/git/commits/" + mergeCommitSHA:
					fmt.Fprint(w, `{"tree":{"sha":"queue-tree"}}`)
				case "/graphql":
					entrySHA := mergeCommitSHA
					if !active {
						entrySHA = "rebuilt-queue-commit"
					}
					fmt.Fprintf(w, `{"data":{"repository":{"pullRequest":{"number":290,"headRefOid":%q,"mergeQueueEntry":{"headCommit":{"oid":%q}}}}}}`, prHeadSHA, entrySHA)
				default:
					t.Errorf("unexpected source lookup: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			t.Setenv("ATMOS_CI_GITHUB_TOKEN", "test-token")
			t.Setenv("ATMOS_CI_GITHUB_API_URL", server.URL+"/")
			store := newLocalPlanfileStore(t)
			info := mergedDeployInfo(t)
			storePlan(t, store, info, prHeadSHA, "reviewed-pr-plan")
			ctx := mergedDeployContext(info, ghprovider.NewProvider(), store)
			ctx.CICtx.MergeQueue = &provider.MergeQueueInfo{PullRequestNumber: 290}
			ctx.CICtx.EventName = "merge_group"
			err := (&Plugin{}).downloadPlanfileForVerification(ctx)
			if active {
				require.NoError(t, err)
				assert.Equal(t, "reviewed-pr-plan", readStoredPlan(t, info))
				assert.Equal(t, mergeCommitSHA, ctx.CICtx.SHA)
			} else {
				require.ErrorIs(t, err, errUtils.ErrArtifactNotFound)
				assert.Empty(t, info.StoredPlanFile)
			}
		})
	}
}
