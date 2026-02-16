package ci

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	cipkg "github.com/cloudposse/atmos/pkg/ci"
	_ "github.com/cloudposse/atmos/pkg/ci/providers/github" // Register GitHub provider.
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// buildConfigAndStacksInfo creates ConfigAndStacksInfo from global flags.
func buildConfigAndStacksInfo(globalFlags *global.Flags) schema.ConfigAndStacksInfo {
	if globalFlags == nil {
		return schema.ConfigAndStacksInfo{}
	}
	return schema.ConfigAndStacksInfo{
		AtmosBasePath:           globalFlags.BasePath,
		AtmosConfigFilesFromArg: globalFlags.Config,
		AtmosConfigDirsFromArg:  globalFlags.ConfigPath,
		ProfilesFromArg:         globalFlags.Profile,
	}
}

const (
	// ShortSHALength is the standard length for displaying abbreviated git SHAs.
	shortSHALength = 7
)

// statusCmd represents the ci status command.
var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show CI status for current branch",
	Long: `Display CI status for the current branch, similar to 'gh pr status'.

Shows status checks, pull request information, and related PRs.`,
	Args: cobra.NoArgs,
	RunE: runStatus,
}

func runStatus(cmd *cobra.Command, _ []string) error {
	ctx := cmd.Context()

	// Parse global flags before loading config to respect --config-path, --base-path, etc.
	globalFlags := flags.ParseGlobalFlags(cmd, viper.GetViper())

	// Load Atmos configuration (optional - CI status can work without it).
	atmosConfig, configErr := cfg.InitCliConfig(buildConfigAndStacksInfo(&globalFlags), false)
	configLoaded := configErr == nil
	if configErr != nil {
		log.Debug("Failed to load atmos config, continuing without CI enabled check", "error", configErr)
	}

	// Check if CI is enabled in config (only relevant when actually in CI).
	if configLoaded && cipkg.IsCI() && !atmosConfig.CI.Enabled {
		return errUtils.Build(errUtils.ErrCIDisabled).
			WithExplanation("CI integration is disabled in atmos.yaml").
			WithHint("Set ci.enabled: true in atmos.yaml to enable CI features").
			Err()
	}

	// Get CI provider.
	provider, err := cipkg.DetectOrError()
	if err != nil {
		// If not in CI, try to get a provider from the registry.
		provider, err = getDefaultProvider()
		if err != nil {
			return errUtils.Build(errUtils.ErrCIProviderNotDetected).
				WithExplanation("Not running in a CI environment and no CI provider configured").
				WithHint("Ensure you have a CI provider token set (e.g., GITHUB_TOKEN for GitHub Actions)").
				Err()
		}
	}

	// Get repository info using provider interface.
	repoCtx, err := getRepoContext(provider)
	if err != nil {
		return err
	}

	// Fetch status.
	status, err := provider.GetStatus(ctx, cipkg.StatusOptions{
		Owner:                 repoCtx.Owner,
		Repo:                  repoCtx.Repo,
		Branch:                repoCtx.Branch,
		SHA:                   repoCtx.SHA,
		IncludeUserPRs:        true,
		IncludeReviewRequests: true,
	})
	if err != nil {
		return errUtils.Build(errUtils.ErrCIStatusFetchFailed).
			WithCause(err).
			Err()
	}

	// Render output.
	renderStatus(status)
	return nil
}

// getDefaultProvider returns a provider from the registry.
// This is used when not running in CI but a provider is still available.
func getDefaultProvider() (cipkg.Provider, error) {
	// Try to get any registered provider from the registry.
	names := cipkg.List()
	for _, name := range names {
		p, err := cipkg.Get(name)
		if err == nil {
			return p, nil
		}
	}
	return nil, errUtils.ErrCIProviderNotFound
}

// repoContext holds repository context information.
type repoContext struct {
	Owner  string
	Repo   string
	Branch string
	SHA    string
}

// getRepoContext extracts repository context using the provider interface with git fallback.
func getRepoContext(provider cipkg.Provider) (*repoContext, error) {
	ctx := getContextFromProvider(provider)

	if err := fillMissingRepoInfo(ctx); err != nil {
		return nil, err
	}

	fillMissingBranch(ctx)
	fillMissingSHA(ctx)

	if ctx.Owner == "" || ctx.Repo == "" {
		return nil, errUtils.Build(errUtils.ErrFailedToGetRepoInfo).
			WithExplanation("Could not determine repository owner and name").
			WithHint("Ensure you're in a git repository with a remote configured").
			Err()
	}

	return ctx, nil
}

// getContextFromProvider extracts context from the CI provider.
func getContextFromProvider(provider cipkg.Provider) *repoContext {
	ctx := &repoContext{}
	ciCtx, ctxErr := provider.Context()
	if ctxErr == nil && ciCtx != nil {
		ctx.Owner = ciCtx.RepoOwner
		ctx.Repo = ciCtx.RepoName
		ctx.SHA = ciCtx.SHA
		ctx.Branch = ciCtx.Branch
	}
	return ctx
}

// fillMissingRepoInfo fills in missing owner/repo from local git.
func fillMissingRepoInfo(ctx *repoContext) error {
	if ctx.Owner != "" && ctx.Repo != "" {
		return nil
	}

	gitRepo := git.NewDefaultGitRepo()
	info, gitErr := gitRepo.GetLocalRepoInfo()
	if gitErr != nil {
		return errUtils.Build(errUtils.ErrFailedToGetRepoInfo).
			WithCause(gitErr).
			WithExplanation("Could not determine repository from CI context or git").
			Err()
	}

	if ctx.Owner == "" {
		ctx.Owner = info.RepoOwner
	}
	if ctx.Repo == "" {
		ctx.Repo = info.RepoName
	}
	return nil
}

// fillMissingBranch fills in missing branch from local git.
func fillMissingBranch(ctx *repoContext) {
	if ctx.Branch != "" {
		return
	}

	localRepo, gitErr := git.GetLocalRepo()
	if gitErr != nil {
		return
	}

	head, refErr := localRepo.Head()
	if refErr != nil {
		return
	}
	ctx.Branch = head.Name().Short()
}

// fillMissingSHA fills in missing SHA from local git.
func fillMissingSHA(ctx *repoContext) {
	if ctx.SHA != "" {
		return
	}
	gitRepo := git.NewDefaultGitRepo()
	ctx.SHA, _ = gitRepo.GetCurrentCommitSHA()
}

// renderStatus renders the CI status to the terminal.
func renderStatus(status *cipkg.Status) {
	ui.Writef("Relevant pull requests in %s\n\n", status.Repository)

	// Current branch.
	if status.CurrentBranch != nil {
		renderBranchStatus(status.CurrentBranch)
	}

	// PRs created by user.
	if len(status.CreatedByUser) > 0 {
		ui.Writeln("\nCreated by you")
		for _, pr := range status.CreatedByUser {
			renderPRStatus(pr)
		}
	}

	// PRs requesting review.
	if len(status.ReviewRequests) > 0 {
		ui.Writeln("\nRequesting a code review from you")
		for _, pr := range status.ReviewRequests {
			renderPRStatus(pr)
		}
	}
}

// renderBranchStatus renders status for a branch.
func renderBranchStatus(bs *cipkg.BranchStatus) {
	ui.Writeln("Current branch")

	if bs.PullRequest != nil {
		renderPRStatus(bs.PullRequest)
	} else {
		ui.Writef("  Commit status for %s\n", truncateSHA(bs.CommitSHA))
		renderChecks(bs.Checks, "  ")
		ui.Writeln("\n  No open pull request for current branch.")
	}
}

// renderPRStatus renders status for a pull request.
func renderPRStatus(pr *cipkg.PRStatus) {
	ui.Writef("  #%d  %s [%s]\n", pr.Number, pr.Title, pr.Branch)

	if pr.AllPassed && len(pr.Checks) > 0 {
		ui.Writeln("    - All checks passing")
	} else {
		renderChecks(pr.Checks, "    ")
	}
}

// renderChecks renders a list of checks.
func renderChecks(checks []*cipkg.CheckStatus, indent string) {
	for _, check := range checks {
		icon := getCheckIcon(check.CheckState())
		ui.Writef("%s- %s %s (%s)\n", indent, icon, check.Name, check.Conclusion)
	}
}

// getCheckIcon returns the icon for a check state.
func getCheckIcon(state cipkg.CheckStatusState) string {
	switch state {
	case cipkg.CheckStatusStateSuccess:
		return "\u2713" // Check mark.
	case cipkg.CheckStatusStateFailure:
		return "\u2717" // X mark.
	case cipkg.CheckStatusStatePending:
		return "\u25CB" // Open circle.
	case cipkg.CheckStatusStateCancelled:
		return "\u25CF" // Filled circle.
	case cipkg.CheckStatusStateSkipped:
		return "\u2212" // Minus.
	default:
		return "?"
	}
}

// truncateSHA truncates a SHA to 7 characters.
func truncateSHA(sha string) string {
	if len(sha) > shortSHALength {
		return sha[:shortSHALength]
	}
	return sha
}

// init is intentionally empty - statusCmd is added to ciCmd in ci.go's init().
func init() {
	// Ensure the GitHub provider is registered by importing the package.
	// The import above handles this.
}
