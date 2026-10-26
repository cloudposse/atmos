package planfile

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/stretchr/testify/require"
)

func TestSavedApplyApprovalGate(t *testing.T) {
	t.Parallel()
	for _, outcome := range []string{"approved", "pending", "unavailable"} {
		t.Run(outcome, func(t *testing.T) {
			t.Parallel()
			directory := t.TempDir()
			source := filepath.Join(directory, "input.plan")
			require.NoError(t, os.WriteFile(source, []byte("binary"), 0o600))
			guard, runner := newTestGuard(t, "true")
			guard.Rules[0].Effects = policy.Effects{Approvals: []policy.ApprovalRequirement{{ID: "platform", Provider: "github-actions", Spec: []byte(`{"reviewers":["alice"],"minimum":1}`)}}}
			expectShow(t, runner, validPlan)
			guard.ResolveApprovals = func(_ context.Context, request *policy.Request, decision policy.Decision) (policy.Decision, error) {
				require.Equal(t, policy.TerraformPlan, request.Kind)
				require.Equal(t, policy.AwaitingApproval, decision.Status)
				if outcome == "unavailable" {
					return decision, errUtils.ErrPolicyInputUnavailable
				}
				var reviews []policy.GitHubReview
				if outcome == "approved" {
					reviews = []policy.GitHubReview{{ID: 1, Login: "alice", State: "APPROVED", CommitID: "head", SubmittedAt: time.Unix(1, 0)}}
				}
				return policy.ResolveGitHubApprovals(decision, "head", "author", reviews)
			}
			called := false
			_, err := guard.Execute(context.Background(), &Invocation{Dir: directory, Args: []string{"apply", source}}, func([]string) process.Result {
				called = true
				return process.Result{Started: true}
			})
			require.Equal(t, outcome == "approved", called)
			if outcome == "approved" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
