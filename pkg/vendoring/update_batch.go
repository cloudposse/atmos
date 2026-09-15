package vendoring

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/broker"
	"github.com/cloudposse/atmos/pkg/filelock"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/batch"
	"github.com/cloudposse/atmos/pkg/vendoring/concurrency"
	"github.com/cloudposse/atmos/pkg/vendoring/ordered"
	"github.com/cloudposse/atmos/pkg/vendoring/version"
)

// UpdateSourcesContext checks a resolved selection in parallel with ordered manifest edits.
// Progress callbacks are serialized and the returned report follows source order.
func UpdateSourcesContext(ctx context.Context, config *schema.AtmosConfiguration, sources []*ResolvedSource, params *UpdateParams) (*UpdateReport, error) {
	defer perf.Track(config, "vendoring.UpdateSourcesContext")()
	n, err := concurrency.Effective(config, params.MaxConcurrency)
	if err != nil {
		return nil, err
	}
	selected := selectUpdateSources(sources, params)
	if config != nil {
		broker.EnsureCredentials(ctx, config)
	}
	lister := params.Lister
	if lister == nil {
		lister = version.DefaultLister
	}
	// Retain the legacy callback's stable, one-based source enumeration. Rich
	// event consumers receive actual worker transitions below.
	emitLegacyUpdateProgress(params, selected)
	labels := updateProgressLabels(selected)
	var mu sync.Mutex
	emit := func(e batch.Event) {
		mu.Lock()
		defer mu.Unlock()
		e.Count = len(selected)
		if e.Label != "" {
			e.Label = labels[e.ID]
		}
		if params.OnEvent != nil {
			params.OnEvent(e)
		}
	}
	emit(batch.Event{Reset: true, Phase: "Checking"})
	checks := *params
	checks.DryRun = true
	results := &updateBatchResults{ctx: ctx, selected: selected, params: params, report: &UpdateReport{}, emit: emit}
	err = ordered.Run(ctx, ordered.Options[*SourceUpdateResult]{
		Count: len(selected), Concurrency: n,
		Prepare: func(ctx context.Context, i int) (*SourceUpdateResult, error) {
			src := selected[i]
			emit(batch.Event{ID: i, Label: src.Source.Component, Phase: "Checking"})
			return checkAndUpdateSourceContext(ctx, &sourceCheck{file: src.File, src: src.Source, componentType: src.ComponentType, params: &checks, lister: lister})
		},
		Commit: results.commit,
	})
	return results.finish(err)
}

// updateBatchResults owns ordered materialization and report aggregation.
type updateBatchResults struct {
	ctx      context.Context
	selected []*ResolvedSource
	params   *UpdateParams
	report   *UpdateReport
	failures []error
	emit     batch.Observer
}

func (b *updateBatchResults) commit(i int, res *SourceUpdateResult, err error) error {
	src := b.selected[i]
	if err == nil && res != nil && res.Status == StatusUpdated && !b.params.DryRun {
		b.emit(batch.Event{ID: i, Label: src.Source.Component, Phase: "Waiting for manifest lock"})
		err = applyUpdateProposal(b.ctx, src, res.LatestVersion, b.params.VersionSetter)
	}
	if err != nil {
		err = vendorUpdateError(src.Source.Component, err)
		b.failures = append(b.failures, err)
		if res != nil {
			res.Status = StatusFailed
			res.Reason = err.Error()
		}
	}
	if res != nil {
		b.report.Results = append(b.report.Results, *res)
		b.emit(batch.Event{ID: i, Label: src.Source.Component, Done: true, Outcome: string(res.Status), Err: err})
	}
	return nil
}

func (b *updateBatchResults) finish(err error) (*UpdateReport, error) {
	if err != nil {
		b.failures = append(b.failures, err)
		for i := len(b.report.Results); i < len(b.selected); i++ {
			src := b.selected[i]
			b.report.Results = append(b.report.Results, SourceUpdateResult{File: src.File, Component: src.Source.Component, CurrentVersion: src.Source.Version, ComponentType: src.ComponentType, Status: StatusFailed, Reason: err.Error()})
			b.emit(batch.Event{ID: i, Label: src.Source.Component, Done: true, Outcome: "canceled"})
		}
	}
	return b.report, errors.Join(b.failures...)
}

func applyUpdateProposal(ctx context.Context, src *ResolvedSource, latest string, setter func(string, string, string) error) error {
	file, err := filepath.Abs(src.File)
	if err != nil {
		return err
	}
	if canonical, err := filepath.EvalSymlinks(file); err == nil {
		file = canonical
	}
	return filelock.New(file+".lock").WithExclusive(ctx, func() error {
		if err := validateUpdateDeclaration(src); err != nil {
			return err
		}
		if setter != nil {
			return setter(src.File, src.Source.Component, latest)
		}
		if src.FromComponentManifest {
			return setComponentManifestVersionUnlocked(src.File, latest)
		}
		return setComponentVersionUnlocked(src.File, src.Source.Component, latest)
	})
}

// validateUpdateDeclaration tolerates unrelated edits but never overwrites a changed source.
func validateUpdateDeclaration(src *ResolvedSource) error {
	unchanged, err := updateDeclarationMatches(src)
	if err != nil {
		return err
	}
	if unchanged {
		return nil
	}
	return fmt.Errorf("%w: source %q changed during discovery; rerun vendor update", errUtils.ErrVendorUpdateFailed, src.Source.Component)
}

// updateDeclarationMatches reads only the physical file whose lock is held.
func updateDeclarationMatches(src *ResolvedSource) (bool, error) {
	if src.FromComponentManifest {
		current, err := ReadComponentManifest(src.File)
		if err != nil {
			return false, err
		}
		// Compare the fields used for version discovery. The component name is
		// derived from its directory rather than stored in component.yaml.
		return current.Spec.Source.Version == src.Source.Version && current.Spec.Source.Uri == src.Source.Source && reflect.DeepEqual(current.Spec.Source.Constraints, src.Source.Constraints), nil
	}
	sources, err := readVendorSources(src.File)
	if err != nil {
		return false, err
	}
	for i := range sources {
		current := &sources[i]
		if current.Component == src.Source.Component {
			current.File = src.Source.File
			return reflect.DeepEqual(current, src.Source), nil
		}
	}
	return false, nil
}

func selectUpdateSources(sources []*ResolvedSource, params *UpdateParams) []*ResolvedSource {
	selected := make([]*ResolvedSource, 0, len(sources))
	for _, src := range sources {
		if sourceMatchesFilter(src.Source, params.Component, params.Tags, params.Type) {
			selected = append(selected, src)
		}
	}
	return selected
}

func emitLegacyUpdateProgress(params *UpdateParams, selected []*ResolvedSource) {
	if params.OnProgress == nil {
		return
	}
	for i, src := range selected {
		params.OnProgress(src.Source.Component, i+1, len(selected))
	}
}

// withVersionFileLock protects standalone version setters as well as batch updates.
func withVersionFileLock(file string, fn func() error) error {
	abs, err := filepath.Abs(file)
	if err != nil {
		return err
	}
	if canonical, err := filepath.EvalSymlinks(abs); err == nil {
		abs = canonical
	}
	return filelock.New(abs+".lock").WithExclusive(context.Background(), fn)
}

// updateProgressLabels distinguishes equal component names without changing report identities.
func updateProgressLabels(sources []*ResolvedSource) []string {
	counts := map[string]int{}
	for _, src := range sources {
		counts[src.Source.Component]++
	}
	labels := make([]string, len(sources))
	for i, src := range sources {
		labels[i] = src.Source.Component
		if counts[labels[i]] > 1 {
			labels[i] = fmt.Sprintf("%s (%s/%s, job %d)", labels[i], src.ComponentType, filepath.Base(src.File), i+1)
		}
	}
	return labels
}
