package updater

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/vendoring"
)

func TestMarkdownSummaryIncludesPRAndDryRun(t *testing.T) {
	tests := []struct {
		name     string
		result   *Result
		contains []string
	}{
		{
			name: "dry run with pull request",
			result: &Result{
				Scope: "all", Check: true, Updated: 1, Status: "updated", Branch: "atmos/component-updater/all", Commit: "abc123",
				PullRequest: &PullRequest{Number: 42, URL: "https://github.com/acme/repo/pull/42"},
				Updates:     []vendoring.SourceUpdateResult{{Component: "terraform/vpc", CurrentVersion: "1.0.0", LatestVersion: "1.1.0", Status: vendoring.StatusUpdated}},
			},
			contains: []string{"dry run", "[#42](https://github.com/acme/repo/pull/42)", "terraform/vpc"},
		},
		{name: "nil pull request", result: &Result{Scope: "all", Status: "no_updates"}, contains: []string{"Status:** no_updates"}},
		{name: "failure", result: &Result{Scope: "all", Status: "failed", Failure: "broken pipe"}, contains: []string{"Status:** failed", "> broken pipe"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := MarkdownSummary(tt.result)
			for _, want := range tt.contains {
				assert.Contains(t, summary, want)
			}
			assert.False(t, strings.Contains(summary, "<script"))
		})
	}
}
