package policy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestGitHubApprovalDecisions(t *testing.T) {
	t.Parallel()
	requirement := ApprovalRequirement{ID: "platform", Provider: "github-actions", Spec: []byte(`{"reviewers":["alice","bob"],"minimum":1}`)}
	pending := Decision{Status: AwaitingApproval, Findings: []Finding{{RuleID: "review", Status: AwaitingApproval, Effects: Effects{Approvals: []ApprovalRequirement{requirement}}}}}
	for _, tc := range []struct {
		name    string
		reviews []GitHubReview
		author  string
		want    DecisionStatus
	}{
		{"missing", nil, "author", AwaitingApproval},
		{"approved", []GitHubReview{testGitHubReview(1, "Alice", "APPROVED", "head")}, "author", Allow},
		{"stale", []GitHubReview{testGitHubReview(1, "alice", "APPROVED", "old")}, "author", AwaitingApproval},
		{"self", []GitHubReview{testGitHubReview(1, "alice", "APPROVED", "head")}, "ALICE", AwaitingApproval},
		{"dismissed", []GitHubReview{testGitHubReview(1, "alice", "APPROVED", "head"), testGitHubReview(2, "alice", "DISMISSED", "head")}, "author", AwaitingApproval},
		{"changes requested", []GitHubReview{testGitHubReview(2, "alice", "CHANGES_REQUESTED", "head"), testGitHubReview(1, "alice", "APPROVED", "head")}, "author", AwaitingApproval},
		{"comment preserves", []GitHubReview{testGitHubReview(2, "alice", "COMMENTED", "head"), testGitHubReview(1, "alice", "APPROVED", "head")}, "author", Allow},
		{"unlisted", []GitHubReview{testGitHubReview(1, "mallory", "APPROVED", "head")}, "author", AwaitingApproval},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			decision, err := ResolveGitHubApprovals(pending, "head", tc.author, tc.reviews)
			require.NoError(t, err)
			require.Equal(t, tc.want, decision.Status)
			require.Len(t, pending.Findings[0].Effects.Approvals, 1, "must not mutate evaluated evidence")
			if tc.want == Allow {
				require.NoError(t, EnforceDecision(decision))
			} else {
				require.Error(t, EnforceDecision(decision))
			}
		})
	}
	_, err := ResolveGitHubApprovals(pending, "head", "author", []GitHubReview{testGitHubReview(1, "alice", "UNKNOWN", "head")})
	require.Error(t, err)
	_, err = ResolveGitHubApprovals(pending, "", "author", nil)
	require.Error(t, err)
	denied := Decision{Status: Deny}
	decision, err := ResolveGitHubApprovals(denied, "head", "author", nil)
	require.NoError(t, err)
	require.Equal(t, Deny, decision.Status)
}

func TestGitHubApprovalSpec(t *testing.T) {
	t.Parallel()
	for _, spec := range []string{`{}`, `{"reviewers":["org/team"],"minimum":1}`, `{"reviewers":["Alice","alice"],"minimum":1}`, `{"reviewers":["alice"],"minimum":2}`, `{"reviewers":["alice"],"minimum":0}`, `{"reviewers":["alice"],"minimum":1,"team":"platform"}`} {
		_, err := ParseGitHubApproval(ApprovalRequirement{ID: "x", Provider: "github-actions", Spec: []byte(spec)})
		require.Error(t, err)
	}
}

func testGitHubReview(id int64, login, state, commit string) GitHubReview {
	return GitHubReview{ID: id, Login: login, State: state, CommitID: commit, SubmittedAt: time.Unix(id, 0)}
}

func TestGitHubApprovalSubmissionOrder(t *testing.T) {
	t.Parallel()
	pending := Decision{Status: AwaitingApproval, Findings: []Finding{{Status: AwaitingApproval, Effects: Effects{Approvals: []ApprovalRequirement{{ID: "x", Provider: "github-actions", Spec: []byte(`{"reviewers":["alice"],"minimum":1}`)}}}}}}
	oldDraft := testGitHubReview(1, "alice", "CHANGES_REQUESTED", "head")
	oldDraft.SubmittedAt = time.Unix(10, 0)
	approval := testGitHubReview(2, "alice", "APPROVED", "head")
	decision, err := ResolveGitHubApprovals(pending, "head", "author", []GitHubReview{approval, oldDraft})
	require.NoError(t, err)
	require.Equal(t, AwaitingApproval, decision.Status)
	_, err = ResolveGitHubApprovals(pending, "head", "author", []GitHubReview{approval, approval})
	require.Error(t, err, "duplicate pages cannot be complete evidence")
	approval.SubmittedAt = time.Time{}
	_, err = ResolveGitHubApprovals(pending, "head", "author", []GitHubReview{approval})
	require.Error(t, err, "missing submission time must not grant approval")
}

func TestGitHubApprovalPreservesOtherObligations(t *testing.T) {
	t.Parallel()
	hook := HookRequirement{Ref: "scanner", Event: "after-terraform-plan", Required: true, RequireSuccess: true}
	pending := Decision{Status: AwaitingApproval, Findings: []Finding{{
		Status: AwaitingApproval,
		Effects: Effects{
			Approvals: []ApprovalRequirement{{ID: "platform", Provider: "github-actions", Spec: []byte(`{"reviewers":["alice"],"minimum":1}`)}},
			Hooks:     []HookRequirement{hook},
		},
	}}}
	reviews := []GitHubReview{testGitHubReview(1, "alice", "APPROVED", "head")}
	resolved, err := ResolveGitHubApprovals(pending, "head", "author", reviews)
	require.NoError(t, err)
	require.Equal(t, Allow, resolved.Status)
	require.Empty(t, resolved.Findings[0].Effects.Approvals)
	require.Equal(t, []HookRequirement{hook}, resolved.Findings[0].Effects.Hooks)
	require.ErrorIs(t, EnforceDecision(resolved), errUtils.ErrPolicyRequirements)

	pending.Status = Deny
	pending.Findings = append(pending.Findings, Finding{Status: Deny, Effects: Effects{Deny: true}})
	resolved, err = ResolveGitHubApprovals(pending, "head", "author", reviews)
	require.NoError(t, err)
	require.Equal(t, pending, resolved, "approval cannot override an unrelated denial")
	require.ErrorIs(t, EnforceDecision(resolved), errUtils.ErrPolicyDenied)
}
