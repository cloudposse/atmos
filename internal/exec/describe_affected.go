package exec

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/ci"
	cfg "github.com/cloudposse/atmos/pkg/config"
	flagsPkg "github.com/cloudposse/atmos/pkg/flags"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	ghactions "github.com/cloudposse/atmos/pkg/github/actions"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/matrix"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/proexec"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// affectedLabelsSource names both places a describe affected --labels value can come from,
// for error messages that cannot tell which one supplied it.
const affectedLabelsSource = "--labels (or ATMOS_LABELS)"

var ErrRepoPathConflict = errors.New("if the '--repo-path' flag is specified, the '--base', '--ref', '--sha', '--ssh-key' and '--ssh-key-password' flags can't be used")

type DescribeAffectedExecCreator func(atmosConfig *schema.AtmosConfiguration) DescribeAffectedExec

type DescribeAffectedCmdArgs struct {
	CLIConfig                   *schema.AtmosConfiguration
	Base                        string // Unified base commit (ref or SHA). Takes precedence over Ref/SHA.
	CloneTargetRef              bool
	Format                      string
	IncludeDependents           bool
	IncludeSettings             bool
	IncludeSpaceliftAdminStacks bool
	OutputFile                  string
	GithubOutputFile            string // Output file for $GITHUB_OUTPUT format (key=value).
	Ref                         string
	RepoPath                    string
	SHA                         string
	SSHKeyPath                  string
	SSHKeyPassword              string
	Verbose                     bool
	Upload                      bool
	Stack                       string
	Query                       string
	ProcessTemplates            bool
	ProcessYamlFunctions        bool
	Skip                        []string
	ExcludeLocked               bool
	Tags                        []string          // Keep only components whose `metadata.tags` contain any of these (from --tags).
	LabelsRaw                   string            // Raw --labels value (comma-separated key=value pairs).
	Labels                      map[string]string // Parsed --labels: keep only components whose `metadata.labels` contain all of these.
	TagsEnvVar                  string            // Environment variable that supplied Tags (for example ATMOS_TAGS); empty when Tags came from the command line or are unset.
	LabelsEnvVar                string            // Environment variable that supplied Labels; empty when Labels came from the command line or are unset.
	Flatten                     bool              // Lift every dependent into the top-level list as `affected: dependent` (from --flatten); requires IncludeDependents.
	FlattenEnvVar               string            // Environment variable that supplied Flatten; empty when it came from the command line or is unset.
	AuthManager                 auth.AuthManager  // Optional: Auth manager for credential management (from --identity flag).
	AuthDisabled                bool              // True when --identity=false (or alias) explicitly disables authentication; routes stack resolution to ExecuteDescribeStacksWithAuthDisabled.
	HeadSHAOverride             string            // PR head SHA from CI event payload, used for upload correlation with Atmos Pro.
	CIEventType                 string            // CI event type (e.g., "pull_request", "push") for upload validation.
	TargetBranch                string            // PR target branch (e.g., "main") used to auto-fetch when refs are missing locally.
	ErrorMode                   string            // How to handle recoverable errors: "strict" (default), "warn", or "silent".
	Cmd                         *cobra.Command    // The invoking Cobra command, used to derive Flags for the exec-metadata sync capture (proexec.FlagsFromCommand).
}

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type DescribeAffectedExec interface {
	Execute(*DescribeAffectedCmdArgs) error
}

type describeAffectedExec struct {
	atmosConfig                               *schema.AtmosConfiguration
	executeDescribeAffectedWithTargetRepoPath func(
		atmosConfig *schema.AtmosConfiguration,
		targetRefPath string,
		includeSpaceliftAdminStacks bool,
		includeSettings bool,
		stack string,
		processTemplates bool,
		processYamlFunctions bool,
		skip []string,
		filter AffectedFilter,
		authManager auth.AuthManager,
		authDisabled bool,
		errOptions DescribeStacksErrorOptions,
	) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error)
	executeDescribeAffectedWithTargetRefClone func(
		atmosConfig *schema.AtmosConfiguration,
		ref string,
		sha string,
		sshKeyPath string,
		sshKeyPassword string,
		includeSpaceliftAdminStacks bool,
		includeSettings bool,
		stack string,
		processTemplates bool,
		processYamlFunctions bool,
		skip []string,
		filter AffectedFilter,
		authManager auth.AuthManager,
		authDisabled bool,
		errOptions DescribeStacksErrorOptions,
	) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error)
	executeDescribeAffectedWithTargetRefCheckout func(
		atmosConfig *schema.AtmosConfiguration,
		ref string,
		sha string,
		targetBranch string,
		includeSpaceliftAdminStacks bool,
		includeSettings bool,
		stack string,
		processTemplates bool,
		processYamlFunctions bool,
		skip []string,
		filter AffectedFilter,
		authManager auth.AuthManager,
		authDisabled bool,
		errOptions DescribeStacksErrorOptions,
	) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error)
	addDependentsToAffected func(
		atmosConfig *schema.AtmosConfiguration,
		affected *[]schema.Affected,
		includeSettings bool,
		processTemplates bool,
		processYamlFunctions bool,
		skip []string,
		onlyInStack string,
		authManager auth.AuthManager,
		authDisabled bool,
		errOptions DescribeStacksErrorOptions,
	) error
	// addDependentsToAffectedWithFilter is used instead of addDependentsToAffected when the `--tags` /
	// `--labels` selectors are set, because it records each dependent's metadata for the pruning step.
	addDependentsToAffectedWithFilter func(
		atmosConfig *schema.AtmosConfiguration,
		affected *[]schema.Affected,
		opts *dependentsOptions,
	) error
	printOrWriteToFile func(
		atmosConfig *schema.AtmosConfiguration,
		format string,
		file string,
		data any,
	) error
	IsTTYSupportForStdout func() bool
	pageCreator           pager.PageCreator
}

// NewDescribeAffectedExec creates a new `describe affected` executor.
func NewDescribeAffectedExec(
	atmosConfig *schema.AtmosConfiguration,
) DescribeAffectedExec {
	defer perf.Track(atmosConfig, "exec.NewDescribeAffectedExec")()

	return &describeAffectedExec{
		atmosConfig: atmosConfig,
		executeDescribeAffectedWithTargetRepoPath:    ExecuteDescribeAffectedWithTargetRepoPathWithOptions,
		executeDescribeAffectedWithTargetRefClone:    ExecuteDescribeAffectedWithTargetRefCloneWithOptions,
		executeDescribeAffectedWithTargetRefCheckout: ExecuteDescribeAffectedWithTargetRefCheckoutWithOptions,
		addDependentsToAffected:                      addDependentsToAffected,
		addDependentsToAffectedWithFilter:            addDependentsToAffectedWithFilter,
		printOrWriteToFile:                           printOrWriteToFile,
		IsTTYSupportForStdout:                        term.IsTTYSupportForStdout,
		pageCreator:                                  pager.New(),
	}
}

// ParseDescribeAffectedCliArgs parses the command-line arguments of the `atmos describe affected` command.
func ParseDescribeAffectedCliArgs(cmd *cobra.Command, args []string) (DescribeAffectedCmdArgs, error) {
	defer perf.Track(nil, "exec.ParseDescribeAffectedCliArgs")()

	var atmosConfig schema.AtmosConfiguration
	if info, err := ProcessCommandLineArgs("", cmd, args, nil); err != nil {
		return DescribeAffectedCmdArgs{}, err
	} else if atmosConfig, err = cfg.InitCliConfig(info, true); err != nil {
		return DescribeAffectedCmdArgs{}, err
	}
	if err := ValidateStacks(&atmosConfig); err != nil {
		return DescribeAffectedCmdArgs{}, err
	}
	// Process flags
	flags := cmd.Flags()

	result := DescribeAffectedCmdArgs{
		CLIConfig: &atmosConfig,
		Cmd:       cmd,
	}
	SetDescribeAffectedFlagValueInCliArgs(flags, &result)

	// Resolve --error-mode: explicit flag/env value wins, else atmos.yaml's
	// describe.error_mode, else "warn".
	result.ErrorMode = ResolveErrorMode(result.ErrorMode, atmosConfig.Describe.ErrorMode)

	if err := validateDescribeAffectedArgs(&result); err != nil {
		return DescribeAffectedCmdArgs{}, err
	}

	return result, nil
}

// validateDescribeAffectedArgs checks the parsed `describe affected` arguments for values and
// combinations that cannot be honored, and normalizes the --tags / --labels / --flatten values.
func validateDescribeAffectedArgs(a *DescribeAffectedCmdArgs) error {
	if err := validateDescribeAffectedBasics(a); err != nil {
		return err
	}
	if err := resolveAffectedSelectors(a); err != nil {
		return err
	}
	return resolveAffectedFlatten(a)
}

// validateDescribeAffectedBasics checks the output format, the repo-path combination, and the error mode.
func validateDescribeAffectedBasics(a *DescribeAffectedCmdArgs) error {
	switch a.Format {
	case "yaml", "json", "matrix":
	default:
		return ErrInvalidFormat
	}
	if a.RepoPath != "" && hasGitComparisonFlags(a) {
		return ErrRepoPathConflict
	}
	switch a.ErrorMode {
	case "strict", "warn", "silent":
	default:
		return fmt.Errorf("%w: %q", ErrInvalidErrorMode, a.ErrorMode)
	}
	return nil
}

// hasGitComparisonFlags reports whether any flag that selects a Git comparison target was set,
// which cannot be combined with --repo-path.
func hasGitComparisonFlags(a *DescribeAffectedCmdArgs) bool {
	return a.Base != "" || a.Ref != "" || a.SHA != "" || a.SSHKeyPath != "" || a.SSHKeyPassword != ""
}

// resolveAffectedSelectors normalizes the --tags and --labels selectors and rejects combinations
// that cannot be honored. The Atmos Pro upload is intentionally unfiltered, so a selector alongside
// --upload would filter the printed output while silently uploading everything.
//
// A selector that came from an environment variable (ATMOS_TAGS / ATMOS_LABELS) is ambient rather
// than a request for this run, so with --upload it is dropped with a warning; only a selector typed
// on the command line is an error.
func resolveAffectedSelectors(a *DescribeAffectedCmdArgs) error {
	a.Tags = tags.ParseTagsFlag(strings.Join(a.Tags, ","))

	labels, err := tags.ParseLabelsFlagFrom(a.LabelsRaw, affectedLabelsSource)
	if err != nil {
		return err
	}
	a.Labels = labels

	if !a.Upload {
		return nil
	}

	if len(a.Tags) > 0 && a.TagsEnvVar != "" {
		ui.Warningf("Ignoring %s: --upload always uploads the unfiltered affected set", a.TagsEnvVar)
		a.Tags = nil
	}
	if len(a.Labels) > 0 && a.LabelsEnvVar != "" {
		ui.Warningf("Ignoring %s: --upload always uploads the unfiltered affected set", a.LabelsEnvVar)
		a.Labels = nil
		a.LabelsRaw = ""
	}

	if len(a.Tags) > 0 || len(a.Labels) > 0 {
		return errUtils.Build(fmt.Errorf("%w: --tags/--labels is not supported with --upload (the upload is always unfiltered)", errUtils.ErrInvalidFlag)).
			WithHint("Run `atmos describe affected --upload` without selectors, and apply --tags/--labels in a separate `describe affected` step for the matrix.").
			WithHint("To override selectors supplied through ATMOS_TAGS/ATMOS_LABELS for this run, pass --tags= --labels=.").
			Err()
	}
	return nil
}

// resolveAffectedFlatten rejects --flatten combinations that cannot be honored. Flattening lifts
// dependents into the top-level list, so it needs --include-dependents, and the Atmos Pro upload is
// always the unflattened affected set. Upload is checked first because it forces IncludeDependents on.
//
// A flatten request that came from ATMOS_DESCRIBE_AFFECTED_FLATTEN is ambient rather than a request
// for this run, so it is dropped with a warning instead of failing the command.
func resolveAffectedFlatten(a *DescribeAffectedCmdArgs) error {
	if !a.Flatten {
		return nil
	}

	switch {
	case a.Upload && a.FlattenEnvVar != "":
		ui.Warningf("Ignoring %s: --upload always uploads the unflattened affected set", a.FlattenEnvVar)
		a.Flatten = false
	case a.Upload:
		return errUtils.Build(fmt.Errorf("%w: --flatten is not supported with --upload (the upload is always the unflattened affected set)", errUtils.ErrInvalidFlag)).
			WithHint("Run `atmos describe affected --upload` without --flatten, and use --flatten in a separate `describe affected` step for the matrix.").
			Err()
	case !a.IncludeDependents && a.FlattenEnvVar != "":
		ui.Warningf("Ignoring %s: --flatten requires --include-dependents", a.FlattenEnvVar)
		a.Flatten = false
	case !a.IncludeDependents:
		return errUtils.Build(fmt.Errorf("%w: --flatten requires --include-dependents", errUtils.ErrInvalidFlag)).
			WithHint("Add --include-dependents so there are dependents to lift into the top-level list.").
			Err()
	}
	return nil
}

// affectedFilter builds the component filter applied while computing the affected set.
func (a *DescribeAffectedCmdArgs) affectedFilter() AffectedFilter {
	return AffectedFilter{ExcludeLocked: a.ExcludeLocked, Tags: a.Tags, Labels: a.Labels}
}

// findAffectedFilter builds the filter handed to the strategy that computes the affected set. With
// `--include-dependents` and `--tags` / `--labels`, the selectors are deferred: a component that fails
// them can still have matching dependents, which are only known once the dependents are resolved, so
// the selectors are applied afterward by applySelectorsToAffectedForest.
func (a *DescribeAffectedCmdArgs) findAffectedFilter() AffectedFilter {
	f := a.affectedFilter()
	f.DeferSelectors = a.IncludeDependents && f.hasSelectors()
	return f
}

// SetDescribeAffectedFlagValueInCliArgs sets the flag values in CLI arguments.
func SetDescribeAffectedFlagValueInCliArgs(flags *pflag.FlagSet, describe *DescribeAffectedCmdArgs) {
	defer perf.Track(nil, "exec.SetDescribeAffectedFlagValueInCliArgs")()

	flagsKeyValue := map[string]any{
		"base":                           &describe.Base,
		"ref":                            &describe.Ref,
		"sha":                            &describe.SHA,
		"repo-path":                      &describe.RepoPath,
		"ssh-key":                        &describe.SSHKeyPath,
		"ssh-key-password":               &describe.SSHKeyPassword,
		"include-spacelift-admin-stacks": &describe.IncludeSpaceliftAdminStacks,
		"include-settings":               &describe.IncludeSettings,
		"upload":                         &describe.Upload,
		"clone-target-ref":               &describe.CloneTargetRef,
		"process-templates":              &describe.ProcessTemplates,
		"process-functions":              &describe.ProcessYamlFunctions,
		"skip":                           &describe.Skip,
		"pager":                          &describe.CLIConfig.Settings.Terminal.Pager,
		"stack":                          &describe.Stack,
		"format":                         &describe.Format,
		"file":                           &describe.OutputFile,
		"output-file":                    &describe.GithubOutputFile,
		"query":                          &describe.Query,
		"verbose":                        &describe.Verbose,
		"exclude-locked":                 &describe.ExcludeLocked,
		"tags":                           &describe.Tags,
		"labels":                         &describe.LabelsRaw,
		"flatten":                        &describe.Flatten,
		"error-mode":                     &describe.ErrorMode,
	}

	// By default, process templates and YAML functions
	describe.ProcessTemplates = true
	describe.ProcessYamlFunctions = true

	var err error
	for k := range flagsKeyValue {
		if !flags.Changed(k) {
			continue
		}
		switch v := flagsKeyValue[k].(type) {
		case *string:
			*v, err = flags.GetString(k)
		case *bool:
			*v, err = flags.GetBool(k)
		case *[]string:
			*v, err = flags.GetStringSlice(k)
		default:
			er := fmt.Errorf("unsupported type %T for flag %s", v, k)
			errUtils.CheckErrorPrintAndExit(er, "", "")
		}
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}
	// --include-dependents is a plain bool on `atmos describe affected` but a
	// depth-carrying string flag on the terraform commands (bare = unlimited,
	// --include-dependents=N bounds the expansion), so it cannot go through the
	// typed map above — read it according to the flag type actually registered.
	if flags.Changed(flagsPkg.FlagIncludeDependents) {
		describe.IncludeDependents, err = includeDependentsFlagValue(flags)
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}

	// Resolve --base flag: auto-detect ref vs SHA and populate the appropriate field.
	if describe.Base != "" {
		if ci.IsCommitSHA(describe.Base) {
			describe.SHA = describe.Base
		} else {
			describe.Ref = describe.Base
		}
	}

	// Auto-detect base from CI environment when ci.enabled is true and no explicit base provided.
	if describe.Ref == "" && describe.SHA == "" && describe.CLIConfig != nil && describe.CLIConfig.CI.Enabled {
		resolveBaseFromCI(describe)
	}

	// When uploading, always include dependents and settings for all affected components.
	if describe.Upload {
		describe.IncludeDependents = true
		describe.IncludeSettings = true
	}
	if describe.Format == "" {
		describe.Format = "json"
	}

	// Record which values came from environment variables rather than the command line, so the
	// validation can warn about an ambient value instead of failing on it. The lookups are nil-safe
	// because not every command that reuses this reader registers these flags.
	describe.TagsEnvVar, _ = flagsPkg.FlagValueFromEnv(flags.Lookup("tags"))
	describe.LabelsEnvVar, _ = flagsPkg.FlagValueFromEnv(flags.Lookup("labels"))
	describe.FlattenEnvVar, _ = flagsPkg.FlagValueFromEnv(flags.Lookup("flatten"))
}

// includeDependentsFlagValue reads the include-dependents flag as a boolean
// regardless of how the owning command registered it: `atmos describe affected`
// uses a bool flag, while the terraform commands use a depth-carrying string
// flag where any enabled depth (unlimited or bounded) means "include them".
func includeDependentsFlagValue(flags *pflag.FlagSet) (bool, error) {
	flag := flags.Lookup(flagsPkg.FlagIncludeDependents)
	if flag == nil {
		return false, nil
	}
	if flag.Value.Type() == "bool" {
		return flags.GetBool(flagsPkg.FlagIncludeDependents)
	}
	value, err := flags.GetString(flagsPkg.FlagIncludeDependents)
	if err != nil {
		return false, err
	}
	depth, err := flagsPkg.ParseClosureDepth(flagsPkg.FlagIncludeDependents, value)
	if err != nil {
		return false, err
	}
	return depth != 0, nil
}

// resolveBaseFromCI attempts to auto-detect the base commit from the CI provider.
//
// This function is only invoked when `ci.enabled: true` in atmos.yaml and no
// explicit base flag was provided (see the call site above). Because the user
// has opted into CI auto-detect, it is appropriate to mutate the runner's git
// config (via `EnsureGitSafeDirectory`) so that downstream git commands —
// `merge-base`, the targeted `git fetch` for shallow clones, and the
// checkout-classified merged-PR lookups (merge-commit parents, payload-anchor
// fetches) — do not fail with "dubious ownership in repository" inside GitHub
// Actions container jobs. The helper is a no-op outside GitHub Actions, so it
// does nothing when running locally.
func resolveBaseFromCI(describe *DescribeAffectedCmdArgs) {
	defer perf.Track(nil, "exec.resolveBaseFromCI")()

	p := ci.Detect()
	if p == nil {
		return
	}

	// Trust the GitHub Actions workspace before any git operation the
	// provider is about to run. Gated by `ci.enabled` via the call site.
	if err := atmosgit.EnsureGitSafeDirectory(); err != nil {
		// Non-fatal: subsequent git commands will surface a clearer error
		// if this actually matters. We log so the cause is visible.
		log.Warn("Failed to configure git safe.directory for GitHub Actions workspace", "error", err)
	}

	resolution, err := p.ResolveBase()
	if err != nil {
		log.Warn("Failed to auto-detect CI base", "provider", p.Name(), "error", err)
		return
	}
	if resolution == nil {
		return
	}

	describe.Ref = resolution.Ref
	describe.SHA = resolution.SHA
	describe.HeadSHAOverride = resolution.HeadSHA
	describe.CIEventType = resolution.EventType
	describe.TargetBranch = resolution.TargetBranch

	base := resolution.SHA
	if base == "" {
		base = resolution.Ref
	}
	logArgs := []any{
		"provider", p.Name(),
		"event", resolution.EventType,
		"base", base,
		"source", resolution.Source,
	}
	// The checkout classification tells support which strategy family was
	// valid for this run — wrong-base incidents are diagnosable from this
	// one line instead of run-log archaeology.
	if resolution.Checkout != "" {
		logArgs = append(logArgs, "checkout", resolution.Checkout)
	}
	log.Info("Auto-detected CI base", logArgs...)
}

// Execute executes `describe affected` command. It reports an execution
// record to Atmos Pro synchronously (warn-and-continue on failure) before
// returning, per the synchronous allowlist (terraform plan/apply, describe
// affected). The record's structured Data carries the same per-stack data
// already reported to the existing POST /api/v1/affected-stacks upload, as
// {"version": 1, "stacks": [...]} — unconditionally, not gated on --upload,
// since the affected list is already computed for every invocation
// (FR-006b, research.md Decision 22).
func (d *describeAffectedExec) Execute(a *DescribeAffectedCmdArgs) error {
	affected, err := d.executeInner(a)

	// describe affected has no numeric "exit code" the way a shell command
	// does; 0/1 mirrors the success/failure convention used elsewhere in this
	// feature (e.g. internal/exec/terraform.go's captureExecMetadataSync).
	exitCode := 0
	if err != nil {
		exitCode = 1
	}
	// Flags MUST be sourced from the invoking Cobra command's own record of
	// explicitly-set flags, matching internal/exec/terraform.go's
	// captureExecMetadataSync (research.md Decision 14).
	flags := proexec.FlagsFromCommand(a.Cmd)

	in := &proexec.ExecRecordInput{
		Command:  "describe affected",
		Flags:    flags,
		ExitCode: exitCode,
		Data:     proexec.VersionedData(1, "stacks", affected),
	}
	if syncErr := proexec.CaptureSync(a.CLIConfig, in); syncErr != nil {
		log.Debug("Exec-metadata sync capture returned an error.", "error", syncErr)
	}

	return err
}

// affectedResolution bundles the raw result of a target-resolution strategy
// (affected stacks plus the HEAD/BASE references and repo URL used to
// compute them) into a single value so resolveAffectedStacks stays under the
// linter's return-count limit.
type affectedResolution struct {
	Affected []schema.Affected
	HeadHead *plumbing.Reference
	BaseHead *plumbing.Reference
	RepoURL  string
}

// resolveAffectedStacks dispatches to the target-resolution strategy selected
// by a's RepoPath/CloneTargetRef fields (explicit repo path, cloned target
// ref, or checked-out target ref, in that priority order) and returns its
// raw result. Split out of executeInner to keep that function's line count
// under the linter's limit.
func (d *describeAffectedExec) resolveAffectedStacks(a *DescribeAffectedCmdArgs, errOptions DescribeStacksErrorOptions) (affectedResolution, error) {
	switch {
	case a.RepoPath != "":
		return toAffectedResolution(d.executeDescribeAffectedWithTargetRepoPath(
			a.CLIConfig,
			a.RepoPath,
			a.IncludeSpaceliftAdminStacks,
			a.IncludeSettings,
			a.Stack,
			a.ProcessTemplates,
			a.ProcessYamlFunctions,
			a.Skip,
			a.findAffectedFilter(),
			a.AuthManager,
			a.AuthDisabled,
			errOptions,
		))
	case a.CloneTargetRef:
		return toAffectedResolution(d.executeDescribeAffectedWithTargetRefClone(
			a.CLIConfig,
			a.Ref,
			a.SHA,
			a.SSHKeyPath,
			a.SSHKeyPassword,
			a.IncludeSpaceliftAdminStacks,
			a.IncludeSettings,
			a.Stack,
			a.ProcessTemplates,
			a.ProcessYamlFunctions,
			a.Skip,
			a.findAffectedFilter(),
			a.AuthManager,
			a.AuthDisabled,
			errOptions,
		))
	default:
		return toAffectedResolution(d.executeDescribeAffectedWithTargetRefCheckout(
			a.CLIConfig,
			a.Ref,
			a.SHA,
			a.TargetBranch,
			a.IncludeSpaceliftAdminStacks,
			a.IncludeSettings,
			a.Stack,
			a.ProcessTemplates,
			a.ProcessYamlFunctions,
			a.Skip,
			a.findAffectedFilter(),
			a.AuthManager,
			a.AuthDisabled,
			errOptions,
		))
	}
}

// toAffectedResolution adapts a target-resolution strategy's raw 5-value
// return into an affectedResolution, so resolveAffectedStacks's per-case
// return statements stay within the linter's return-count limit.
func toAffectedResolution(affected []schema.Affected, headHead, baseHead *plumbing.Reference, repoURL string, err error) (affectedResolution, error) {
	return affectedResolution{Affected: affected, HeadHead: headHead, BaseHead: baseHead, RepoURL: repoURL}, err
}

// executeInner contains the original `describe affected` execution logic. It
// returns the computed affected list alongside its error so Execute can
// attach it to the execution record's structured Data (FR-006b, research.md
// Decision 22) — the same slice already used for rendering/upload below, no
// second computation.
func (d *describeAffectedExec) executeInner(a *DescribeAffectedCmdArgs) ([]schema.Affected, error) {
	defer perf.Track(nil, "exec.Execute")()

	// Built once and reused across every describe-stacks call this command makes (HEAD,
	// BASE, and any dependents resolution) so the end-of-command summary reports one
	// combined count instead of one per call site.
	errOptions, collector := ErrorOptionsFromMode(a.ErrorMode)

	resolution, err := d.resolveAffectedStacks(a, errOptions)
	if err != nil {
		return nil, err
	}
	affected := resolution.Affected

	// Add dependent components and stacks for each affected component, then apply the
	// `--tags` / `--labels` selectors to them and, with `--flatten`, lift them into the top-level list.
	if len(affected) > 0 && a.IncludeDependents {
		err = finalizeAffectedDependents(a.CLIConfig, &affected, &AffectedDependentsOptions{
			IncludeSettings:      a.IncludeSettings,
			ProcessTemplates:     a.ProcessTemplates,
			ProcessYamlFunctions: a.ProcessYamlFunctions,
			Skip:                 a.Skip,
			OnlyInStack:          a.Stack,
			AuthManager:          a.AuthManager,
			AuthDisabled:         a.AuthDisabled,
			ErrOptions:           errOptions,
			Filter:               a.affectedFilter(),
			Flatten:              a.Flatten,
		}, dependentsResolvers{plain: d.addDependentsToAffected, withFilter: d.addDependentsToAffectedWithFilter})
		if err != nil {
			return nil, err
		}
	}

	// Strip unnecessary fields when uploading to Atmos Pro to reduce payload size
	// and stay within serverless function payload limits.
	if a.Upload {
		affected = StripAffectedForUpload(affected)
	}

	if err := d.view(a, resolution.RepoURL, resolution.HeadHead, resolution.BaseHead, affected); err != nil {
		return nil, err
	}

	PrintErrorModeSummary(a.ErrorMode, collector)
	return affected, nil
}

// AffectedDependentsOptions configures FinalizeAffectedDependents.
type AffectedDependentsOptions struct {
	IncludeSettings      bool
	ProcessTemplates     bool
	ProcessYamlFunctions bool
	Skip                 []string
	OnlyInStack          string
	AuthManager          auth.AuthManager
	AuthDisabled         bool
	ErrOptions           DescribeStacksErrorOptions
	// Filter holds the `--tags` / `--labels` selectors. When it has selectors they are applied to the
	// affected list and its dependents after the dependents are resolved, so the affected list must have
	// been computed with the selectors deferred (AffectedFilter.DeferSelectors).
	Filter AffectedFilter
	// Flatten lifts every dependent into the top-level list as an entry with the reason "dependent".
	Flatten bool
}

// dependentsResolvers are the two ways to resolve dependents; they are injectable so tests can stub the
// stack resolution behind them.
type dependentsResolvers struct {
	plain      func(atmosConfig *schema.AtmosConfiguration, affected *[]schema.Affected, includeSettings, processTemplates, processYamlFunctions bool, skip []string, onlyInStack string, authManager auth.AuthManager, authDisabled bool, errOptions DescribeStacksErrorOptions) error
	withFilter func(atmosConfig *schema.AtmosConfiguration, affected *[]schema.Affected, opts *dependentsOptions) error
}

// FinalizeAffectedDependents resolves the (nested) dependents of every affected component and shapes the
// result: with `--tags` / `--labels` selectors it prunes the dependents by them, keeps only the matching
// top-level components, and promotes the matching dependents of a dropped component; with Flatten it then
// lifts all remaining dependents into the top-level list. It is the single path shared by
// `describe affected --include-dependents` and `list affected --include-dependents`.
func FinalizeAffectedDependents(atmosConfig *schema.AtmosConfiguration, affected *[]schema.Affected, opts *AffectedDependentsOptions) error {
	defer perf.Track(atmosConfig, "exec.FinalizeAffectedDependents")()

	return finalizeAffectedDependents(atmosConfig, affected, opts, dependentsResolvers{
		plain:      addDependentsToAffected,
		withFilter: addDependentsToAffectedWithFilter,
	})
}

// finalizeAffectedDependents is FinalizeAffectedDependents with injectable dependents resolution.
func finalizeAffectedDependents(atmosConfig *schema.AtmosConfiguration, affected *[]schema.Affected, opts *AffectedDependentsOptions, resolve dependentsResolvers) error {
	if len(*affected) == 0 {
		return nil
	}

	// With `--tags` / `--labels`, dependents are resolved together with their metadata so the nested lists and
	// the top-level list can be pruned by the same selectors; without selectors the original path is used unchanged.
	if opts.Filter.hasSelectors() {
		depOpts := &dependentsOptions{
			IncludeSettings:      opts.IncludeSettings,
			ProcessTemplates:     opts.ProcessTemplates,
			ProcessYamlFunctions: opts.ProcessYamlFunctions,
			Skip:                 opts.Skip,
			OnlyInStack:          opts.OnlyInStack,
			AuthManager:          opts.AuthManager,
			AuthDisabled:         opts.AuthDisabled,
			ErrOptions:           opts.ErrOptions,
			Filter:               opts.Filter,
		}
		if err := resolve.withFilter(atmosConfig, affected, depOpts); err != nil {
			return err
		}
		*affected = applySelectorsToAffectedForest(*affected, opts.Filter, depOpts.stacks)
	} else if err := resolve.plain(atmosConfig, affected, opts.IncludeSettings, opts.ProcessTemplates, opts.ProcessYamlFunctions, opts.Skip, opts.OnlyInStack, opts.AuthManager, opts.AuthDisabled, opts.ErrOptions); err != nil {
		return err
	}

	if opts.Flatten {
		*affected = flattenAffectedDependents(*affected)
	}
	return nil
}

func (d *describeAffectedExec) view(a *DescribeAffectedCmdArgs, repoUrl string, headHead, baseHead *plumbing.Reference, affected []schema.Affected) error {
	// Handle matrix format specially - it bypasses the normal view flow.
	if a.Format == "matrix" {
		entries := convertAffectedToMatrix(affected)

		// Resolve output file: explicit flag > CI auto-detect > stdout.
		outputFile := a.GithubOutputFile
		if outputFile == "" && d.atmosConfig.CI.Enabled {
			outputFile = ghactions.GetOutputPath()
		}
		return matrix.WriteOutput(entries, outputFile)
	}

	// Reject --output-file for non-matrix formats — it would be silently ignored.
	if a.GithubOutputFile != "" {
		return fmt.Errorf("%w: --output-file is only supported with --format=matrix", errUtils.ErrInvalidFlag)
	}

	if a.Query == "" {
		if err := d.uploadableQuery(a, repoUrl, headHead, baseHead, affected); err != nil {
			return err
		}
	} else {
		res, err := u.EvaluateYqExpression(d.atmosConfig, affected, a.Query)
		if err != nil {
			return err
		}

		err = viewWithScroll(&viewWithScrollProps{d.pageCreator, term.IsTTYSupportForStdout, d.printOrWriteToFile, d.atmosConfig, "Affected components and stacks", a.Format, a.OutputFile, res})
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *describeAffectedExec) uploadableQuery(args *DescribeAffectedCmdArgs, repoUrl string, headHead, baseHead *plumbing.Reference, affected []schema.Affected) error {
	log.Debug("Affected components and stacks:")

	// When uploading, suppress the large JSON dump unless verbose mode or file output is requested.
	if !args.Upload || args.Verbose || args.OutputFile != "" {
		err := viewWithScroll(&viewWithScrollProps{d.pageCreator, d.IsTTYSupportForStdout, d.printOrWriteToFile, d.atmosConfig, "Affected components and stacks", args.Format, args.OutputFile, affected})
		if err != nil {
			return err
		}
	}

	if !args.Upload {
		return nil
	}

	// Validate that the CI event is one Atmos Pro can correlate when uploading.
	// Supported events: pull_request, pull_request_target, and merge_group (GitHub merge queue).
	// Push events and other ad-hoc triggers cannot be correlated to a check run.
	if args.CIEventType != "" &&
		args.CIEventType != "pull_request" &&
		args.CIEventType != "pull_request_target" &&
		args.CIEventType != "merge_group" {
		return errUtils.Build(
			fmt.Errorf("%w: detected CI event %q, but Atmos Pro only supports pull_request, pull_request_target, and merge_group events", errUtils.ErrUploadRequiresSupportedEvent, args.CIEventType),
		).
			WithHint("Trigger your workflow on pull_request, pull_request_target, or merge_group events when using --upload.").
			WithHint("Push events and other ad-hoc triggers cannot be correlated to an Atmos Pro check run.").
			WithHint("See https://atmos.tools/cli/configuration/settings/pro for supported CI configurations.").
			Err()
	}

	repoURLParts, err := atmosgit.ParseRepoURL(repoUrl)
	if err != nil {
		return err
	}

	log.Debug("Creating API client")
	apiClient, err := pro.NewAtmosProAPIClientFromEnv(d.atmosConfig)
	if err != nil {
		return errUtils.Build(
			fmt.Errorf("%w: %w", errUtils.ErrFailedToCreateAPIClient, err),
		).
			WithHint("Ensure your GitHub Actions workflow has `id-token: write` permission for OIDC authentication.").
			WithHint("Verify that `ATMOS_PRO_WORKSPACE_ID` is set to the correct workspace ID for this repository.").
			WithHint("See https://atmos.tools/pro for authentication setup.").
			Err()
	}

	// Use the PR head SHA from the CI event payload when available.
	// This ensures the upload SHA matches what Atmos Pro indexed from the webhook,
	// regardless of which commit the workflow has checked out (e.g., merge commit vs PR head).
	headSHA := headHead.Hash().String()
	if args.HeadSHAOverride != "" {
		headSHA = args.HeadSHAOverride
		log.Debug("Using PR head SHA for upload correlation", "headSHA", headSHA, "localHEAD", headHead.Hash().String())
	}

	req := dtos.UploadAffectedStacksRequest{
		HeadSHA:   headSHA,
		BaseSHA:   baseHead.Hash().String(),
		RepoURL:   repoUrl,
		RepoName:  repoURLParts.Name,
		RepoOwner: repoURLParts.Owner,
		RepoHost:  repoURLParts.Host,
		Stacks:    affected,
	}

	log.Debug("Preparing upload affected stacks request", "req", req)

	if uploadErr := apiClient.UploadAffectedStacks(&req); uploadErr != nil {
		ui.Error("Failed to upload affected stacks to Atmos Pro")
		return uploadErr
	}

	ui.Successf("Uploaded %d affected component(s) to Atmos Pro", len(affected))

	return nil
}

type viewWithScrollProps struct {
	pageCreator           pager.PageCreator
	isTTYSupportForStdout func() bool
	printOrWriteToFile    func(atmosConfig *schema.AtmosConfiguration, format string, file string, data any) error
	atmosConfig           *schema.AtmosConfiguration
	displayName           string
	format                string
	file                  string
	res                   any
}

func viewWithScroll(v *viewWithScrollProps) error {
	if v.atmosConfig.Settings.Terminal.IsPagerEnabled() && v.file == "" {
		err := viewConfig(&viewConfigProps{v.pageCreator, v.isTTYSupportForStdout, v.atmosConfig, v.displayName, v.format, v.res})
		switch err.(type) {
		case DescribeConfigFormatError:
			return err
		case nil:
			return nil
		default:
			log.Debug("Failed to use pager")
		}
	}

	err := v.printOrWriteToFile(v.atmosConfig, v.format, v.file, v.res)
	if err != nil {
		return err
	}
	return nil
}

type viewConfigProps struct {
	pageCreator           pager.PageCreator
	isTTYSupportForStdout func() bool
	atmosConfig           *schema.AtmosConfiguration
	displayName           string
	format                string
	data                  any
}

func viewConfig(v *viewConfigProps) error {
	if !v.isTTYSupportForStdout() {
		return ErrTTYNotSupported
	}
	var content string
	var err error
	switch v.format {
	case "yaml":
		content, err = u.GetHighlightedYAML(v.atmosConfig, v.data)
		if err != nil {
			return err
		}
	case "json":
		content, err = u.GetHighlightedJSON(v.atmosConfig, v.data)
		if err != nil {
			return err
		}
	default:
		return DescribeConfigFormatError{
			v.format,
		}
	}
	if err := v.pageCreator.Run(v.displayName, content); err != nil {
		return err
	}
	return nil
}

// convertAffectedToMatrix converts the affected list to matrix entries.
func convertAffectedToMatrix(affected []schema.Affected) []matrix.Entry {
	entries := make([]matrix.Entry, 0, len(affected))
	for i := range affected {
		a := &affected[i]
		entries = append(entries, matrix.Entry{
			Stack:         a.Stack,
			Component:     a.Component,
			ComponentPath: a.ComponentPath,
			ComponentType: a.ComponentType,
		})
	}
	return entries
}
