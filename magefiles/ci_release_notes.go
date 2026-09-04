//go:build mage

package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/magefile/mage/mg"

	"github.com/cloudposse/atmos/internal/ci/releasenotes"
)

// Release groups CI targets for the drafted-release-notes pipeline.
type Release mg.Namespace

// defaultSummaryModel is used when OPENAI_MODEL is unset; the lowest tier
// is cheap and fast enough for a one-sentence-per-PR summarization pass.
const defaultSummaryModel = "gpt-5.6-luna"

// releaseNotesHTTPTimeout bounds the whole SummarizeNotes call: one release
// GET, one OpenAI request covering every PR at once, one release PATCH.
const releaseNotesHTTPTimeout = 2 * time.Minute

// SummarizeNotes condenses a drafted GitHub release's body with an LLM so it
// stays well under GitHub's 125,000-character limit on repos that merge many
// pull requests between releases (see docs/fixes). It is opt-in: with no
// OPENAI_API_KEY in the environment it logs why and returns success, so a
// workflow step gated only by this target's presence degrades cleanly on a
// fork or a repo that hasn't configured the secret. Any failure past that
// point (OpenAI down, malformed response, GitHub API error) is caught,
// logged, and also treated as success - this must never be able to fail the
// release job over a summarization hiccup, since release-drafter's own
// (unsummarized) body is still perfectly usable.
func (Release) SummarizeNotes(repo, releaseID string) error {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "mage: OPENAI_API_KEY not set, skipping release notes summarization")
		return nil
	}
	ghToken := os.Getenv("GITHUB_TOKEN")
	if ghToken == "" {
		fmt.Fprintln(os.Stderr, "mage: GITHUB_TOKEN not set, skipping release notes summarization")
		return nil
	}
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = defaultSummaryModel
	}

	ctx, cancel := context.WithTimeout(context.Background(), releaseNotesHTTPTimeout)
	defer cancel()

	client := &http.Client{Timeout: releaseNotesHTTPTimeout}
	params := &releasenotes.SummarizeParams{
		GHToken:      ghToken,
		OpenAIAPIKey: apiKey,
		Model:        model,
		Release:      releasenotes.ReleaseRef{Repo: repo, ID: releaseID},
	}
	if err := releasenotes.SummarizeRelease(ctx, client, params); err != nil {
		fmt.Fprintf(os.Stderr, "::warning::release notes summarization skipped: %s\n", err)
		return nil
	}
	fmt.Fprintln(os.Stderr, "mage: release notes summarized")
	return nil
}
