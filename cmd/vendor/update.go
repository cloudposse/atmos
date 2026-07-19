package vendor

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ci"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	_ "github.com/cloudposse/atmos/pkg/git/providers/cli"
	_ "github.com/cloudposse/atmos/pkg/git/providers/github"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring"
	"github.com/cloudposse/atmos/pkg/vendoring/updater"
)

var vendorUpdateParser *flags.StandardParser

// vendorUpdateCmd checks Git sources for newer allowed versions and updates the
// version fields in the vendor manifest(s), preserving formatting.
var vendorUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update vendored component versions to the latest allowed release",
	Long: `Check each Git-backed source in the vendor manifest for a newer version (honoring
any per-source constraints) and update the version field in place, preserving
comments, anchors, and templates. Use --check for a dry run.`,
	Example: "atmos vendor update --check\natmos vendor update --component vpc\natmos vendor update --group platform --check\natmos vendor update --all --format json\natmos vendor update --pull\natmos vendor update --pull-request",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(nil, "vendor.updateRunE")()

		v := viper.GetViper()
		if err := vendorUpdateParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		check := v.GetBool("check")
		components, flagErr := cmd.Flags().GetStringSlice("component")
		if flagErr != nil {
			return flagErr
		}
		if !cmd.Flags().Changed("component") {
			components = nil
		}
		// The shared flag parser serializes an omitted StringSlice default as
		// the literal "[]" in a few embedded-command test paths. Treat that
		// representation as the empty selector users intended.
		components = normalizeComponentSelectors(components)
		component := ""
		if len(components) == 1 {
			component = components[0]
		}
		componentType := v.GetString("type")
		tags := splitTags(v.GetString("tags"))
		typeChanged := cmd.Flags().Changed("type")
		pullRequest := v.GetBool("pull-request")
		all := v.GetBool("all")
		group := v.GetString("group")
		format := v.GetString("format")
		scope := updateScope(group, components)
		result := updater.Result{Scope: scope, Check: check, Status: "no_updates"}
		defer func() {
			if !vendorSummaryEnabled(v) {
				return
			}
			// CI summary output is intentionally best-effort: it must never hide
			// the actual update, push, or API result.
			_ = ci.WriteStepSummary(updater.MarkdownSummary(&result))
		}()

		invocation := updateInvocation{PullRequest: pullRequest, All: all, Group: group, Components: components}
		if err := validateUpdateInvocation(v, cmd, invocation); err != nil {
			result.Status, result.Failure = "failed", err.Error()
			return err
		}
		if pullRequest {
			// Publishing necessarily pulls the updated sources, even if --pull was
			// omitted. --check remains strictly mutation-free.
			v.Set("pull", true)
		}

		var (
			report     *vendoring.UpdateReport
			baseBranch string
		)
		var err error

		// Discover first for PR publication. This guarantees a no-op update does
		// not create a branch, commit, push, or pull request.
		selected := components
		if pullRequest && !check {
			discovery, dErr := runVendorUpdate(v, componentType, tags, typeChanged, selected, group, true)
			if dErr != nil {
				result.Status, result.Failure = "failed", dErr.Error()
				return dErr
			}
			if discovery.UpdatedCount() == 0 {
				result.Updates, result.Updated = discovery.Results, 0
				renderVendorUpdateResult(discovery, true, v, format)
				if err := renderComponentUpdaterJSON(&result, format); err != nil {
					result.Status, result.Failure = "failed", err.Error()
					return err
				}
				return nil
			}
			if group != "" {
				selected = updatedComponents(discovery)
			}
			branch, base, pErr := prepareComponentUpdateBranch(cmd.Context(), v, scope)
			if pErr != nil {
				result.Status, result.Failure = "failed", pErr.Error()
				return pErr
			}
			result.Branch = branch
			baseBranch = base
		}

		report, err = runVendorUpdate(v, componentType, tags, typeChanged, selected, group, check)

		if report != nil {
			applyComponentUpdaterReport(&result, report)
			renderVendorUpdateResult(report, check, v, format)
		}
		if err != nil {
			result.Status, result.Failure = "failed", err.Error()
			return err
		}

		// Reconciliation is independent of version discovery: a matching version
		// can still have an absent or locally modified materialization. The pull
		// executor uses vendor.lock.yaml to skip fully verified targets.
		if report != nil && v.GetBool("pull") && !check {
			err = runVendorPull(cmd, args, report, vendorPullParams{
				component:     component,
				componentType: componentType,
				dryRun:        v.GetBool("dry-run"),
			})
			if err != nil {
				result.Status, result.Failure = "failed", err.Error()
				return err
			}
		}
		if pullRequest && !check && report != nil && report.UpdatedCount() > 0 {
			pr, commit, pErr := publishComponentUpdate(cmd.Context(), v, componentUpdatePublication{scope: scope, branch: result.Branch, base: baseBranch, report: report})
			if pErr != nil {
				result.Status, result.Failure = "failed", pErr.Error()
				return pErr
			}
			result.Commit = commit
			result.PullRequest = pr
		}
		if err := renderComponentUpdaterJSON(&result, format); err != nil {
			result.Status, result.Failure = "failed", err.Error()
			return err
		}
		return nil
	},
}

func applyComponentUpdaterReport(result *updater.Result, report *vendoring.UpdateReport) {
	result.Updates, result.Updated = report.Results, report.UpdatedCount()
	if result.Updated > 0 {
		result.Status = "updated"
		return
	}
	result.Status = "no_updates"
}

// repoWideUpdateParams bundles runRepoWideUpdate's inputs (an Options-pattern struct, since the
// argument list grew past a readable positional length once OnProgress joined it).
type repoWideUpdateParams struct {
	typeChanged   bool
	componentType string
	tags          []string
	check         bool
	onProgress    vendorProgressFunc
}

// runRepoWideUpdate handles the --component-less path: vendor.yaml's sources, combined with a
// sweep of per-component component.yaml manifests. The sweep runs automatically whenever no
// vendor.yaml is found, so a repo that vendors exclusively via component.yaml (no vendor.yaml at
// all) works out of the box with no extra flag. --component-manifests additionally forces the
// sweep to run even when a vendor.yaml IS present, for repos that mix both manifest styles. A hard
// error is only raised when the sweep finds nothing to update either.
func runRepoWideUpdate(v *viper.Viper, p repoWideUpdateParams) (*vendoring.UpdateReport, error) {
	includeComponentManifests := v.GetBool("component-manifests")

	vendorFile, hasVendorFile := vendoring.VendorFilePresent(v.GetString("file"))
	var files []string
	if hasVendorFile {
		var err error
		files, err = vendoring.CollectManifestFiles(vendorFile)
		if err != nil {
			return nil, err
		}
	}

	var extra []*vendoring.ResolvedSource
	if includeComponentManifests || !hasVendorFile {
		found, err := vendoring.DiscoverAllComponentManifests(p.componentType, p.typeChanged)
		if err != nil {
			return nil, err
		}
		extra = found
	}

	if !hasVendorFile && len(extra) == 0 {
		if _, err := resolveVendorFileWithOverride(v.GetString("file")); err != nil {
			return nil, err
		}
	}

	updateType := ""
	if p.typeChanged {
		updateType = p.componentType
	}

	return vendoring.Update(nil, &vendoring.UpdateParams{
		VendorFiles:  files,
		ExtraSources: extra,
		Tags:         p.tags,
		Type:         updateType,
		DryRun:       p.check,
		OnProgress:   p.onProgress,
	})
}

func init() {
	vendorUpdateParser = flags.NewStandardParser(
		flags.WithStringSliceFlag("component", "c", []string{}, "Update only these components (repeatable)"),
		flags.WithStringFlag("type", "t", "terraform", "Component type (terraform, helmfile, or packer)"),
		flags.WithStringFlag("tags", "", "", "Update only components with any of these comma-separated tags"),
		flags.WithBoolFlag("check", "", false, "Dry run: show available updates without modifying files"),
		flags.WithBoolFlag("pull", "", false, "After updating versions, run 'atmos vendor pull'"),
		flags.WithBoolFlag("all", "", false, "Update all discoverable vendor sources (the default when no selector is given)"),
		flags.WithBoolFlag("pull-request", "", false, "Commit, push, and create or update a pull request for available updates"),
		flags.WithStringFlag("group", "", "", "Update the named vendor.update.groups selection"),
		flags.WithStringFlag("format", "", "table", "Output format: table or json"),
		flags.WithBoolFlag("component-manifests", "", false,
			"Also check per-component component.yaml manifests when a vendor.yaml is present (automatic when no vendor.yaml exists)"),
		flags.WithBoolFlag("outdated", "", false, "Show only sources with an available update"),
		flags.WithBoolFlag("archived", "", false, "Show only sources whose upstream repository is archived"),
		flags.WithStringFlag("file", "", "", "Vendor manifest file (default: ./vendor.yaml)"),
		// Flags consumed by 'vendor pull' when --pull is set.
		flags.WithStringFlag("stack", "s", "", "Only pull the specified stack (used with --pull)"),
		flags.WithBoolFlag("everything", "", false, "Pull all components (used with --pull)"),
		flags.WithBoolFlag("dry-run", "", false, "Simulate the pull (used with --pull)"),
	)
	vendorUpdateParser.RegisterFlags(vendorUpdateCmd)
	if err := vendorUpdateParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	vendorCmd.AddCommand(vendorUpdateCmd)
}

// vendorPullParams bundles runVendorPull's inputs beyond cmd/args/report (an Options-pattern
// struct, since the argument list grew past a readable positional length once componentType and
// dryRun joined it - see repoWideUpdateParams above for the same convention).
type vendorPullParams struct {
	component     string
	componentType string
	dryRun        bool
}

// runVendorPull invokes the existing vendor pull execution after an update.
//
// When p.component is set (the single-component "--component X --pull" path), behavior is
// unchanged: that path already pulls correctly regardless of whether the component's source
// comes from vendor.yaml or a standalone component.yaml, so it's delegated straight through.
//
// When p.component is empty (a repo-wide "--pull" sweep), pull only the components update actually
// changed instead of setting --everything=true. --everything only knows how to enumerate a
// vendor.yaml's sources and hard-errors when one doesn't exist (internal/exec/vendor.go's
// handleVendorConfig / ErrVendorConfigNotExist), which broke repo-wide "--pull" in a
// component.yaml-only repo (no vendor.yaml at all) even though every updated component's own pull
// already worked fine. Re-pulling only what changed is also strictly better behavior on its own
// merits: there's no reason to re-pull untouched (up-to-date/skipped/failed) components after an
// update.
//
// The updated results are partitioned in two:
//   - Components declared via their own component.yaml/component.yml manifest are pulled together
//     in a single ExecuteComponentVendorPullBatch call, producing one progress bar and one
//     completion summary instead of one "0/1" block per component (the reported UX bug).
//   - Everything else (vendor.yaml-declared sources, including ones reached via an import) keeps
//     using the existing per-component pullUpdatedComponent loop: executeVendorModel's generic
//     package-type constraint means a vendor.yaml-declared package can't be batched into the same
//     call as a component.yaml-declared one, and batching the vendor.yaml path itself would require
//     touching ExecuteAtmosVendorInternal's component-filtering, out of scope for this fix.
func runVendorPull(cmd *cobra.Command, args []string, report *vendoring.UpdateReport, p vendorPullParams) error {
	if p.component != "" {
		// Clear "stack"/"tags" the same way pullUpdatedComponent does: they're vendor update's
		// own flags of the same name (used, e.g., with a repo-wide "--tags foo --pull" run) and
		// validateVendorFlags (internal/exec/vendor.go) rejects "component" combined with either.
		if err := resetUnchangedFlag(cmd, "stack"); err != nil {
			return err
		}
		if err := resetUnchangedFlag(cmd, "tags"); err != nil {
			return err
		}
		return e.ExecuteVendorPullCmd(cmd, args)
	}

	batchComponentsByType, fallback := partitionPullResults(report)

	var errs []error
	// Batch per type: DiscoverAllComponentManifests' repo-wide sweep (no explicit --type) can mix
	// terraform/helmfile/packer component.yaml updates in one report, and
	// ExecuteComponentVendorPullBatch only accepts a single type per call (it resolves every
	// component's directory under that one type's base path) - forwarding a mixed batch under one
	// type would resolve some components under the wrong components/<type>/<name> path.
	for componentType, components := range batchComponentsByType {
		if err := pullBatchedComponentManifests(components, componentType, p.dryRun); err != nil {
			errs = append(errs, err)
		}
	}
	for _, result := range fallback {
		if err := pullUpdatedComponent(cmd, args, result.Component); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// componentManifestBasenames are the physical file basenames a component.yaml-declared source's
// SourceUpdateResult.File can carry (see ReadAndProcessComponentVendorConfigFile's
// findComponentConfigFile), used by partitionReportResults to distinguish it from a
// vendor.yaml-declared source (vendor.yaml itself, or any file it imports).
var componentManifestBasenames = map[string]bool{
	"component.yaml": true,
	"component.yml":  true,
}

// partitionPullResults selects every report entry because an unchanged version
// can still need materialization reconciliation against vendor.lock.yaml.
func partitionPullResults(report *vendoring.UpdateReport) (batchComponentsByType map[string][]string, fallback []vendoring.SourceUpdateResult) {
	return partitionReportResults(report, false)
}

// partitionReportResults splits report's results (optionally filtered to StatusUpdated only,
// via updatedOnly) into components declared via their own component.yaml/component.yml manifest
// (eligible for the batched ExecuteComponentVendorPullBatch call, grouped by ComponentType since a
// repo-wide sweep can mix types in one report) versus everything else (vendor.yaml or an imported
// manifest file), which keeps using the existing per-component pullUpdatedComponent loop.
func partitionReportResults(report *vendoring.UpdateReport, updatedOnly bool) (batchComponentsByType map[string][]string, fallback []vendoring.SourceUpdateResult) {
	batchComponentsByType = map[string][]string{}
	for i := range report.Results {
		result := report.Results[i]
		if updatedOnly && result.Status != vendoring.StatusUpdated {
			continue
		}
		if componentManifestBasenames[filepath.Base(result.File)] {
			batchComponentsByType[result.ComponentType] = append(batchComponentsByType[result.ComponentType], result.Component)
			continue
		}
		fallback = append(fallback, result)
	}
	return batchComponentsByType, fallback
}

// pullBatchedComponentManifests initializes the CLI config the same way other component-manifest
// resolution call sites do (e.g. pkg/vendoring/resolve.go's DefaultComponentDirResolver) and pulls
// every entry in components in a single ExecuteComponentVendorPullBatch call.
func pullBatchedComponentManifests(components []string, componentType string, dryRun bool) error {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}
	return e.ExecuteComponentVendorPullBatch(&atmosConfig, components, componentType, dryRun)
}

// pullUpdatedComponent drives a single "vendor pull --component <component>" call by resetting, on
// the shared cmd, every flag ExecuteVendorPullCmd/parseVendorFlags (internal/exec/vendor.go) reads
// that could otherwise carry stale state across loop iterations or leak in from vendor update's own
// flags of the same name:
//   - "component" is set to the target component.
//   - "everything" is force-reset to false so it never wins over "component" (parseVendorFlags'
//     setDefaultEverythingFlag only defaults it to true when nothing else is set, but a prior
//     iteration - or an earlier design of this function - could otherwise have left it true).
//   - "stack" and "tags" are cleared via resetUnchangedFlag (not cmd.Flags().Set, see its doc)
//     since validateVendorFlags rejects "component" combined with either ("--component" and
//     "--stack" are mutually exclusive, likewise "--component" and "--tags"): a
//     "vendor update --tags foo --pull" or "--stack bar --pull" run would otherwise fail here even
//     though the top-level update already resolved exactly which components to pull.
//
// "type" and "dry-run" are intentionally left untouched: they're identical across every iteration
// (the same --type/--dry-run the user passed to "vendor update"), so vendorUpdateCmd's own "type"
// flag (shared with the pull path) continues to thread through correctly, e.g.
// "vendor update --type packer --pull" pulls with "--type packer" too.
func pullUpdatedComponent(cmd *cobra.Command, args []string, component string) error {
	if err := cmd.Flags().Set("component", component); err != nil {
		return err
	}
	if err := cmd.Flags().Set("everything", "false"); err != nil {
		return err
	}
	if err := resetUnchangedFlag(cmd, "stack"); err != nil {
		return err
	}
	if err := resetUnchangedFlag(cmd, "tags"); err != nil {
		return err
	}
	return e.ExecuteVendorPullCmd(cmd, args)
}

// resetUnchangedFlag clears name's value back to "" and marks it Changed=false, rather than merely
// calling cmd.Flags().Set (which unconditionally marks a flag Changed=true, even when set to "").
// This distinction matters for "stack": ExecuteVendorPullCommand (internal/exec/vendor.go) reads
// flags.Changed("stack") directly - not its value - to decide whether to process stacks at all.
// Leaving "stack" spuriously marked Changed after this per-component pull loop would force stack
// processing even though no --stack was ever provided, which can fail outright in a repo with no (or
// minimal) stack configuration. "tags" has no such Changed() reader today but is reset the same way
// defensively, in case one is added later.
func resetUnchangedFlag(cmd *cobra.Command, name string) error {
	f := cmd.Flags().Lookup(name)
	if f == nil {
		return nil
	}
	if err := f.Value.Set(""); err != nil {
		return err
	}
	f.Changed = false
	return nil
}

// splitTags splits a comma-separated tag list, trimming whitespace and empties.
func splitTags(csv string) []string {
	if strings.TrimSpace(csv) == "" {
		return nil
	}
	var tags []string
	for _, t := range strings.Split(csv, ",") {
		if trimmed := strings.TrimSpace(t); trimmed != "" {
			tags = append(tags, trimmed)
		}
	}
	return tags
}
