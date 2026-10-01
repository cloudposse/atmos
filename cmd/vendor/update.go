package vendor

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/ci"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	_ "github.com/cloudposse/atmos/pkg/git/providers/azuredevops"
	_ "github.com/cloudposse/atmos/pkg/git/providers/cli"
	_ "github.com/cloudposse/atmos/pkg/git/providers/github"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	// Aliased: this file already has a local "tags" variable (the --tags flag's parsed value).
	pkgtags "github.com/cloudposse/atmos/pkg/tags"
	"github.com/cloudposse/atmos/pkg/ui/batch"
	"github.com/cloudposse/atmos/pkg/vendoring"
	"github.com/cloudposse/atmos/pkg/vendoring/concurrency"
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

		atmosConfig, configErr := cfg.InitCliConfig(flags.BuildConfigAndStacksInfo(cmd, v), false)
		if configErr != nil {
			return configErr
		}
		maxConcurrency, configErr := concurrency.Resolve(cmd.Flags(), &atmosConfig)
		if configErr != nil {
			return configErr
		}
		atmosConfig.Vendor.MaxConcurrency = maxConcurrency

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
		componentType := v.GetString("type")
		tags := splitTags(v.GetString("tags"))
		typeChanged := cmd.Flags().Changed("type")

		stack := v.GetString("stack")
		labels, labelsErr := pkgtags.ParseLabelsFlag(v.GetString("labels"))
		if labelsErr != nil {
			return labelsErr
		}
		if err := validateUpdateSelectorFlags(components, stack, labels); err != nil {
			return err
		}
		components, selectErr := resolveUpdateSelectors(&updateSelectorParams{
			cmd:           cmd,
			viper:         v,
			components:    components,
			componentType: componentType,
			typeChanged:   typeChanged,
			stack:         stack,
			labels:        labels,
		})
		if selectErr != nil {
			return selectErr
		}

		component := ""
		if len(components) == 1 {
			component = components[0]
		}
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
			execWorkdir, ewErr := updater.ResolveExecutionWorkdir(cmd.Context(), v, currentWorkdir)
			if ewErr != nil {
				result.Status, result.Failure = "failed", ewErr.Error()
				return ewErr
			}
			defer execWorkdir.Cleanup()
			workdir = execWorkdir.Workdir

			discovery, dErr := runVendorUpdate(&vendorUpdateParams{ctx: cmd.Context(), config: &atmosConfig, maxConcurrency: maxConcurrency, viper: v, componentType: componentType, tags: tags, typeChanged: typeChanged, components: selected, group: group, check: true})
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

		report, err = runVendorUpdate(&vendorUpdateParams{ctx: cmd.Context(), config: &atmosConfig, maxConcurrency: maxConcurrency, viper: v, componentType: componentType, tags: tags, typeChanged: typeChanged, components: selected, group: group, check: check})

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
			// commit/pr may be partially populated even when pErr != nil (e.g. the pull request
			// itself was created but a later label/assignee/reviewer step failed) -- record
			// whatever succeeded so the user isn't left with no way to find it.
			result.Commit = commit
			result.PullRequest = pr
			if pErr != nil {
				result.Status, result.Failure = "failed", pErr.Error()
				renderPullRequestResult(&result, format)
				return errors.Join(pErr, renderComponentUpdaterJSON(&result, format))
			}
			renderPullRequestResult(&result, format)
		}
		if err := renderComponentUpdaterJSON(&result, format); err != nil {
			result.Status, result.Failure = "failed", err.Error()
			return err
		}
		return nil
	},
}

// updateSelectorParams bundles resolveUpdateSelectors' inputs (Options Pattern, CLAUDE.md: revive's
// argument-limit caps functions at 5 positional parameters, crossed once labels joined
// stack/componentType/typeChanged alongside cmd/v/components).
type updateSelectorParams struct {
	cmd           *cobra.Command
	viper         *viper.Viper
	components    []string
	componentType string
	typeChanged   bool
	stack         string
	labels        map[string]string
}

// resolveUpdateSelectors resolves vendorUpdateCmd's --stack/--labels selector, if given, into a
// concrete component list, then normalizes cmd's own flag state so every downstream reader (the
// --pull delegation to ExecuteVendorPullCmd in particular, which reuses this same cmd/FlagSet and
// was built around --component) sees --stack/--labels' resolution the same way it would see an
// equivalent --component list, without needing to know --stack/--labels exist. Mirrors
// pullUpdatedComponent's own flag-normalization approach below. When neither --stack nor --labels
// was given, p.components is returned unchanged.
func resolveUpdateSelectors(p *updateSelectorParams) ([]string, error) {
	if p.stack == "" && len(p.labels) == 0 {
		return p.components, nil
	}

	// processStacks=true: this branch resolves components via ExecuteDescribeStacksScoped, which
	// requires atmosConfig.StackConfigFilesAbsolutePaths to be populated. info is built via
	// flags.BuildConfigAndStacksInfo (reads --base-path/--config/--config-path/--profile from the
	// already-bound Viper values) rather than an empty schema.ConfigAndStacksInfo{}, so those global
	// flags are honored on this path too instead of silently ignored.
	info := flags.BuildConfigAndStacksInfo(p.cmd, p.viper)
	atmosConfig, cfgErr := cfg.InitCliConfig(info, true)
	if cfgErr != nil {
		return nil, cfgErr
	}
	resolved, resolveErr := resolveVendorStackLabelsSelector(&atmosConfig, p.stack, p.labels, p.componentType, p.typeChanged)
	if resolveErr != nil {
		return nil, resolveErr
	}
	if len(resolved) == 0 {
		// A --stack/--labels selector was explicitly given but matched nothing (e.g. an unknown
		// stack name). Falling through with an empty components would be indistinguishable from "no
		// selector given", silently widening to a repo-wide update instead of reporting no match.
		return nil, errUtils.Build(errUtils.ErrInvalidArgumentError).
			WithExplanation("No components matched the given --stack/--labels selector.").
			Err()
	}
	components := resolved

	if err := resetUnchangedFlag(p.cmd, "stack"); err != nil {
		return nil, err
	}
	if err := resetUnchangedFlag(p.cmd, "labels"); err != nil {
		return nil, err
	}
	if sliceValue, ok := p.cmd.Flags().Lookup("component").Value.(pflag.SliceValue); ok {
		if err := sliceValue.Replace(components); err != nil {
			return nil, err
		}
		p.cmd.Flags().Lookup("component").Changed = len(components) > 0
	}

	return components, nil
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
		Organization: v.GetString("vendor.ci.pull_request.organization"),
		Project:      v.GetString("vendor.ci.pull_request.project"),
		Repository:   v.GetString("vendor.ci.pull_request.repository"),
	}
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
	ctx            context.Context
	config         *schema.AtmosConfiguration
	maxConcurrency int
	onEvent        batch.Observer
	typeChanged    bool
	componentType  string
	tags           []string
	check          bool
	onProgress     vendorProgressFunc
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

	ctx := p.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	return vendoring.UpdateContext(ctx, p.config, &vendoring.UpdateParams{
		MaxConcurrency: p.maxConcurrency, OnEvent: p.onEvent,
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
		flags.WithIntFlag(concurrency.Flag, "", 0, "Maximum concurrent preparations or version checks (edition default: 4; earlier editions: 1)"),
		flags.WithEnvVars(concurrency.Flag, concurrency.Env),
		flags.WithStringSliceFlag("component", "c", []string{}, "Update only these components (repeatable)"),
		flags.WithStringFlag("type", "t", "terraform", componentTypeFlagHelp),
		flags.WithStringFlag("tags", "", "", "Update only components whose vendor.yaml source declares any of these tags (comma-separated, matches any)"),
		flags.WithStringFlag("stack", "s", "", "Update only components belonging to the specified stack"),
		flags.WithStringFlag("labels", "", "", vendorLabelsFlagHelp),
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
// When p.component is empty, reconcile every selected report entry, including
// unchanged versions whose local materialization may be absent or modified.
// Resolve packages in report order, preserving each component's source/mixin
// order, then run one concurrent installation batch with ordered destination writes.
func runVendorPull(cmd *cobra.Command, args []string, report *vendoring.UpdateReport, p vendorPullParams) error {
	originalContext := cmd.Context()
	ctx := originalContext
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()
	cmd.SetContext(ctx)
	defer cmd.SetContext(originalContext)

	if p.component != "" {
		if err := resetUnchangedFlag(cmd, "tags"); err != nil {
			return err
		}
		return e.ExecuteVendorPullCmd(cmd, args)
	}
	info := flags.BuildConfigAndStacksInfo(cmd, viper.GetViper())
	config, err := cfg.InitCliConfig(info, false)
	if err != nil {
		return err
	}
	n, err := concurrency.Resolve(cmd.Flags(), &config)
	if err != nil {
		return err
	}
	enforcement := p.lockEnforcement
	if enforcement == "" {
		enforcement = e.DefaultLockEnforcement(&config)
	}
	opts := install.InstallOptions{Context: cmd.Context(), DryRun: p.dryRun, RefreshLock: p.refreshLock, LockEnforcement: enforcement, MaxConcurrency: n}
	var packages []install.VendorPackage
	collect := opts
	collect.Collect = &packages
	for i := range report.Results {
		result := &report.Results[i]
		if componentManifestBasenames[filepath.Base(result.File)] {
			if err := e.ExecuteComponentVendorPullBatch(&config, []string{result.Component}, result.ComponentType, collect); err != nil {
				return err
			}
			continue
		}
		if err := setPullComponentFlags(cmd, result.Component); err != nil {
			return err
		}
		plan, err := e.PlanVendorPull(cmd, args)
		if err != nil {
			return err
		}
		packages = append(packages, plan.Packages...)
	}
	return e.ExecuteVendorPackages(cmd.Context(), &config, packages, opts)
}

// componentManifestBasenames are the physical file basenames a component.yaml-declared source's
// SourceUpdateResult.File can carry (see ReadAndProcessComponentVendorConfigFile's
// findComponentConfigFile), used by runVendorPull to distinguish it from a
// vendor.yaml-declared source (vendor.yaml itself, or any file it imports).
var componentManifestBasenames = map[string]bool{
	"component.yaml": true,
	"component.yml":  true,
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
	// info carries --base-path/--config/--config-path/--profile into cfg.InitCliConfig below, the
	// same way resolveUpdateSelectors already does for --stack/--labels selector resolution above --
	// an empty schema.ConfigAndStacksInfo{} here silently dropped those global flags on this
	// repo-wide "--pull" batched path.
	info schema.ConfigAndStacksInfo
}

// pullBatchedComponentManifests initializes the CLI config from p.info the same way other
// component-manifest resolution call sites do (e.g. pkg/vendoring/resolve.go's
// DefaultComponentDirResolver), honoring --base-path/--config/--config-path/--profile, and pulls
// every entry in p.components in a single ExecuteComponentVendorPullBatch call.
func pullBatchedComponentManifests(p *batchedComponentManifestsParams) error {
	atmosConfig, err := cfg.InitCliConfig(p.info, false)
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
	if err := setPullComponentFlags(cmd, component); err != nil {
		return err
	}
	return e.ExecuteVendorPullCmd(cmd, args)
}

func setPullComponentFlags(cmd *cobra.Command, component string) error {
	// "component" is a repeatable string slice on vendorUpdateCmd, and pflag's Set *appends* to a
	// slice flag once it has been changed — a plain Set here would accumulate one component per
	// loop iteration. Replace the slice wholesale so each pull targets exactly one component.
	if sliceValue, ok := cmd.Flags().Lookup("component").Value.(pflag.SliceValue); ok {
		if err := sliceValue.Replace([]string{component}); err != nil {
			return err
		}
	} else if err := cmd.Flags().Set("component", component); err != nil {
		return err
	}
	if err := cmd.Flags().Set("everything", "false"); err != nil {
		return err
	}
	if err := resetUnchangedFlag(cmd, "tags"); err != nil {
		return err
	}
	return nil
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
