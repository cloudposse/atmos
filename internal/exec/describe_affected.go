package exec

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	"github.com/go-git/go-git/v5/plumbing"
	giturl "github.com/kubescape/go-git-url"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/ci"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/matrix"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var ErrRepoPathConflict = errors.New("if the '--repo-path' flag is specified, the '--base', '--ref', '--sha', '--ssh-key' and '--ssh-key-password' flags can't be used")

// logKeyProvider is the structured-log key used for the CI provider name
// in auto-detect log lines. Extracted to satisfy the revive `add-constant`
// linter (the literal appears in four log calls across the CI helpers).
const logKeyProvider = "provider"

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
	AuthManager                 auth.AuthManager // Optional: Auth manager for credential management (from --identity flag).
	HeadSHAOverride             string           // PR head SHA from CI event payload, used for upload correlation with Atmos Pro.
	CIEventType                 string           // CI event type (e.g., "pull_request", "push") for upload validation.
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
		excludeLocked bool,
		authManager auth.AuthManager,
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
		excludeLocked bool,
		authManager auth.AuthManager,
	) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error)
	executeDescribeAffectedWithTargetRefCheckout func(
		atmosConfig *schema.AtmosConfiguration,
		ref string,
		sha string,
		includeSpaceliftAdminStacks bool,
		includeSettings bool,
		stack string,
		processTemplates bool,
		processYamlFunctions bool,
		skip []string,
		excludeLocked bool,
		authManager auth.AuthManager,
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
		executeDescribeAffectedWithTargetRepoPath:    ExecuteDescribeAffectedWithTargetRepoPath,
		executeDescribeAffectedWithTargetRefClone:    ExecuteDescribeAffectedWithTargetRefClone,
		executeDescribeAffectedWithTargetRefCheckout: ExecuteDescribeAffectedWithTargetRefCheckout,
		addDependentsToAffected:                      addDependentsToAffected,
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
	}
	SetDescribeAffectedFlagValueInCliArgs(flags, &result)

	if result.Format != "yaml" && result.Format != "json" && result.Format != "matrix" {
		return DescribeAffectedCmdArgs{}, ErrInvalidFormat
	}
	if result.RepoPath != "" && (result.Base != "" || result.Ref != "" || result.SHA != "" || result.SSHKeyPath != "" || result.SSHKeyPassword != "") {
		return DescribeAffectedCmdArgs{}, errUtils.Build(ErrRepoPathConflict).
			WithHint("Pass only one of: --repo-path OR (--base | --ref | --sha | --ssh-key | --ssh-key-password). --repo-path points at an already-cloned sibling repository to diff against; the others clone or check out a target ref.").
			WithHint("To compare against a specific ref or SHA, use --base without --repo-path. To compare against an already-cloned repo, use --repo-path without --base / --ref / --sha.").
			Err()
	}

	return result, nil
}

// isCIEnabledForDescribeAffected resolves whether CI features (specifically,
// --base auto-detection from the CI provider's environment) should run for
// this invocation of `describe affected`.
//
// Precedence (highest to lowest):
//  1. --ci CLI flag on this invocation (checked via pflag.Flag.Changed).
//  2. ATMOS_CI / CI env vars (checked via os.LookupEnv). ATMOS_CI wins
//     if both are set.
//  3. ci.enabled in atmos.yaml.
//  4. false (default).
//
// Why not viper.IsSet("ci")? The StandardParser.BindFlagsToViper helper
// (pkg/flags/standard.go:455) calls v.SetDefault("ci", false) for the
// flag's default, which flips viper.IsSet("ci") to true for every
// invocation — masking ci.enabled from atmos.yaml. Checking pflag.Changed
// + os.LookupEnv directly is the only way to distinguish explicit
// user overrides from the parser's default, so config fall-through
// works when the user has opted in via atmos.yaml but not via flag/env.
// See TestIsCIEnabledForDescribeAffected_RealBinding for the regression
// this avoids.
func isCIEnabledForDescribeAffected(flags *pflag.FlagSet, describe *DescribeAffectedCmdArgs) bool {
	defer perf.Track(nil, "exec.isCIEnabledForDescribeAffected")()

	// 1. Explicit --ci CLI flag.
	if flags != nil {
		if f := flags.Lookup("ci"); f != nil && f.Changed {
			if val, err := flags.GetBool("ci"); err == nil {
				return val
			}
		}
	}
	// 2. Explicit env var. Order mirrors flags.WithEnvVars("ci", "ATMOS_CI",
	//    "CI"): ATMOS_CI checked first so it wins over CI when both set.
	//
	// When an env var is set but strconv.ParseBool rejects the value
	// (e.g. ATMOS_CI=yes / on / enabled), we log a warning and BREAK out
	// of the env-var loop without falling through to the next env var.
	// Rationale: the user's explicit ATMOS_CI was an override attempt —
	// silently falling through to the ambient CI=true (set by every
	// runner) would be the opposite of their intent. We still fall
	// through to ci.enabled in atmos.yaml (tier 3), which preserves
	// declared intent from committed config while surfacing the typo
	// via the warning.
	for _, name := range []string{"ATMOS_CI", "CI"} {
		val, ok := os.LookupEnv(name)
		if !ok || val == "" {
			continue
		}
		parsed, err := strconv.ParseBool(val)
		if err != nil {
			log.Warn("Ignoring unparseable CI env var; falling through to ci.enabled in atmos.yaml",
				"name", name, "value", val, "error", err)
			break
		}
		return parsed
	}
	// 3. ci.enabled in atmos.yaml (primary fallback for the common case:
	//    user committed `ci.enabled: true` and is running locally without
	//    env overrides).
	if describe.CLIConfig == nil {
		return false
	}
	return describe.CLIConfig.CI.Enabled
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
		"include-dependents":             &describe.IncludeDependents,
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
	// Resolve --base flag: auto-detect ref vs SHA and populate the appropriate field.
	if describe.Base != "" {
		if ci.IsCommitSHA(describe.Base) {
			describe.SHA = describe.Base
		} else {
			describe.Ref = describe.Base
		}
	}

	// Auto-detect CI metadata when CI is enabled and no explicit base is provided.
	//
	// Two cases:
	//   - Normal (no --repo-path): populate Ref/SHA + HeadSHAOverride/CIEventType
	//     from the CI provider.
	//   - --repo-path: skip base detection (--repo-path is mutually exclusive with
	//     --base/--ref/--sha per ErrRepoPathConflict — auto-injecting them from
	//     CI env would break every describe-affected call that passes --repo-path,
	//     e.g. the cloudposse/github-action-atmos-affected-trigger-spacelift
	//     action). When --upload is also set, still populate the upload correlation
	//     metadata (HeadSHAOverride, CIEventType) so Atmos Pro can match the
	//     uploaded affected list to the PR webhook SHA even though this checkout
	//     is at a merge commit.
	if describe.Ref == "" && describe.SHA == "" && isCIEnabledForDescribeAffected(flags, describe) {
		switch {
		case describe.RepoPath == "":
			resolveBaseFromCI(describe)
		case describe.Upload:
			resolveCIUploadMetadataFromCI(describe)
		}
	}

	// When uploading, always include dependents and settings for all affected components.
	if describe.Upload {
		describe.IncludeDependents = true
		describe.IncludeSettings = true
	}
	if describe.Format == "" {
		describe.Format = "json"
	}
}

// resolveBaseFromCI attempts to auto-detect the base commit from the CI provider.
func resolveBaseFromCI(describe *DescribeAffectedCmdArgs) {
	defer perf.Track(nil, "exec.resolveBaseFromCI")()

	p := ci.Detect()
	if p == nil {
		return
	}

	resolution, err := p.ResolveBase()
	if err != nil {
		log.Warn("Failed to auto-detect CI base", logKeyProvider, p.Name(), "error", err)
		return
	}
	if resolution == nil {
		return
	}

	describe.Ref = resolution.Ref
	describe.SHA = resolution.SHA
	describe.HeadSHAOverride = resolution.HeadSHA
	describe.CIEventType = resolution.EventType

	base := resolution.SHA
	if base == "" {
		base = resolution.Ref
	}
	log.Info("Auto-detected CI base",
		logKeyProvider, p.Name(),
		"event", resolution.EventType,
		"base", base,
		"source", resolution.Source)
}

// resolveCIUploadMetadataFromCI populates the upload correlation metadata
// (HeadSHAOverride, CIEventType) from the CI provider WITHOUT populating
// Ref or SHA.
//
// Used in the `--repo-path` + `--upload` + CI code path. Base auto-detection
// is skipped there to avoid ErrRepoPathConflict, but Atmos Pro still needs
// the PR head SHA and event type to correlate the uploaded affected list
// with the PR webhook — the current checkout is typically a GitHub
// pull_request merge commit, whose SHA does not match what the webhook
// indexed.
func resolveCIUploadMetadataFromCI(describe *DescribeAffectedCmdArgs) {
	defer perf.Track(nil, "exec.resolveCIUploadMetadataFromCI")()

	p := ci.Detect()
	if p == nil {
		return
	}

	resolution, err := p.ResolveBase()
	if err != nil {
		log.Warn("Failed to auto-detect CI upload metadata", logKeyProvider, p.Name(), "error", err)
		return
	}
	if resolution == nil {
		return
	}

	describe.HeadSHAOverride = resolution.HeadSHA
	describe.CIEventType = resolution.EventType

	log.Debug("Auto-detected CI upload metadata (base auto-detect skipped due to --repo-path)",
		logKeyProvider, p.Name(),
		"event", resolution.EventType,
		"headSHA", resolution.HeadSHA)
}

// Execute executes `describe affected` command.
func (d *describeAffectedExec) Execute(a *DescribeAffectedCmdArgs) error {
	defer perf.Track(nil, "exec.Execute")()

	var affected []schema.Affected
	var headHead, baseHead *plumbing.Reference
	var repoUrl string
	var err error

	switch {
	case a.RepoPath != "":
		affected, headHead, baseHead, repoUrl, err = d.executeDescribeAffectedWithTargetRepoPath(
			a.CLIConfig,
			a.RepoPath,
			a.IncludeSpaceliftAdminStacks,
			a.IncludeSettings,
			a.Stack,
			a.ProcessTemplates,
			a.ProcessYamlFunctions,
			a.Skip,
			a.ExcludeLocked,
			a.AuthManager,
		)
	case a.CloneTargetRef:
		affected, headHead, baseHead, repoUrl, err = d.executeDescribeAffectedWithTargetRefClone(
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
			a.ExcludeLocked,
			a.AuthManager,
		)
	default:
		affected, headHead, baseHead, repoUrl, err = d.executeDescribeAffectedWithTargetRefCheckout(
			a.CLIConfig,
			a.Ref,
			a.SHA,
			a.IncludeSpaceliftAdminStacks,
			a.IncludeSettings,
			a.Stack,
			a.ProcessTemplates,
			a.ProcessYamlFunctions,
			a.Skip,
			a.ExcludeLocked,
			a.AuthManager,
		)
	}
	if err != nil {
		return err
	}

	// Add dependent components and stacks for each affected component.
	if len(affected) > 0 && a.IncludeDependents {
		err = d.addDependentsToAffected(a.CLIConfig, &affected, a.IncludeSettings, a.ProcessTemplates, a.ProcessYamlFunctions, a.Skip, a.Stack, a.AuthManager)
		if err != nil {
			return err
		}
	}

	// Strip unnecessary fields when uploading to Atmos Pro to reduce payload size
	// and stay within serverless function payload limits.
	if a.Upload {
		affected = StripAffectedForUpload(affected)
	}

	return d.view(a, repoUrl, headHead, baseHead, affected)
}

func (d *describeAffectedExec) view(a *DescribeAffectedCmdArgs, repoUrl string, headHead, baseHead *plumbing.Reference, affected []schema.Affected) error {
	// Handle matrix format specially - it bypasses the normal view flow.
	if a.Format == "matrix" {
		entries := convertAffectedToMatrix(affected)
		return matrix.WriteOutput(entries, a.GithubOutputFile)
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

	// Validate that the CI event is a pull_request event when uploading.
	// Atmos Pro only processes pull_request webhooks, so push events cannot be correlated.
	if args.CIEventType != "" && args.CIEventType != "pull_request" && args.CIEventType != "pull_request_target" {
		return errUtils.Build(
			fmt.Errorf("%w: detected CI event %q, but Atmos Pro only supports pull_request events", errUtils.ErrUploadRequiresPullRequestEvent, args.CIEventType),
		).
			WithHint("Ensure your workflow triggers on pull_request events when using --upload.").
			WithHint("Push events and other event types are not supported for Atmos Pro uploads.").
			WithHint("See https://atmos.tools/integrations/pro for supported CI configurations.").
			Err()
	}

	// Parse the repo URL.
	gitURL, err := giturl.NewGitURL(repoUrl)
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
		RepoName:  gitURL.GetRepoName(),
		RepoOwner: gitURL.GetOwnerName(),
		RepoHost:  gitURL.GetHostName(),
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
