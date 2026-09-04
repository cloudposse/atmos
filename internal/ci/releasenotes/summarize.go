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
	GHToken string
	// OpenAIAPIKey is optional: without one, entries fall back to
	// CodeRabbit's summary or a truncated description (FallbackSummary).
	OpenAIAPIKey string
	Model        string
	Release      ReleaseRef

	// DryRun stops short of updating the release: the rewritten body is
	// written to Output instead, so the result can be previewed (or the
	// OpenAI wiring tested) without touching the draft. Output defaults to
	// io.Discard when nil.
	DryRun bool
	Output io.Writer
}

// Result describes what SummarizeRelease produced.
type Result struct {
	// Entries is the number of pull requests in the release.
	Entries int
	// Chars is the length of the rewritten body.
	Chars int
	// AI is true when the summaries came from the model rather than the
	// fallback.
	AI bool
	// Degraded is true when even the summarized body exceeded
	// maxReleaseBodyChars and the release was rewritten as bare bullets.
	Degraded bool
}

// SummarizeRelease rewrites p.Release with condensed pull request notes. It
// reads release-drafter's skeleton body (or a full org-template body) for
// the PR list and categories, fetches each pull request's description from
// the pull-request API - the drafted release cannot be the source, since a
// release body over 125,000 characters cannot even be written - condenses
// each one (with the model when p.OpenAIAPIKey is set, otherwise
// FallbackSummary), renders the org template's <details> shape, and updates
// the release. Any returned error means the release was left as it was;
// callers on a "never break the release path" budget should log and move
// on - see magefiles/ci_release_notes.go.
func SummarizeRelease(ctx context.Context, client HTTPClient, p *SummarizeParams) (Result, error) {
	defer perf.Track(nil, "releasenotes.SummarizeRelease")()

	body, err := GetReleaseBody(ctx, client, p.GHToken, p.Release)
	if err != nil {
		return Result{}, err
	}
	entries, err := ParseDraftedBody(body)
	if err != nil {
		return Result{}, fmt.Errorf("releasenotes: parse release %s: %w", p.Release.ID, err)
	}
	if err := fetchMissingBodies(ctx, client, p, entries); err != nil {
		return Result{}, err
	}

	res := Result{Entries: len(entries)}
	summaries, err := condense(ctx, client, p, entries)
	if err != nil {
		return Result{}, err
	}
	res.AI = p.OpenAIAPIKey != ""

	rendered, err := RenderBody(entries, summaries)
	if err != nil {
		return Result{}, err
	}
	if len(rendered) > maxReleaseBodyChars {
		// Bare bullets are the skeleton itself: always small, always valid.
		res.Degraded = true
		if rendered, err = RenderBody(entries, make([]string, len(entries))); err != nil {
			return Result{}, err
		}
	}
	res.Chars = len(rendered)

	if p.DryRun {
		out := p.Output
		if out == nil {
			out = io.Discard
		}
		_, err = io.WriteString(out, rendered)
		return res, err
	}
	return res, UpdateReleaseBody(ctx, client, p.GHToken, p.Release, rendered)
}

// fetchMissingBodies fills in Body for entries the skeleton left empty, from
// the pull-request API. Entries that already carry a body (an org-template
// draft) or have no PR number are left alone.
func fetchMissingBodies(ctx context.Context, client HTTPClient, p *SummarizeParams, entries []PREntry) error {
	for i := range entries {
		if entries[i].Body != "" || entries[i].Number == 0 {
			continue
		}
		body, err := GetPullRequestBody(ctx, client, p.GHToken, p.Release.Repo, entries[i].Number)
		if err != nil {
			return err
		}
		entries[i].Body = body
	}
	return nil
}

// condense produces one summary per entry: from the model when a key is
// configured (fed the cleaned description, without CodeRabbit's block),
// otherwise FallbackSummary.
func condense(ctx context.Context, client HTTPClient, p *SummarizeParams, entries []PREntry) ([]string, error) {
	if p.OpenAIAPIKey == "" {
		out := make([]string, len(entries))
		for i, e := range entries {
			out[i] = FallbackSummary(e.Body)
		}
		return out, nil
	}
	cleaned := make([]PREntry, len(entries))
	for i, e := range entries {
		cleaned[i] = e
		cleaned[i].Body = CleanBody(e.Body)
	}
	return Summarize(ctx, client, p.OpenAIAPIKey, p.Model, cleaned)
}
