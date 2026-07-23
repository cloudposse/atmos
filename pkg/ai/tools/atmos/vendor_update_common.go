package atmos

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/vendoring"
)

// paramTags is the vendor tag-filter parameter shared by the vendor
// check-updates and update tools.
const paramTags = "tags"

// extractStringSliceParam extracts an optional []string parameter, which
// arrives as []interface{} when tool arguments are decoded from JSON.
func extractStringSliceParam(params map[string]interface{}, name string) []string {
	raw, ok := params[name]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// runVendorUpdateCheck resolves and version-checks either a single component
// (when component is non-empty) or every source declared in the project's
// vendor.yaml (when component is empty), without discovering component.yaml
// extra sources -- a deliberately narrower scope than `atmos vendor update`'s
// full repo-wide sweep.
func runVendorUpdateCheck(component string, tags []string, dryRun bool) (*vendoring.UpdateReport, error) {
	if component != "" {
		resolved, err := vendoring.ResolveComponentSource(&vendoring.ResolveSourceParams{Component: component})
		if err != nil {
			return nil, err
		}
		return vendoring.UpdateResolved(resolved, &vendoring.UpdateParams{Tags: tags, DryRun: dryRun})
	}

	vendorFile, found := vendoring.VendorFilePresent("")
	if !found {
		return nil, errUtils.ErrAIVendorFileNotFound
	}
	files, err := vendoring.CollectManifestFiles(vendorFile)
	if err != nil {
		return nil, err
	}
	return vendoring.Update(nil, &vendoring.UpdateParams{VendorFiles: files, Tags: tags, DryRun: dryRun})
}

// buildVendorUpdateResult formats an UpdateReport into a tools.Result, shared
// by atmos_vendor_check_updates (dryRun always true) and atmos_vendor_update.
func buildVendorUpdateResult(report *vendoring.UpdateReport, dryRun bool) *tools.Result {
	var out strings.Builder
	if dryRun {
		out.WriteString("Vendor update check:\n\n")
	} else {
		out.WriteString("Vendor update:\n\n")
	}

	entries := make([]map[string]interface{}, 0, len(report.Results))
	for _, r := range report.Results {
		line := fmt.Sprintf("  - %s: %s", r.Component, r.Status)
		if r.CurrentVersion != "" || r.LatestVersion != "" {
			line += fmt.Sprintf(" (%s -> %s)", r.CurrentVersion, r.LatestVersion)
		}
		if r.Archived {
			line += " [archived]"
		}
		if r.Reason != "" {
			line += fmt.Sprintf(" -- %s", r.Reason)
		}
		out.WriteString(line + "\n")

		entries = append(entries, map[string]interface{}{
			paramComponent:    r.Component,
			"file":            r.File,
			"current_version": r.CurrentVersion,
			"latest_version":  r.LatestVersion,
			"status":          string(r.Status),
			"reason":          r.Reason,
			"archived":        r.Archived,
		})
	}
	if len(report.Results) == 0 {
		out.WriteString("  (no vendored sources found)\n")
	}

	return &tools.Result{
		Success: true,
		Output:  out.String(),
		Data: map[string]interface{}{
			"updated_count": report.UpdatedCount(),
			"results":       entries,
		},
	}
}
