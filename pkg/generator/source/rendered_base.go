package source

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/project/config"
)

// ValidateRenderedSource rejects a --update-strategy=rendered generation up
// front when src/resolvedRef can never produce a project record a later
// rendered update can actually reconstruct from -- rather than silently
// generating now and only failing at that later update, once the answers
// that produced this generation are no longer easily reproducible.
//
// A record is reconstructible in exactly two cases:
//   - src is a pinnable source (see IsPinnableSource: git:: or oci://) and
//     resolvedRef is non-empty -- the normal case, an immutable commit SHA
//     or manifest digest recorded as spec.renderedRef.
//   - src is a non-pinnable source Hydrate can still re-fetch verbatim by
//     src itself (a local path, file://, or s3::/plain http(s) archive) --
//     UnpinnedRenderedRefMarker documents that case; it is not immutable,
//     but a later Hydrate(src) call still succeeds.
//
// It is NOT reconstructible for:
//   - config.SourceEmbedded ("embedded"): a template bundled into the Atmos
//     binary has no fetchable location Hydrate can re-resolve by that literal
//     string, so a later rendered update would always fail regardless of
//     what gets recorded now.
//   - a pinnable source (git/oci) whose ref failed to resolve: recording
//     UnpinnedRenderedRefMarker there would corrupt replaceRef's git ref=
//     query parameter with the literal marker string on the next fetch
//     attempt (see UnpinnedRenderedRefMarker's own doc comment for why the
//     marker is safe only for non-git/non-oci sources), so this is
//     deliberately not treated the same as the local/s3/http case above.
func ValidateRenderedSource(src, resolvedRef string) error {
	defer perf.Track(nil, "source.ValidateRenderedSource")()

	if IsPinnableSource(src) {
		if resolvedRef != "" {
			return nil
		}
		return errUtils.Build(errUtils.ErrRenderedStrategyUnsupportedSource).
			WithExplanationf("Failed to resolve an immutable commit or digest for `%s`", src).
			WithHint("`--update-strategy=rendered` needs a resolved ref to pin a future update's merge base to").
			WithHint("Re-run with `--update-strategy=tracked` instead").
			WithContext("source", src).
			WithExitCode(2).
			Err()
	}
	if src == config.SourceEmbedded {
		return errUtils.Build(errUtils.ErrRenderedStrategyUnsupportedSource).
			WithExplanationf("`--update-strategy=rendered` is not supported for the embedded `%s` template", src).
			WithHint("Embedded templates have no location Atmos can re-fetch for a future update's merge base").
			WithHint("Re-run with `--update-strategy=tracked` instead").
			WithContext("source", src).
			WithExitCode(2).
			Err()
	}
	return nil
}

// RenderedBase bundles ResolveRenderedBase's results (grouped into a struct,
// rather than four separate return values, to stay under revive's
// function-result-limit).
type RenderedBase struct {
	// Config is the old ref's fully-hydrated template configuration.
	Config *templates.Configuration
	// Values is that generation's own recorded answers.
	Values map[string]interface{}
	// Cleanup releases the temporary fetch directory Config's Files were
	// hydrated into. Always non-nil on success; must be called once the
	// merge that consumes Config is done.
	Cleanup func()
}

// ResolveRenderedBase loads targetDir's own recorded project state
// (.atmos/scaffold.yaml, written by the last successful generation) and
// fetches the template pinned at the resolved ref recorded there
// (spec.renderedRef -- a commit SHA for git sources, a manifest digest for
// OCI sources, see pinRenderedRef), so engine.UpdateStrategyRendered can
// re-render it as the 3-way merge base.
//
// Must be called before this run's own SaveProjectRecord overwrites that
// file -- the whole point is to capture "what generated what's currently on
// disk" before this run's new answers replace it.
//
// Returns ErrRenderedStrategyRequiresConfig if no project record exists, or
// one exists but was never generated under rendered at all (neither
// spec.renderedRef nor spec.baseRef is set). Unlike UpdateStrategyTracked
// (which falls back to literal "HEAD" against the target's own git history
// when no metadata is pinned), there is no equivalent fallback here -- a
// pristine re-render needs a real commit and real answers to reconstruct,
// not an assumption.
//
// Returns ErrUpdateStrategySwitchedToRendered if the record shows
// spec.baseRef set and spec.renderedRef empty: the project was last managed
// with tracked, which never records a resolved commit SHA for the template
// source, so there is nothing here to re-render from.
func ResolveRenderedBase(targetDir, sourceOverride string) (*RenderedBase, error) {
	defer perf.Track(nil, "source.ResolveRenderedBase")()

	record, err := config.LoadProjectRecord(targetDir)
	if err != nil {
		return nil, fmt.Errorf("failed to load the existing project record: %w", err)
	}
	if record == nil {
		return nil, errUtils.Build(errUtils.ErrRenderedStrategyRequiresConfig).
			WithExplanationf("No recorded scaffold configuration found at `%s`", targetDir).
			WithHint("`--update-strategy=rendered` needs a previous generation's recorded answers to re-render as the merge base").
			WithHint("Use `--update-strategy=tracked` (the default) instead").
			WithContext("target_dir", targetDir).
			WithExitCode(2).
			Err()
	}
	if record.Spec.RenderedRef == "" {
		if record.Spec.BaseRef != "" {
			return nil, errUtils.Build(errUtils.ErrUpdateStrategySwitchedToRendered).
				WithExplanationf("`%s` was last updated with `--update-strategy=tracked`", targetDir).
				WithHint("Tracked mode never records a resolved commit for the template source, so there is no rendered base to reconstruct").
				WithHint("Re-run with `--update-strategy=tracked`, or `--force` to fully regenerate and start fresh with `rendered`").
				WithContext("target_dir", targetDir).
				WithExitCode(2).
				Err()
		}
		return nil, errUtils.Build(errUtils.ErrRenderedStrategyRequiresConfig).
			WithExplanationf("`%s` has no recorded rendered-strategy history", targetDir).
			WithHint("`--update-strategy=rendered` needs a previous generation's recorded answers to re-render as the merge base").
			WithHint("Use `--update-strategy=tracked` (the default) instead").
			WithContext("target_dir", targetDir).
			WithExitCode(2).
			Err()
	}

	pinnedSource, err := pinRenderedRef(record.Spec.Source, record.Spec.RenderedRef)
	if err != nil {
		return nil, fmt.Errorf("failed to pin the recorded source to %q for the rendered update-strategy base: %w", record.Spec.RenderedRef, err)
	}
	stub := templates.Configuration{
		Name:   record.Metadata.Name,
		Source: pinnedSource,
	}
	cleanup, err := Hydrate(&stub, sourceOverride)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch the recorded commit %q for the rendered update-strategy base: %w", record.Spec.RenderedRef, err)
	}

	return &RenderedBase{Config: &stub, Values: record.Spec.Values, Cleanup: cleanup}, nil
}
