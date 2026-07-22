package vendor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ci"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/flags"
	_ "github.com/cloudposse/atmos/pkg/git/providers/cli"
	_ "github.com/cloudposse/atmos/pkg/git/providers/github"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring"
	"github.com/cloudposse/atmos/pkg/vendoring/install"
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
comments, anchors, and templates. Use --check for a dry run. This never checks whether
what's already on disk matches vendor.lock.yaml — see 'atmos vendor verify' for that.`,
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
		scope := updater.UpdateScope(group, components)
		result := updater.Result{Scope: scope, Check: check, Status: "no_updates"}
		defer func() {
			if !vendorSummaryEnabled(v) {
				return
			}
			// CI summary output is intentionally best-effort: it must never hide
			// the actual update, push, or API result.
			_ = ci.WriteStepSummary(updater.MarkdownSummary(&result))
		}()

		invocation := updater.Invocation{PullRequest: pullRequest, All: all, Group: group, Components: components}
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
			workdir    = currentWorkdir
		)
		var err error

		// Discover first for PR publication. This guarantees a no-op update does
		// not create a branch, commit, push, or pull request.
		selected := components
		if pullRequest && !check {
			execWorkdir, ewErr := resolveExecutionWorkdir(cmd.Context(), v, currentWorkdir)
			if ewErr != nil {
				result.Status, result.Failure = "failed", ewErr.Error()
				return ewErr
			}
			defer execWorkdir.Cleanup()
			workdir = execWorkdir.Workdir

			discovery, dErr := runVendorUpdate(&vendorUpdateParams{viper: v, componentType: componentType, tags: tags, typeChanged: typeChanged, components: selected, group: group, check: true})
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
			baseBranchOverride := v.GetString("vendor.ci.pull_request.base_branch")
			if execWorkdir.ResolvedBase != "" {
				// vendor.update.execution.mode: worktree already resolved the base branch once
				// (to check out the worktree itself) -- reuse it instead of re-resolving via a
				// second remote call.
				baseBranchOverride = execWorkdir.ResolvedBase
			}
			branch, base, pErr := updater.PrepareBranch(cmd.Context(), workdir, "origin", baseBranchOverride, v.GetString("vendor.ci.pull_request.branch_prefix"), scope)
			if pErr != nil {
				result.Status, result.Failure = "failed", pErr.Error()
				return pErr
			}
			result.Branch = branch
			baseBranch = base
		}

		report, err = runVendorUpdate(&vendorUpdateParams{viper: v, componentType: componentType, tags: tags, typeChanged: typeChanged, components: selected, group: group, check: check})

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
				component:       component,
				componentType:   componentType,
				dryRun:          v.GetBool("dry-run"),
				refreshLock:     v.GetBool("refresh-lock"),
				lockEnforcement: v.GetString("lock-enforcement"),
			})
			if err != nil {
				result.Status, result.Failure = "failed", err.Error()
				return err
			}
		}
		if pullRequest && !check && report != nil && report.UpdatedCount() > 0 {
			publication := updater.Publication{Scope: scope, Branch: result.Branch, Base: baseBranch, Report: report}
			prConfig := vendorPullRequestConfig(v)
			pr, commit, pErr := updater.PublishComponentUpdate(cmd.Context(), workdir, "origin", publication, &prConfig, gitHubRepository)
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

// vendorPullRequestConfig extracts the vendor.ci.pull_request.* viper values into the typed
// schema.VendorPullRequestConfig pkg/vendoring/updater's publish functions take, keeping that
// package's own viper reads at zero.
func vendorPullRequestConfig(v *viper.Viper) schema.VendorPullRequestConfig {
	return schema.VendorPullRequestConfig{
		Provider:     v.GetString("vendor.ci.pull_request.provider"),
		BaseBranch:   v.GetString("vendor.ci.pull_request.base_branch"),
		BranchPrefix: v.GetString("vendor.ci.pull_request.branch_prefix"),
		Title:        v.GetString("vendor.ci.pull_request.title"),
		Body:         v.GetString("vendor.ci.pull_request.body"),
		Labels:       v.GetStringSlice("vendor.ci.pull_request.labels"),
		Draft:        v.GetBool("vendor.ci.pull_request.draft"),
		Reviewers:    v.GetStringSlice("vendor.ci.pull_request.reviewers"),
		Assignees:    v.GetStringSlice("vendor.ci.pull_request.assignees"),
	}
}

// executionWorkdir is the outcome of resolveExecutionWorkdir.
type executionWorkdir struct {
	// Workdir is the git workdir the --pull-request publish path's branch/commit/push operations
	// use: currentWorkdir unchanged for the default execution mode, or the isolated worktree's path
	// for vendor.update.execution.mode: worktree.
	Workdir string
	// ResolvedBase is the concrete base branch worktree mode already resolved (so the caller can
	// reuse it instead of re-resolving via a second remote call); empty for the default execution
	// mode, where the caller keeps using its own vendor.ci.pull_request.base_branch config,
	// unresolved, exactly as before this feature existed.
	ResolvedBase string
	// Cleanup unwinds whatever resolveExecutionWorkdir set up. Always non-nil, including on error,
	// so callers can unconditionally `defer execWorkdir.Cleanup()` right after the call; it is a
	// no-op for the default execution mode.
	Cleanup func()
}

// resolveExecutionWorkdir sets up the git workdir (and, for worktree mode, the BasePath redirect)
// the --pull-request publish path uses for its whole discover -> bump -> branch -> commit -> push
// cycle. When vendor.update.execution.mode is "worktree", that entire cycle runs inside an isolated
// git worktree, leaving the invoking checkout's working tree byte-for-byte unchanged (no modified
// files, no branch switch, no new local branch). When execution mode is not "worktree" (empty or
// "current"), the returned executionWorkdir.Workdir is currentWorkdir unchanged, ResolvedBase is
// empty, and Cleanup is a no-op -- this is a strict superset feature and never changes the default
// path's behavior.
//
// The redirect combines two mechanisms, both necessary (confirmed empirically, not just by reading
// cfg.InitCliConfig):
//
//  1. Temporarily pointing the ATMOS_BASE_PATH environment variable at the worktree. Every
//     path-resolution call in pkg/vendoring's discovery/resolve/write chain
//     (DiscoverAllComponentManifests, DefaultComponentDirResolver.ComponentDir,
//     ResolveComponentSource's component.yaml fallback) calls cfg.InitCliConfig fresh with an empty
//     schema.ConfigAndStacksInfo{}, so nothing in that chain supplies a stronger override -- only an
//     explicit ConfigAndStacksInfo.AtmosBasePath (the CLI --base-path flag or Terraform provider
//     param, never set by those call sites) would outrank the env var.
//  2. Temporarily changing the process's actual working directory to the worktree, via pkg/env's
//     existing Chdir primitive (the same one --chdir itself uses). This is required because
//     VendorFilePresent's default lookup (no --file override, no vendor.base_path configured in
//     atmos.yaml -- the common case) checks for "./vendor.yaml" relative to the process's real
//     working directory *before* ever consulting atmosConfig.BasePath -- ATMOS_BASE_PATH alone does
//     not redirect that check, so without also moving the process cwd, that lookup would keep
//     resolving (and writing) vendor.yaml in the invoking checkout instead of the worktree.
func resolveExecutionWorkdir(ctx context.Context, v *viper.Viper, currentWorkdir string) (*executionWorkdir, error) {
	defer perf.Track(nil, "vendor.resolveExecutionWorkdir")()

	if v.GetString("vendor.update.execution.mode") != "worktree" {
		return &executionWorkdir{Workdir: currentWorkdir, Cleanup: func() {}}, nil
	}

	prepared, wErr := updater.PrepareUpdateWorktree(ctx, currentWorkdir, "origin", v.GetString("vendor.ci.pull_request.base_branch"))
	if wErr != nil {
		return &executionWorkdir{Cleanup: func() {}}, wErr
	}

	restoreBasePath, envErr := env.SetWithRestore(map[string]string{"ATMOS_BASE_PATH": prepared.Path})
	if envErr != nil {
		prepared.Cleanup()
		return &executionWorkdir{Cleanup: func() {}}, envErr
	}

	originalWd, wdErr := os.Getwd()
	if wdErr != nil {
		restoreBasePath()
		prepared.Cleanup()
		return &executionWorkdir{Cleanup: func() {}}, wdErr
	}
	if chdirErr := env.Chdir(prepared.Path); chdirErr != nil {
		restoreBasePath()
		prepared.Cleanup()
		return &executionWorkdir{Cleanup: func() {}}, chdirErr
	}

	cleanup := func() {
		_ = env.Chdir(originalWd)
		restoreBasePath()
		prepared.Cleanup()
	}
	return &executionWorkdir{Workdir: prepared.Path, ResolvedBase: prepared.Base, Cleanup: cleanup}, nil
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
		flags.WithStringFlag("type", "t", "terraform", componentTypeFlagHelp),
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
		flags.WithBoolFlag("everything", "", false, "Pull all components (used with --pull)"),
		flags.WithBoolFlag("dry-run", "", false, "Simulate the pull (used with --pull)"),
		flags.WithBoolFlag("refresh-lock", "", false, "Refresh immutable vendor lock entries from declared sources (used with --pull)"),
		flags.WithStringFlag("lock-enforcement", "", "", "Override vendor.lock.enforcement (strict, warn, or silent; used with --pull)"),
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
	// refreshLock/lockEnforcement carry vendor update's own --refresh-lock/--lock-enforcement
	// flags through to the repo-wide "--pull" sweep path (pullBatchedComponentManifests); the
	// single-component "--component X --pull" path instead reads them directly off cmd's flags
	// (registered on vendorUpdateCmd itself, see init()) via ExecuteVendorPullCmd's own
	// parseVendorFlags, so they don't need to be threaded through this struct for that path too.
	refreshLock     bool
	lockEnforcement string
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
		// Clear "tags" the same way pullUpdatedComponent does: it's vendor update's own flag of
		// the same name (used, e.g., with a repo-wide "--tags foo --pull" run) and
		// validateVendorFlags (internal/exec/vendor.go) rejects "component" combined with it.
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
		if err := pullBatchedComponentManifests(&batchedComponentManifestsParams{
			components:      components,
			componentType:   componentType,
			dryRun:          p.dryRun,
			refreshLock:     p.refreshLock,
			lockEnforcement: p.lockEnforcement,
		}); err != nil {
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

// batchedComponentManifestsParams bundles pullBatchedComponentManifests' inputs (Options Pattern,
// CLAUDE.md: two adjacent bools (dryRun/refreshLock) plus lockEnforcement crossed both the
// same-type-adjacent and >4-total-parameters thresholds once vendor update grew its own
// --refresh-lock/--lock-enforcement flags).
type batchedComponentManifestsParams struct {
	components      []string
	componentType   string
	dryRun          bool
	refreshLock     bool
	lockEnforcement string
}

// pullBatchedComponentManifests initializes the CLI config the same way other component-manifest
// resolution call sites do (e.g. pkg/vendoring/resolve.go's DefaultComponentDirResolver) and pulls
// every entry in p.components in a single ExecuteComponentVendorPullBatch call.
func pullBatchedComponentManifests(p *batchedComponentManifestsParams) error {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}
	// lockEnforcement falls back to vendor.lock.enforcement (defaulting to "warn") the same way
	// any other flagless call path does when vendor update --lock-enforcement wasn't passed.
	lockEnforcement := p.lockEnforcement
	if lockEnforcement == "" {
		lockEnforcement = e.DefaultLockEnforcement(&atmosConfig)
	}
	opts := install.InstallOptions{DryRun: p.dryRun, RefreshLock: p.refreshLock, LockEnforcement: lockEnforcement}
	return e.ExecuteComponentVendorPullBatch(&atmosConfig, p.components, p.componentType, opts)
}

// pullUpdatedComponent drives a single "vendor pull --component <component>" call by resetting, on
// the shared cmd, every flag ExecuteVendorPullCmd/parseVendorFlags (internal/exec/vendor.go) reads
// that could otherwise carry stale state across loop iterations or leak in from vendor update's own
// flags of the same name:
//   - "component" is set to the target component.
//   - "everything" is force-reset to false so it never wins over "component" (parseVendorFlags'
//     setDefaultEverythingFlag only defaults it to true when nothing else is set, but a prior
//     iteration - or an earlier design of this function - could otherwise have left it true).
//   - "tags" is cleared via resetUnchangedFlag (not cmd.Flags().Set, see its doc) since
//     validateVendorFlags rejects "component" combined with "tags": a
//     "vendor update --tags foo --pull" run would otherwise fail here even though the top-level
//     update already resolved exactly which components to pull.
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
	if err := resetUnchangedFlag(cmd, "tags"); err != nil {
		return err
	}
	return e.ExecuteVendorPullCmd(cmd, args)
}

// resetUnchangedFlag clears name's value back to "" and marks it Changed=false, rather than merely
// calling cmd.Flags().Set (which unconditionally marks a flag Changed=true, even when set to "").
// This distinction matters if any Changed()-sensitive flag reader is ever added to
// ExecuteVendorPullCommand (internal/exec/vendor.go) in the future - a plain cmd.Flags().Set("")
// would leave that flag spuriously marked Changed after this per-component pull loop, even though
// the user never actually passed it. "tags" has no such Changed() reader today but is reset this
// way defensively, in case one is added later.
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
