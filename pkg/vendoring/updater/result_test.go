package updater

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/vendoring"
)

func TestMarkdownSummaryIncludesPRAndDryRun(t *testing.T) {
	summary := MarkdownSummary(&Result{
		Scope: "all", Check: true, Updated: 1, Status: "updated", Branch: "atmos/component-updater/all", Commit: "abc123",
		PullRequest: &PullRequest{Number: 42, URL: "https://github.com/acme/repo/pull/42"},
		Updates:     []vendoring.SourceUpdateResult{{Component: "terraform/vpc", CurrentVersion: "1.0.0", LatestVersion: "1.1.0", Status: vendoring.StatusUpdated}},
	})
	assert.Contains(t, summary, "dry run")
	assert.Contains(t, summary, "[#42](https://github.com/acme/repo/pull/42)")
	assert.Contains(t, summary, "terraform/vpc")
	assert.False(t, strings.Contains(summary, "<script"))
}
