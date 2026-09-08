package source

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/project/config"
)

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
// fetches the template at that same ref, so engine.UpdateStrategyRendered
// can re-render it as the 3-way merge base.
//
// Must be called before this run's own SaveProjectRecord overwrites that
// file -- the whole point is to capture "what generated what's currently on
// disk" before this run's new answers replace it.
//
// Returns ErrRenderedStrategyRequiresConfig if no project record exists:
// unlike UpdateStrategyTracked (which falls back to literal "HEAD" against
// the target's own git history when no metadata is pinned), there is no
// equivalent fallback here -- a pristine re-render needs a real ref and real
// answers to reconstruct, not an assumption.
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

	stub := templates.Configuration{
		Name:   record.Metadata.Name,
		Source: WithRef(record.Spec.Source, record.Spec.BaseRef),
	}
	cleanup, err := Hydrate(&stub, sourceOverride)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch the old ref %q for the rendered update-strategy base: %w", record.Spec.BaseRef, err)
	}

	return &RenderedBase{Config: &stub, Values: record.Spec.Values, Cleanup: cleanup}, nil
}
