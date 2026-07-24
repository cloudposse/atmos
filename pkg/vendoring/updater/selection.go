package updater

import (
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/vendoring"
)

// RunWithProgress wraps a blocking update operation with a caller-provided progress UI (a terminal
// spinner in production; a plain passthrough in tests/non-interactive runs). This package must not
// depend on terminal/TUI packages, so callers inject their own wrapper -- cmd/vendor passes an
// adapter around its own bubbletea spinner (runUpdateWithSpinner).
type RunWithProgress func(doWork func(onProgress func(component string, index, total int)) (*vendoring.UpdateReport, error)) (*vendoring.UpdateReport, error)

// SelectionParams bundles ResolveGroupSelection and UpdateSelectedComponents' shared inputs
// (Options Pattern, CLAUDE.md: crossed the >4-total-parameters threshold).
type SelectionParams struct {
	Viper           *viper.Viper
	ComponentType   string
	Tags            []string
	Group           string
	Check           bool
	VendorFile      string
	RunWithProgress RunWithProgress
	// RepoWideUpdate resolves the full repo-wide update set -- cmd/vendor.runRepoWideUpdate, bound
	// to its own viper/flag inputs -- so a --group selection's discovery pass can reuse the exact
	// same repo-wide update logic a bare update run uses.
	RepoWideUpdate func(check bool) (*vendoring.UpdateReport, error)
}

// ResolveGroupSelection discovers a --group invocation's outdated components via a check-mode
// repo-wide update (p.RepoWideUpdate), narrowed by the group's include/exclude patterns. When
// p.Check is set, or the resulting selection is empty, it returns a non-nil finalReport for the
// caller to return immediately; otherwise it returns the resolved component names for the caller
// to continue with.
func ResolveGroupSelection(p *SelectionParams) (finalReport *vendoring.UpdateReport, components []string, err error) {
	defer perf.Track(nil, "updater.ResolveGroupSelection")()

	discovery, err := p.RepoWideUpdate(true)
	if err != nil {
		return nil, nil, err
	}
	components = FilterGroupComponents(discovery, p.Viper.GetStringSlice("vendor.update.groups."+p.Group+".include"), p.Viper.GetStringSlice("vendor.update.groups."+p.Group+".exclude"))
	if p.Check {
		return FilterReport(discovery, components), nil, nil
	}
	if len(components) == 0 {
		return &vendoring.UpdateReport{}, nil, nil
	}
	return nil, components, nil
}

// UpdateSelectedComponents runs the update for each of components individually, resolving each
// component's declared source before updating it.
func UpdateSelectedComponents(p *SelectionParams, components []string) (*vendoring.UpdateReport, error) {
	defer perf.Track(nil, "updater.UpdateSelectedComponents")()

	results := make([]vendoring.SourceUpdateResult, 0, len(components))
	for _, component := range components {
		resolved, err := vendoring.ResolveComponentSource(&vendoring.ResolveSourceParams{VendorFile: p.VendorFile, Component: component, ComponentType: p.ComponentType})
		if err != nil {
			return nil, err
		}
		report, err := p.RunWithProgress(func(onProgress func(component string, index, total int)) (*vendoring.UpdateReport, error) {
			return vendoring.UpdateResolved(resolved, &vendoring.UpdateParams{Tags: p.Tags, DryRun: p.Check, OnProgress: onProgress})
		})
		if err != nil {
			return nil, err
		}
		results = append(results, report.Results...)
	}
	return &vendoring.UpdateReport{Results: results}, nil
}
