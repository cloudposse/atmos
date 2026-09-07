//go:build mage

package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/magefile/mage/mg"

	"github.com/cloudposse/atmos/internal/ci/releasenotes"
)

// Release groups CI targets for the drafted-release-notes pipeline.
type Release mg.Namespace

// defaultSummaryModel is used when OPENAI_MODEL is unset; the lowest 5.6
// tier is cheap and fast enough for a one-sentence-per-PR summarization
// pass. A 403 model_not_found here means the OpenAI project has not been
// granted this model - a project setting, not a code problem - and the
// grant takes a while to propagate (observed flapping between 403 and
// success for several minutes after it was made).
const defaultSummaryModel = "gpt-5.6-luna"

// releaseNotesHTTPTimeout bounds the whole SummarizeNotes call: one release
// GET, one pull-request GET per entry, one OpenAI request covering every PR
// at once, one release PATCH.
const releaseNotesHTTPTimeout = 5 * time.Minute

// SummarizeNotes rewrites a drafted GitHub release's skeleton body (see
// .github/auto-release.yml) as collapsible per-PR notes with condensed
// descriptions, so the release stays well under GitHub's 125,000-character
// limit on a repo that merges many pull requests between releases (see
// docs/fixes). With OPENAI_API_KEY the descriptions are summarized by the
// model; without it, CodeRabbit's summary or a truncated description stands
// in. Thin: reads the environment and builds the real HTTP client, then
// delegates to summarizeNotes, which holds the actual (tested)
// skip/success/warning logic.
//
// Set RELEASE_NOTES_DRY_RUN=1 to print the summarized body to stdout instead
// of updating the release - the way to preview the result, or to verify the
// OpenAI key and model work, against a real draft without modifying it.
func (Release) SummarizeNotes(repo, releaseID string) error {
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = defaultSummaryModel
	}

	ctx, cancel := context.WithTimeout(context.Background(), releaseNotesHTTPTimeout)
	defer cancel()

	client := &http.Client{Timeout: releaseNotesHTTPTimeout}
	params := &releasenotes.SummarizeParams{
		GHToken:      os.Getenv("GITHUB_TOKEN"),
		OpenAIAPIKey: os.Getenv("OPENAI_API_KEY"),
		Model:        model,
		Release:      releasenotes.ReleaseRef{Repo: repo, ID: releaseID},
		DryRun:       os.Getenv("RELEASE_NOTES_DRY_RUN") != "",
		Output:       os.Stdout,
	}
	summarizeNotes(ctx, os.Stderr, client, params)
	return nil
}

// summarizeNotes never returns an error: any failure (OpenAI down,
// malformed response, GitHub API error) is caught and logged, not
// propagated - this must never be able to fail the release job over a
// summarization hiccup, since release-drafter's skeleton body is still
// usable release notes on its own. No OPENAI_API_KEY is not a failure: the
// release is still rewritten, with fallback summaries.
func summarizeNotes(ctx context.Context, stderr io.Writer, client releasenotes.HTTPClient, params *releasenotes.SummarizeParams) {
	if params.GHToken == "" {
		fmt.Fprintln(stderr, "mage: GITHUB_TOKEN not set, skipping release notes summarization")
		return
	}
	if params.OpenAIAPIKey == "" {
		fmt.Fprintln(stderr, "mage: OPENAI_API_KEY not set, using CodeRabbit summaries or truncated descriptions instead of the model")
	}

	res, err := releasenotes.SummarizeRelease(ctx, client, params)
	if err != nil {
		fmt.Fprintf(stderr, "::warning::release notes summarization skipped: %s\n", err)
		return
	}
	mode := "fallback summaries"
	if res.AI {
		mode = "model summaries"
	}
	if res.Degraded {
		fmt.Fprintf(stderr, "::warning::release notes rewritten as bare bullets: even the summarized body exceeded the size limit (%d entries)\n", res.Entries)
	}
	if params.DryRun {
		fmt.Fprintf(stderr, "mage: release notes rewritten (%d entries, %d chars, %s; dry run: release not updated)\n", res.Entries, res.Chars, mode)
		return
	}
	fmt.Fprintf(stderr, "mage: release notes rewritten (%d entries, %d chars, %s)\n", res.Entries, res.Chars, mode)
}
