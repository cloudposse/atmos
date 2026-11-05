//go:build mage

package qualitygate

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/magefiles/internal/githubtest"
)

func TestQualityGuidanceRespondsToActualState(t *testing.T) {
	for _, tc := range []struct {
		name       string
		gate       qualityGate
		problem    string
		action     string
		notAction  string
		signalName string
	}{
		{"CI pending", qualityCIGate(map[string]string{"Build (macos)": "in_progress", "Tests": "success"}), "1 CI results are still pending; no failed results", "Wait for the listed", "Fix failing tests", "Build (macos)"},
		{"CI failed", qualityCIGate(map[string]string{"Lint": "failure", "Tests": "queued"}), "1 CI results are unsuccessful; 1 are still pending", "Open the logs for the unsuccessful", "No test or code fix", "Lint"},
		{"CI absent", qualityCIGate(nil), "No CI results", "Wait for the PR workflows to start", "Fix failing tests", ""},
		{"CI awaiting approval", qualityCIGate(map[string]string{"workflow: tests.yml": "action_required", "Lint": "success"}), "1 CI results are waiting for a maintainer to approve their workflow runs; 0 are still pending", "Approve the waiting workflow runs", "Open the logs", "workflow: tests.yml"},
		{"CI failed and awaiting approval", qualityCIGate(map[string]string{"Lint": "failure", "workflow: tests.yml": "action_required", "Build": "queued"}), "1 CI results are unsuccessful; 1 are waiting for workflow approval; 1 are still pending", "Approve the waiting workflow runs", "no failed results", "Lint"},
		{"check run requires action", qualityCIGate(map[string]string{"check: codeql/analyze": "action_required"}), "1 CI results are unsuccessful; 0 are still pending", "Open the logs", "Approve the waiting workflow runs", "check: codeql/analyze"},
		{"coverage absent", qualityCoverageGate(nil), "coverage verdict is unknown", "Wait for this commit's coverage uploads", "add tests", ""},
		{"coverage pending", qualityCoverageGate(map[string]string{"codecov/patch": "pending"}), "still processing", "Wait for the pending Codecov", "add tests", "codecov/patch"},
		{"coverage failed", qualityCoverageGate(map[string]string{"codecov/patch": "failure"}), "non-passing result", "identify the uncovered changed lines", "Wait for the pending", "codecov/patch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.gate.Name, tc.gate.URL = tc.name, "https://github.com/cloudposse/atmos/pull/42/checks"
			tc.gate.Remediation = "Fallback policy must not replace a state-specific action."
			pr := githubtest.PR(githubtest.Run())
			body := qualityGuidance(&pr, []qualityGate{tc.gate}, "")
			require.Contains(t, body, tc.problem)
			require.Contains(t, body, tc.action)
			require.NotContains(t, body, tc.notAction)
			require.NotContains(t, body, "Fallback policy")
			if tc.signalName != "" {
				require.Contains(t, body, tc.signalName)
			}
		})
	}
}

func TestQualitySignalsAreEvidenceAndFailuresComeFirst(t *testing.T) {
	states := map[string]string{"A pending": "waiting", "Z failed": "failure", "Passed": "success", "Neutral": "neutral"}
	signals := qualityBlockingSignals(states, true)
	require.Equal(t, []qualitySignal{{Name: "Z failed", State: "failure"}, {Name: "A pending", State: "waiting"}}, signals)
	malicious := qualitySignal{Name: "```\n</details>\n@team", State: "failure"}
	evidence := qualitySignalEvidence([]qualitySignal{malicious})
	require.NotContains(t, evidence, "```")
	require.NotContains(t, evidence, "</details>")
	require.Contains(t, evidence, `\u0060\u0060\u0060\n\u003c/details\u003e`)
	require.Contains(t, qualitySignalEvidence(make([]qualitySignal, 8)), "2 more results")
	require.Empty(t, qualitySignalEvidence(nil))
}

func TestQualityReviewActionsDistinguishMissingApprovalFromChanges(t *testing.T) {
	for _, tc := range []struct{ state, problem, action string }{
		{"", "has not approved", "Wait for CodeRabbit"},
		{"CHANGES_REQUESTED", "requested changes", "Address the findings"},
		{"DISMISSED", "was dismissed", "Request a new CodeRabbit review"},
	} {
		gate := qualityGate{Detail: tc.state}
		qualityReviewGuidance(&gate)
		require.Contains(t, gate.Problem, tc.problem)
		require.Contains(t, gate.NextAction, tc.action)
		require.Contains(t, gate.NextAction, "https://docs.coderabbit.ai/guides/commands#manually-request-code-reviews")
		require.NotContains(t, gate.NextAction, "@coderabbitai")
		require.NotContains(t, gate.NextAction, "@coderabbitai approve")
		require.NotContains(t, gate.NextAction, "@coderabbitai resolve")
	}
	gate := qualityGate{Passed: true}
	qualityReviewGuidance(&gate)
	require.Empty(t, gate.NextAction)
}

func TestQualityReviewFallbackRequestsReviewWithoutBypassingFindings(t *testing.T) {
	registry, err := defaultQualityRegistry()
	require.NoError(t, err)
	for i := range registry.definitions {
		definition := &registry.definitions[i]
		if definition.ID == "review" {
			require.Contains(t, definition.Remediation, "https://docs.coderabbit.ai/guides/commands#manually-request-code-reviews")
			require.NotContains(t, definition.Remediation, "@coderabbitai")
			require.NotContains(t, definition.Remediation, "@coderabbitai approve")
			require.NotContains(t, definition.Remediation, "@coderabbitai resolve")
			return
		}
	}
	t.Fatal("review gate missing from registry")
}

func TestQualityCommentUsesInlineBadgeAndOmitsEmptyPerformance(t *testing.T) {
	t.Setenv("GITHUB_RUN_ID", "12345")
	t.Setenv("GITHUB_STEP_SUMMARY", "")
	pr := githubtest.PR(githubtest.Run())
	for _, passed := range []bool{true, false} {
		body, ready := qualityMarkdown(&pr, []qualityGate{{Name: "CI", Passed: passed, Problem: "A specific blocker."}}, "Performance measurements pending for this commit.")
		require.Equal(t, passed, ready)
		require.Contains(t, body, "<h2>Quality Gate &nbsp; <a href=")
		heading, _, _ := strings.Cut(body, "</h2>")
		require.Contains(t, heading, "height=\"28\" align=\"top\"")
		require.Contains(t, heading, "?style=for-the-badge")
		require.NotContains(t, body, "badge/Quality_Gate-")
		require.Contains(t, body, "<sub>Current commit: <code>"+pr.Head.SHA+"</code>")
		require.NotContains(t, body, "Additional information")
		require.NotContains(t, body, "Performance measurements pending")
		require.NotContains(t, body, "View job summary")
		if !passed {
			require.Contains(t, body, "— A specific blocker.")
		}
	}
}

func TestQualityFileLimitGuidanceStatesRequiredReduction(t *testing.T) {
	pr := githubtest.PR(githubtest.Run())
	count := 157
	pr.ChangedFiles = &count
	gate, err := qualityFileCount(&pr)
	require.NoError(t, err)
	require.Contains(t, gate.Problem, "157 files")
	require.Contains(t, gate.NextAction, "at least 8 changed files")
}
