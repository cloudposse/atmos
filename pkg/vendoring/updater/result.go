// Package updater contains presentation-independent Component Updater results.
package updater

import (
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/vendoring"
)

type Result struct {
	Scope       string                         `json:"scope"`
	Check       bool                           `json:"check"`
	Updated     int                            `json:"updated"`
	Updates     []vendoring.SourceUpdateResult `json:"updates"`
	Branch      string                         `json:"branch,omitempty"`
	Commit      string                         `json:"commit,omitempty"`
	PullRequest *PullRequest                   `json:"pull_request,omitempty"`
	Status      string                         `json:"status"`
	Failure     string                         `json:"failure,omitempty"`
}

type PullRequest struct {
	Number int    `json:"number"`
	URL    string `json:"url"`
}

// MarkdownSummary is deliberately pure so all CI writers and tests use the
// same safe output regardless of whether a PR was requested.
func MarkdownSummary(result *Result) string {
	defer perf.Track(nil, "updater.MarkdownSummary")()
	var b strings.Builder
	b.WriteString("## Atmos Component Updater\n\n")
	fmt.Fprintf(&b, "- **Scope:** `%s`\n", result.Scope)
	fmt.Fprintf(&b, "- **Status:** %s\n", result.Status)
	fmt.Fprintf(&b, "- **Updates:** %d\n", result.Updated)
	if result.Check {
		b.WriteString("- **Mode:** dry run\n")
	}
	if result.Branch != "" {
		fmt.Fprintf(&b, "- **Branch:** `%s`\n", result.Branch)
	}
	if result.Commit != "" {
		fmt.Fprintf(&b, "- **Commit:** `%s`\n", result.Commit)
	}
	if result.PullRequest != nil && result.PullRequest.URL != "" {
		fmt.Fprintf(&b, "- **Pull request:** [#%d](%s)\n", result.PullRequest.Number, result.PullRequest.URL)
	}
	if result.Failure != "" {
		fmt.Fprintf(&b, "\n> %s\n", result.Failure)
	}
	if len(result.Updates) > 0 {
		b.WriteString("\n| Component | Current | Latest | Status |\n| --- | --- | --- | --- |\n")
		for _, update := range result.Updates {
			fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", update.Component, update.CurrentVersion, update.LatestVersion, update.Status)
		}
	}
	return b.String()
}
