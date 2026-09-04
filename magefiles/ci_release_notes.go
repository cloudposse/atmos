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
// GET, one OpenAI request covering every PR at once, one release PATCH.
const releaseNotesHTTPTimeout = 2 * time.Minute

// SummarizeNotes condenses a drafted GitHub release's body with an LLM so it
// stays well under GitHub's 125,000-character limit on repos that merge many
// pull requests between releases (see docs/fixes). Thin: reads the
// environment and builds the real HTTP client, then delegates to
// summarizeNotes, which holds the actual (tested) skip/success/warning
// logic.
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

// summarizeNotes never returns an error: it's opt-in (with no
// params.OpenAIAPIKey it logs why and does nothing, so a workflow step gated
// only by this target's presence degrades cleanly on a fork or a repo that
// hasn't configured the secret), and any failure past that point (OpenAI
// down, malformed response, GitHub API error) is caught and logged, not
// propagated - this must never be able to fail the release job over a
// summarization hiccup, since release-drafter's own (unsummarized) body is
// still perfectly usable.
func summarizeNotes(ctx context.Context, stderr io.Writer, client releasenotes.HTTPClient, params *releasenotes.SummarizeParams) {
	if params.OpenAIAPIKey == "" {
		fmt.Fprintln(stderr, "mage: OPENAI_API_KEY not set, skipping release notes summarization")
		return
	}
	if params.GHToken == "" {
		fmt.Fprintln(stderr, "mage: GITHUB_TOKEN not set, skipping release notes summarization")
		return
	}

	if err := releasenotes.SummarizeRelease(ctx, client, params); err != nil {
		fmt.Fprintf(stderr, "::warning::release notes summarization skipped: %s\n", err)
		return
	}
	if params.DryRun {
		fmt.Fprintln(stderr, "mage: release notes summarized (dry run: release not updated)")
		return
	}
	fmt.Fprintln(stderr, "mage: release notes summarized")
}
