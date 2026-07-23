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

// runWithProgressAdapter bridges cmd/vendor's own bubbletea-spinner runUpdateWithSpinner (named
// vendorUpdateWork/vendorProgressFunc types) to pkg/vendoring/updater.RunWithProgress's unnamed
// callback shape, since Go does not treat a named function type and a structurally-equivalent
// unnamed one as interchangeable once nested inside another function type.
func runWithProgressAdapter(doWork func(onProgress func(component string, index, total int)) (*vendoring.UpdateReport, error)) (*vendoring.UpdateReport, error) {
	return runUpdateWithSpinner(func(onProgress vendorProgressFunc) (*vendoring.UpdateReport, error) {
		return doWork(onProgress)
	})
}

// selectionParams builds updater.SelectionParams from p, binding RepoWideUpdate to this package's
// own runRepoWideUpdate (which pkg/vendoring/updater cannot call directly -- it depends on
// cmd/vendor's viper-bound flag reads) and RunWithProgress to the bubbletea spinner adapter above.
func (p *vendorUpdateParams) selectionParams() *updater.SelectionParams {
	return &updater.SelectionParams{
		Viper:           p.viper,
		ComponentType:   p.componentType,
		Tags:            p.tags,
		Group:           p.group,
		Check:           p.check,
		VendorFile:      p.viper.GetString("file"),
		RunWithProgress: runWithProgressAdapter,
		RepoWideUpdate: func(check bool) (*vendoring.UpdateReport, error) {
			return runRepoWideUpdate(p.viper, repoWideUpdateParams{typeChanged: p.typeChanged, componentType: p.componentType, tags: p.tags, check: check})
		},
	}
}

func runVendorUpdate(p *vendorUpdateParams) (*vendoring.UpdateReport, error) {
	v := p.viper
	// A nil selection means a direct --group invocation needs discovery. A
	// non-nil selection was already narrowed by the --pull-request discovery
	// pass and must not be rediscovered or widened.
	if p.group != "" && p.components == nil {
		finalReport, components, err := updater.ResolveGroupSelection(p.selectionParams())
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
	return updater.UpdateSelectedComponents(p.selectionParams(), p.components)
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
