package vendoring

import (
	"context"
	"time"

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
)

// SourceUpdateResult is the per-source outcome of an update run.
type SourceUpdateResult struct {
	File           string
	Component      string
	CurrentVersion string
	LatestVersion  string
	Status         UpdateStatus
	Reason         string // populated for StatusSkipped
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
	// Component, when set, restricts updates to that component.
	Component string
	// Tags, when set, restricts updates to sources carrying any of these tags.
	Tags []string
	// DryRun reports what would change without writing files.
	DryRun bool
	// Lister lists remote tags; defaults to version.DefaultLister when nil.
	Lister version.RemoteLister
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

	report := &UpdateReport{}
	for _, file := range params.VendorFiles {
		sources, err := readVendorSources(file)
		if err != nil {
			return nil, err
		}
		for i := range sources {
			res := checkAndUpdateSource(file, &sources[i], params, lister)
			if res != nil {
				report.Results = append(report.Results, *res)
			}
		}
	}
	return report, nil
}

// checkAndUpdateSource evaluates one source and, unless DryRun, applies the update.
// Returns nil when the source is filtered out.
func checkAndUpdateSource(file string, src *schema.AtmosVendorSource, params *UpdateParams, lister version.RemoteLister) *SourceUpdateResult {
	if !sourceMatchesFilter(src, params.Component, params.Tags) {
		return nil
	}

	res := &SourceUpdateResult{File: file, Component: src.Component, CurrentVersion: src.Version}

	if reason := skipReason(src); reason != "" {
		res.Status, res.Reason = StatusSkipped, reason
		return res
	}

	latest, err := resolveLatest(src, lister)
	if err != nil {
		res.Status, res.Reason = StatusSkipped, err.Error()
		return res
	}
	res.LatestVersion = latest

	if !isNewer(src.Version, latest) {
		res.Status = StatusUpToDate
		return res
	}

	res.Status = StatusUpdated
	if !params.DryRun {
		if err := SetComponentVersion(file, src.Component, latest); err != nil {
			res.Status, res.Reason = StatusSkipped, err.Error()
		}
	}
	return res
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

// sourceMatchesFilter applies the component and tags filters.
func sourceMatchesFilter(src *schema.AtmosVendorSource, component string, tags []string) bool {
	if component != "" && src.Component != component {
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
