package releasenotes

import (
	"context"
	"fmt"
	"io"

	"github.com/cloudposse/atmos/pkg/perf"
)

// SummarizeParams bundles SummarizeRelease's inputs so the function itself
// stays within revive's argument-count limit.
type SummarizeParams struct {
	GHToken      string
	OpenAIAPIKey string
	Model        string
	Release      ReleaseRef

	// DryRun stops short of updating the release: the summarized body is
	// written to Output instead, so the result can be previewed (or the
	// OpenAI wiring tested) without touching the draft. Output defaults to
	// io.Discard when nil.
	DryRun bool
	Output io.Writer
}

// SummarizeRelease rewrites p.Release with an AI-condensed body: it fetches
// the current (release-drafter-generated) body, parses its pull request
// entries, asks p.Model to summarize each one, then updates the release with
// the shorter result. Every step is one network call, so callers on a strict
// "never break the release path" budget should treat any returned error as
// skip-and-log rather than fatal - see magefiles/ci_release_notes.go, which
// does exactly that.
func SummarizeRelease(ctx context.Context, client HTTPClient, p *SummarizeParams) error {
	defer perf.Track(nil, "releasenotes.SummarizeRelease")()

	body, err := GetReleaseBody(ctx, client, p.GHToken, p.Release)
	if err != nil {
		return err
	}

	entries, err := ParseDraftedBody(body)
	if err != nil {
		return fmt.Errorf("releasenotes: parse release %s: %w", p.Release.ID, err)
	}

	summaries, err := Summarize(ctx, client, p.OpenAIAPIKey, p.Model, entries)
	if err != nil {
		return err
	}

	rendered, err := RenderBody(entries, summaries)
	if err != nil {
		return err
	}

	if p.DryRun {
		out := p.Output
		if out == nil {
			out = io.Discard
		}
		_, err = io.WriteString(out, rendered)
		return err
	}
	return UpdateReleaseBody(ctx, client, p.GHToken, p.Release, rendered)
}
