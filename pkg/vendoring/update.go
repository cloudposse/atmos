package vendoring

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/version"
)

// listTagsTimeout bounds a single remote tag listing.
const listTagsTimeout = 30 * time.Second

// UpdateStatus enumerates the outcome for a single source in an update run.
type UpdateStatus string

const (
	// StatusUpdated means the source's version was (or would be) changed.
	StatusUpdated UpdateStatus = "updated"
	// StatusUpToDate means the source already pins the latest allowed version.
	StatusUpToDate UpdateStatus = "up-to-date"
	// StatusSkipped means the source was not checked (templated, non-Git, etc.).
	StatusSkipped UpdateStatus = "skipped"
	// StatusFailed means the source could not be checked or updated because of a hard error.
	StatusFailed UpdateStatus = "failed"
)

// SourceUpdateResult is the per-source outcome of an update run.
type SourceUpdateResult struct {
	File           string
	Component      string
	CurrentVersion string
	LatestVersion  string
	Status         UpdateStatus
	Reason         string // Populated for StatusSkipped and StatusFailed.
	// Archived reports whether the source's upstream Git repository is archived (GitHub sources
	// only; best-effort). This is orthogonal to Status: a component can be both StatusUpToDate
	// and Archived at the same time, so it's a separate field rather than another UpdateStatus
	// value.
	Archived bool
}

// UpdateReport summarizes an update run.
type UpdateReport struct {
	Results []SourceUpdateResult
}

// UpdatedCount returns how many sources were (or would be) updated.
func (r *UpdateReport) UpdatedCount() int {
	defer perf.Track(nil, "vendoring.UpdateReport.UpdatedCount")()

	n := 0
	for _, res := range r.Results {
		if res.Status == StatusUpdated {
			n++
		}
	}
	return n
}

// UpdateParams configures an update run.
type UpdateParams struct {
	// VendorFiles are the physical manifest files to process (a vendor.yaml plus
	// any imported files). Edits are applied to the file that declares each source.
	VendorFiles []string
	// ExtraSources are additional already-resolved sources to check/update alongside
	// VendorFiles — used by the opt-in --component-manifests sweep to fold in
	// component.yaml-only components. A component name already present in VendorFiles'
	// sources takes precedence (vendor.yaml wins) and its ExtraSources entry, if any, is
	// skipped.
	ExtraSources []*ResolvedSource
	// Component, when set, restricts updates to that component.
	Component string
	// Tags, when set, restricts updates to sources carrying any of these tags.
	Tags []string
	// Type, when set, restricts updates to sources targeting components of this type.
	Type string
	// DryRun reports what would change without writing files.
	DryRun bool
	// Lister lists remote tags; defaults to version.DefaultLister when nil.
	Lister version.RemoteLister
	// VersionSetter, when set, replaces SetComponentVersion for persisting a resolved
	// version. Component-manifest (component.yaml) sources need spec.source.version
	// instead of vendor.yaml's spec.sources[i].version. Defaults to SetComponentVersion.
	VersionSetter func(file, component, version string) error
	// OnProgress, when set, is called immediately before each source that passes the
	// component/tag/type filters is checked against the remote, reporting the source's
	// component name along with its 1-based position and the total number of sources
	// that will be checked. Used to drive a live "Checking <component>..." progress
	// indicator during what is otherwise a sequence of blocking network calls.
	OnProgress func(component string, index, total int)
	// ArchivedChecker checks whether a source's upstream Git repository is archived; defaults
	// to DefaultArchivedChecker when nil. The check is best-effort and non-fatal: an error here
	// never fails the overall update/check for a source, it only leaves Archived false.
	ArchivedChecker ArchivedChecker
}

// Update checks each Git-backed source for a newer allowed version and updates
// the version field in place (preserving comments, anchors, and templates),
// unless DryRun is set. Non-Git and templated-version sources are skipped.
func Update(atmosConfig *schema.AtmosConfiguration, params *UpdateParams) (*UpdateReport, error) {
	defer perf.Track(atmosConfig, "vendoring.Update")()

	lister := params.Lister
	if lister == nil {
		lister = version.DefaultLister
	}

	fileSources, err := loadVendorFileSources(params.VendorFiles)
	if err != nil {
		return nil, err
	}
	total := countMatchingSources(fileSources, params.ExtraSources, params)

	walk := &updateWalk{params: params, lister: lister, seen: map[string]bool{}, total: total}

	fileResults, failures := processVendorFileSources(fileSources, walk)
	extraResults, extraFailures := processExtraUpdateSources(params.ExtraSources, walk)
	failures = append(failures, extraFailures...)

	report := &UpdateReport{Results: append(fileResults, extraResults...)}
	return report, errors.Join(failures...)
}

// updateWalk bundles the state shared across a single Update run's two source-checking passes
// (an Options-pattern struct, since the helpers below need enough shared context that a positional
// argument list would exceed a readable length).
type updateWalk struct {
	params *UpdateParams
	lister version.RemoteLister
	seen   map[string]bool // Components already checked, so ExtraSources can detect vendor.yaml precedence.
	index  int             // Running 1-based progress counter, shared across both passes.
	total  int             // Total sources that will be checked, for OnProgress.
}

// processVendorFileSources checks every source declared across fileSources, marking each
// component seen (so ExtraSources can detect vendor.yaml precedence) and reporting progress before
// each check.
func processVendorFileSources(fileSources []vendorFileSources, walk *updateWalk) ([]SourceUpdateResult, []error) {
	var results []SourceUpdateResult
	var failures []error
	for _, fs := range fileSources {
		for i := range fs.sources {
			walk.seen[fs.sources[i].Component] = true
			reportProgress(walk.params, &fs.sources[i], &walk.index, walk.total)
			res, err := checkAndUpdateSource(fs.file, &fs.sources[i], walk.params, walk.lister)
			if res != nil {
				results = append(results, *res)
			}
			if err != nil {
				failures = append(failures, err)
			}
		}
	}
	return results, failures
}

// processExtraUpdateSources checks every ExtraSources entry not already seen (a vendor.yaml
// source of the same component name always wins and skips its ExtraSources duplicate).
func processExtraUpdateSources(extra []*ResolvedSource, walk *updateWalk) ([]SourceUpdateResult, []error) {
	var results []SourceUpdateResult
	var failures []error
	for _, ex := range extra {
		if walk.seen[ex.Source.Component] {
			continue // vendor.yaml already declares this component; it wins.
		}
		reportProgress(walk.params, ex.Source, &walk.index, walk.total)
		res, err := updateResolvedSource(ex, walk.params, walk.lister)
		if res != nil {
			results = append(results, *res)
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return results, failures
}

// vendorFileSources pairs a manifest file with the sources it declares, loaded up front so
// Update can compute a post-filter total for progress reporting without re-parsing files.
type vendorFileSources struct {
	file    string
	sources []schema.AtmosVendorSource
}

// loadVendorFileSources reads every VendorFiles entry once, up front.
func loadVendorFileSources(files []string) ([]vendorFileSources, error) {
	loaded := make([]vendorFileSources, 0, len(files))
	for _, file := range files {
		sources, err := readVendorSources(file)
		if err != nil {
			return nil, err
		}
		loaded = append(loaded, vendorFileSources{file: file, sources: sources})
	}
	return loaded, nil
}

// countMatchingSources counts how many sources (across fileSources and extra, applying the same
// vendor.yaml-wins dedup Update itself applies) will actually be checked once the
// component/tags/type filters are applied. Used to give OnProgress a stable total.
func countMatchingSources(fileSources []vendorFileSources, extra []*ResolvedSource, params *UpdateParams) int {
	seen := map[string]bool{}
	total := 0
	for _, fs := range fileSources {
		for i := range fs.sources {
			seen[fs.sources[i].Component] = true
			if sourceMatchesFilter(&fs.sources[i], params.Component, params.Tags, params.Type) {
				total++
			}
		}
	}
	for _, ex := range extra {
		if seen[ex.Source.Component] {
			continue
		}
		if sourceMatchesFilter(ex.Source, params.Component, params.Tags, params.Type) {
			total++
		}
	}
	return total
}

// reportProgress invokes params.OnProgress (when set) for a source that passes the active
// filters, advancing *index. Sources filtered out are not reported and do not advance the index,
// matching checkAndUpdateSource's own filtering so the numbers stay in sync.
func reportProgress(params *UpdateParams, src *schema.AtmosVendorSource, index *int, total int) {
	if params.OnProgress == nil {
		return
	}
	if !sourceMatchesFilter(src, params.Component, params.Tags, params.Type) {
		return
	}
	*index++
	params.OnProgress(src.Component, *index, total)
}

// UpdateResolved evaluates and, unless DryRun, updates a single already-resolved component source
// (from ResolveComponentSource). Used for --component, which may resolve to either a vendor.yaml
// source or a component.yaml fallback; auto-selects the component.yaml VersionSetter when
// resolved.FromComponentManifest and the caller didn't already set one.
func UpdateResolved(resolved *ResolvedSource, params *UpdateParams) (*UpdateReport, error) {
	defer perf.Track(nil, "vendoring.UpdateResolved")()

	lister := params.Lister
	if lister == nil {
		lister = version.DefaultLister
	}

	if params.OnProgress != nil {
		params.OnProgress(resolved.Source.Component, 1, 1)
	}

	res, err := updateResolvedSource(resolved, params, lister)
	report := &UpdateReport{}
	if res != nil {
		report.Results = append(report.Results, *res)
	}
	return report, err
}

// updateResolvedSource applies a component-manifest-flavored VersionSetter default (unless the
// caller already set one) and delegates to checkAndUpdateSource.
func updateResolvedSource(resolved *ResolvedSource, params *UpdateParams, lister version.RemoteLister) (*SourceUpdateResult, error) {
	p := *params
	if resolved.FromComponentManifest && p.VersionSetter == nil {
		p.VersionSetter = func(file, _, ver string) error { return SetComponentManifestVersion(file, ver) }
	}
	return checkAndUpdateSource(resolved.File, resolved.Source, &p, lister)
}

// checkAndUpdateSource evaluates one source and, unless DryRun, applies the update.
// Returns nil when the source is filtered out.
func checkAndUpdateSource(file string, src *schema.AtmosVendorSource, params *UpdateParams, lister version.RemoteLister) (*SourceUpdateResult, error) {
	if !sourceMatchesFilter(src, params.Component, params.Tags, params.Type) {
		return nil, nil
	}

	res := &SourceUpdateResult{File: file, Component: src.Component, CurrentVersion: src.Version}

	if reason := skipReason(src); reason != "" {
		res.Status, res.Reason = StatusSkipped, reason
		return res, nil
	}

	// Archived-ness is orthogonal to the version-check outcome below (a component can be both
	// up to date and archived upstream), so it's computed once here and applies to every branch,
	// including the StatusFailed early return.
	res.Archived = checkArchived(src, params.ArchivedChecker)

	latest, err := resolveLatest(src, lister)
	if err != nil {
		res.Status, res.Reason = StatusFailed, err.Error()
		return res, vendorUpdateError(src.Component, err)
	}
	res.LatestVersion = latest

	if !isNewer(src.Version, latest) {
		res.Status = StatusUpToDate
		return res, nil
	}

	res.Status = StatusUpdated
	if !params.DryRun {
		setter := params.VersionSetter
		if setter == nil {
			setter = SetComponentVersion
		}
		if err := setter(file, src.Component, latest); err != nil {
			res.Status, res.Reason = StatusFailed, err.Error()
			return res, vendorUpdateError(src.Component, err)
		}
	}
	return res, nil
}

func vendorUpdateError(component string, err error) error {
	return errors.Join(errUtils.ErrVendorUpdateFailed, fmt.Errorf("component %q: %w", component, err))
}

// skipReason returns a non-empty reason when a source must not be version-checked.
func skipReason(src *schema.AtmosVendorSource) string {
	switch {
	case src.Version == "":
		return "no version pinned"
	case version.IsTemplatedVersion(src.Version):
		return "templated version"
	case !version.IsGitSource(src.Source):
		return "non-Git source not supported"
	default:
		return ""
	}
}

// resolveLatest lists remote tags and applies the source's constraints.
func resolveLatest(src *schema.AtmosVendorSource, lister version.RemoteLister) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), listTagsTimeout)
	defer cancel()

	gitURI := version.ExtractGitURI(src.Source)
	tags, err := lister.ListTags(ctx, gitURI)
	if err != nil {
		return "", err
	}
	return version.ResolveVersionConstraints(tags, src.Constraints)
}

// isNewer reports whether latest is a strictly higher semver than current. When
// either side is not semver, it falls back to a simple string inequality so
// commit-pinned or non-semver tags still surface as changes.
func isNewer(current, latest string) bool {
	cv, cErr := version.ParseSemVer(current)
	lv, lErr := version.ParseSemVer(latest)
	if cErr == nil && lErr == nil {
		return lv.GreaterThan(cv)
	}
	return latest != "" && latest != current
}

// sourceMatchesFilter applies the component, tags, and type filters.
func sourceMatchesFilter(src *schema.AtmosVendorSource, component string, tags []string, componentType string) bool {
	if component != "" && src.Component != component {
		return false
	}
	if componentType != "" && !sourceTargetsType(src, componentType) {
		return false
	}
	if len(tags) == 0 {
		return true
	}
	for _, want := range tags {
		for _, have := range src.Tags {
			if want == have {
				return true
			}
		}
	}
	return false
}

func sourceTargetsType(src *schema.AtmosVendorSource, componentType string) bool {
	for _, target := range src.Targets {
		if targetPathMatchesType(target.Path, componentType) {
			return true
		}
	}
	return false
}

func targetPathMatchesType(targetPath string, componentType string) bool {
	parts := strings.Split(path.Clean(strings.ReplaceAll(targetPath, "\\", "/")), "/")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "components" && parts[i+1] == componentType {
			return true
		}
	}
	return false
}
