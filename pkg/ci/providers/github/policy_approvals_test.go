package github

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/policy"
	"github.com/stretchr/testify/require"
)

func TestPolicyApprovalFreshEvidence(t *testing.T) {
	t.Parallel()
	head, err := atmosgit.GetCurrentCommitSHA()
	require.NoError(t, err)
	for _, tc := range []struct {
		name, state, head string
		status            int
		changed           bool
		want              policy.DecisionStatus
		wantErr           bool
	}{
		{name: "approved paginated", state: "APPROVED", head: head, want: policy.Allow},
		{name: "dismissed", state: "DISMISSED", head: head, want: policy.AwaitingApproval},
		{name: "stale review", state: "APPROVED", head: "old", want: policy.AwaitingApproval},
		{name: "permission denied", status: 403, head: head, wantErr: true},
		{name: "push during pagination", state: "APPROVED", head: head, changed: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			prReads := 0
			reviewReads := 0
			provider := newPRSearchTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/repos/acme/infra/pulls/7":
					prReads++
					current := head
					if tc.changed && prReads > 1 {
						current = "new-head"
					}
					_, _ = fmt.Fprintf(w, `{"number":7,"state":"open","head":{"sha":%q},"base":{"sha":"base","repo":{"full_name":"acme/infra"}},"user":{"login":"author"}}`, current)
				case "/repos/acme/infra/pulls/7/reviews":
					reviewReads++
					if tc.status != 0 {
						w.WriteHeader(tc.status)
						_, _ = fmt.Fprint(w, `{"message":"forbidden"}`)
						return
					}
					if r.URL.Query().Get("page") == "1" {
						w.Header().Set("Link", fmt.Sprintf(`<http://%s/repos/acme/infra/pulls/7/reviews?page=2>; rel="next"`, r.Host))
						_, _ = fmt.Fprint(w, `[{"id":1,"submitted_at":"2026-10-01T00:00:00Z","user":{"login":"alice"},"state":"APPROVED","commit_id":"old"}]`)
					} else {
						_, _ = fmt.Fprintf(w, `[{"id":2,"submitted_at":"2026-10-02T00:00:00Z","user":{"login":"alice"},"state":%q,"commit_id":%q}]`, tc.state, tc.head)
					}
				default:
					t.Errorf("unexpected API path: %s", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			request := approvalTestRequest(t, head)
			decision, err := provider.ResolvePolicyApprovals(context.Background(), request, approvalTestDecision())
			if tc.wantErr {
				require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, decision.Status)
			require.Equal(t, 2, reviewReads)
			require.Equal(t, 2, prReads)
		})
	}
}

func approvalTestDecision() policy.Decision {
	return policy.Decision{Status: policy.AwaitingApproval, Findings: []policy.Finding{{RuleID: "review", Status: policy.AwaitingApproval, Effects: policy.Effects{Approvals: []policy.ApprovalRequirement{{ID: "platform", Provider: ProviderName, Spec: []byte(`{"reviewers":["alice"],"minimum":1}`)}}}}}}
}

func approvalTestRequest(t *testing.T, head string) *policy.Request {
	t.Helper()
	metadata, err := json.Marshal(map[string]string{"GITHUB_REPOSITORY": "acme/infra", "GITHUB_EVENT_NAME": "pull_request"})
	require.NoError(t, err)
	event := fmt.Sprintf(`{"pull_request":{"number":7,"head":{"sha":%q,"repo":{"full_name":"fork/infra"}}}}`, head)
	return &policy.Request{CI: &policy.CISnapshot{
		Provider: ProviderName, Schema: PolicySnapshotSchema,
		Metadata: policy.Dataset{Status: policy.Available, Complete: true, Data: metadata},
		Event:    policy.Dataset{Status: policy.Available, Complete: true, Data: []byte(event)},
	}}
}

func TestPolicyApprovalUnavailable(t *testing.T) {
	t.Parallel()
	provider := NewProvider()
	_, err := provider.ResolvePolicyApprovals(context.Background(), nil, approvalTestDecision())
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	request := approvalTestRequest(t, "stale")
	request.CI.Event.Complete = false
	_, err = provider.ResolvePolicyApprovals(context.Background(), request, approvalTestDecision())
	require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
	decision, err := provider.ResolvePolicyApprovals(context.Background(), nil, policy.Decision{Status: policy.Deny})
	require.NoError(t, err)
	require.Equal(t, policy.Deny, decision.Status)
}

func TestPolicyApprovalGitHubTokenOnly(t *testing.T) {
	clearAPIURLEnv(t)
	t.Setenv("GITHUB_TOKEN", "workflow-token")
	head, err := atmosgit.GetCurrentCommitSHA()
	require.NoError(t, err)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer workflow-token" {
			t.Error("workflow token not sent to configured API")
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/reviews") {
			_, _ = fmt.Fprintf(w, `[{"id":1,"submitted_at":"2026-10-01T00:00:00Z","user":{"login":"alice"},"state":"APPROVED","commit_id":%q}]`, head)
		} else {
			_, _ = fmt.Fprintf(w, `{"state":"open","head":{"sha":%q},"base":{"sha":"base","repo":{"full_name":"acme/infra"}},"user":{"login":"author"}}`, head)
		}
	}))
	defer server.Close()
	t.Setenv("GITHUB_API_URL", server.URL)
	provider := NewProvider()
	require.NoError(t, provider.ensureClient())
	provider.client.tokenTransport.Base = server.Client().Transport
	decision, err := provider.ResolvePolicyApprovals(context.Background(), approvalTestRequest(t, head), approvalTestDecision())
	require.NoError(t, err)
	require.NoError(t, policy.EnforceDecision(decision))
}

func TestPolicyApprovalPaginationFailsClosed(t *testing.T) {
	t.Parallel()
	for _, loop := range []bool{false, true} {
		t.Run(fmt.Sprint(loop), func(t *testing.T) {
			t.Parallel()
			provider := newPRSearchTestProvider(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				page, _ := strconv.Atoi(r.URL.Query().Get("page"))
				next := page + 1
				if loop {
					next = 1
				}
				w.Header().Set("Link", fmt.Sprintf(`<http://%s/repos/acme/infra/pulls/7/reviews?page=%d>; rel="next"`, r.Host, next))
				_, _ = fmt.Fprint(w, `[]`)
			}))
			_, err := provider.collectApprovalReviews(context.Background(), "acme", "infra", 7)
			require.ErrorIs(t, err, errUtils.ErrPolicyInputUnavailable)
		})
	}
}

func TestPolicyApprovalTargetValidation(t *testing.T) {
	t.Parallel()
	for _, event := range []string{`{`, `null`, `{}`, `{"pull_request":{"number":7,"head":{"sha":""}}}`} {
		request := approvalTestRequest(t, "head")
		request.CI.Event.Data = []byte(event)
		_, err := approvalTarget(request)
		require.Error(t, err)
	}
	request := approvalTestRequest(t, "head")
	request.CI.Metadata.Data = []byte(`{"GITHUB_REPOSITORY":"acme/infra","GITHUB_EVENT_NAME":"push"}`)
	_, err := approvalTarget(request)
	require.Error(t, err)
	request = approvalTestRequest(t, "head")
	request.Revisions.Head = "different-checkout"
	require.Error(t, validateApprovalCheckout(request, nil, "head"))
}
