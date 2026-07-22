package vendor

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/data"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/vendoring"
	"github.com/cloudposse/atmos/pkg/vendoring/updater"
)

// vendorUpdateParams bundles runVendorUpdate's inputs (an Options-pattern struct, mirroring
// repoWideUpdateParams/vendorPullParams in update.go).
type vendorUpdateParams struct {
	viper         *viper.Viper
	componentType string
	tags          []string
	typeChanged   bool
	components    []string
	group         string
	check         bool
}

func runVendorUpdate(p *vendorUpdateParams) (*vendoring.UpdateReport, error) {
	v := p.viper
	// A nil selection means a direct --group invocation needs discovery. A
	// non-nil selection was already narrowed by the --pull-request discovery
	// pass and must not be rediscovered or widened.
	if p.group != "" && p.components == nil {
		finalReport, components, err := resolveGroupSelection(v, p)
		if err != nil {
			return nil, err
		}
		if finalReport != nil {
			return finalReport, nil
		}
		p.components = components
	}
	if p.group != "" && len(p.components) == 0 {
		// Never let an empty group selection fall through to an unscoped update.
		return &vendoring.UpdateReport{}, nil
	}
	if len(p.components) == 0 {
		return runUpdateWithSpinner(func(onProgress vendorProgressFunc) (*vendoring.UpdateReport, error) {
			return runRepoWideUpdate(v, repoWideUpdateParams{typeChanged: p.typeChanged, componentType: p.componentType, tags: p.tags, check: p.check, onProgress: onProgress})
		})
	}
	return updateSelectedComponents(v, p)
}

// resolveGroupSelection discovers a --group invocation's outdated components via a check-mode
// repo-wide update, narrowed by the group's include/exclude patterns. When p.check is set, or the
// resulting selection is empty, it returns a non-nil finalReport for runVendorUpdate to return
// immediately; otherwise it returns the resolved component names for runVendorUpdate to continue
// with.
func resolveGroupSelection(v *viper.Viper, p *vendorUpdateParams) (finalReport *vendoring.UpdateReport, components []string, err error) {
	discovery, err := runRepoWideUpdate(v, repoWideUpdateParams{typeChanged: p.typeChanged, componentType: p.componentType, tags: p.tags, check: true})
	if err != nil {
		return nil, nil, err
	}
	components = updater.FilterGroupComponents(discovery, v.GetStringSlice("vendor.update.groups."+p.group+".include"), v.GetStringSlice("vendor.update.groups."+p.group+".exclude"))
	if p.check {
		return updater.FilterReport(discovery, components), nil, nil
	}
	if len(components) == 0 {
		return &vendoring.UpdateReport{}, nil, nil
	}
	return nil, components, nil
}

// updateSelectedComponents runs the update for each of p.components individually, resolving each
// component's declared source before updating it.
func updateSelectedComponents(v *viper.Viper, p *vendorUpdateParams) (*vendoring.UpdateReport, error) {
	results := make([]vendoring.SourceUpdateResult, 0, len(p.components))
	for _, component := range p.components {
		resolved, err := vendoring.ResolveComponentSource(&vendoring.ResolveSourceParams{VendorFile: v.GetString("file"), Component: component, ComponentType: p.componentType})
		if err != nil {
			return nil, err
		}
		report, err := runUpdateWithSpinner(func(onProgress vendorProgressFunc) (*vendoring.UpdateReport, error) {
			return vendoring.UpdateResolved(resolved, &vendoring.UpdateParams{Tags: p.tags, DryRun: p.check, OnProgress: onProgress})
		})
		if err != nil {
			return nil, err
		}
		results = append(results, report.Results...)
	}
	return &vendoring.UpdateReport{Results: results}, nil
}

func normalizeComponentSelectors(components []string) []string {
	if components == nil {
		return nil
	}
	result := make([]string, 0, len(components))
	for _, component := range components {
		component = strings.TrimSpace(component)
		if component != "" && component != "[]" {
			result = append(result, component)
		}
	}
	return result
}

// validateUpdateInvocation extracts the viper/cobra values validation needs into
// updater.ValidationConfig and delegates to updater.ValidateInvocation.
func validateUpdateInvocation(v *viper.Viper, cmd *cobra.Command, invocation updater.Invocation) error {
	config := updater.ValidationConfig{
		Format:          v.GetString("format"),
		ExecutionMode:   v.GetString("vendor.update.execution.mode"),
		BatchingMode:    v.GetString("vendor.update.batching.mode"),
		GroupConfigured: v.IsSet("vendor.update.groups." + invocation.Group),
		Templates: updater.PRTemplates{
			Title: v.GetString("vendor.ci.pull_request.title"),
			Body:  v.GetString("vendor.ci.pull_request.body"),
		},
	}
	checkExplicitlyRequested := cmd.Flags().Changed("check") && v.GetBool("check")
	return updater.ValidateInvocation(invocation, &config, checkExplicitlyRequested)
}

func updatedComponents(report *vendoring.UpdateReport) []string {
	var components []string
	for _, row := range report.Results {
		if row.Status == vendoring.StatusUpdated {
			components = append(components, row.Component)
		}
	}
	return components
}

func vendorSummaryEnabled(v *viper.Viper) bool {
	return !v.IsSet("vendor.ci.summary.enabled") || v.GetBool("vendor.ci.summary.enabled")
}

func renderVendorUpdateResult(report *vendoring.UpdateReport, check bool, v *viper.Viper, format string) {
	if format == "json" {
		return
	}
	renderUpdateReport(report, check, v.GetBool("outdated"), v.GetBool("archived"))
}

func renderComponentUpdaterJSON(result *updater.Result, format string) error {
	if format != "json" {
		return nil
	}
	return data.WriteJSON(result)
}

// currentWorkdir and gitHubRepository are the CLI layer's test seams, passed explicitly into
// pkg/vendoring/updater's PrepareBranch/PublishComponentUpdate calls.
var (
	currentWorkdir   = "."
	gitHubRepository = atmosgit.GitHubRepository
)
