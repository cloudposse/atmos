package pro

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	maxCommitMessageLen = 500
	maxCommentLen       = 2000
	maxBranchLen        = 256
	maxChanges          = 200
	maxFileSizeBytes    = 2 * 1024 * 1024 // 2 MiB.
)

// AtmosProBotActor is the GitHub actor name for the Atmos Pro GitHub App.
// When the app pushes a commit, the subsequent CI run sees this as the actor.
const AtmosProBotActor = "atmos-pro[bot]"

// branchPattern matches valid branch names: word chars, dots, hyphens, slashes.
var branchPattern = regexp.MustCompile(`^[\w.\-/]+$`)

// ExecuteCommit detects changed files in the git index, base64-encodes their
// contents, and sends them to Atmos Pro to create a server-side commit via the
// GitHub App. This ensures commits trigger CI (unlike GITHUB_TOKEN commits).
//
// Loop prevention: if GITHUB_ACTOR is the Atmos Pro bot, the command exits
// early with a success message. This protects users who forget to add the
// `if: github.actor != 'atmos-pro[bot]'` guard to their workflow.
func ExecuteCommit(atmosConfig *schema.AtmosConfiguration, message, comment, addPattern string, stageAll bool) error {
	defer perf.Track(atmosConfig, "pro.ExecuteCommit")()

	// Prevent infinite loops: if this workflow was triggered by the Atmos Pro
	// bot itself, skip the commit silently.
	if atmosConfig != nil && atmosConfig.Settings.GithubUsername == AtmosProBotActor {
		ui.Info("Skipping commit: triggered by " + AtmosProBotActor + " (loop prevention)")
		return nil
	}

	if err := validateCommitInputs(message, comment); err != nil {
		return err
	}

	if err := validateAuth(atmosConfig); err != nil {
		return err
	}

	if err := atmosgit.EnsureGitSafeDirectory(); err != nil {
		return err
	}

	if err := stageFiles(addPattern, stageAll); err != nil {
		return err
	}

	branch, err := resolveBranch(atmosConfig)
	if err != nil {
		return err
	}

	changes, err := buildChanges()
	if err != nil {
		return err
	}

	if changes == nil {
		return nil
	}

	return submitCommit(atmosConfig, branch, message, comment, changes)
}

// resolveBranch reads and validates the PR branch from config (GITHUB_HEAD_REF).
func resolveBranch(atmosConfig *schema.AtmosConfiguration) (string, error) {
	var branch string
	if atmosConfig != nil {
		branch = atmosConfig.Settings.Pro.GitHubHeadRef
	}

	if branch == "" {
		return "", errUtils.Build(errUtils.ErrBranchRequired).
			WithHint("This command is designed for GitHub Actions pull_request workflows where GITHUB_HEAD_REF is set automatically.").
			Err()
	}

	if err := validateBranch(branch); err != nil {
		return "", err
	}

	return branch, nil
}

// validateAuth checks that the Atmos Pro authentication prerequisites are met
// before attempting any API calls. This provides fast, actionable errors instead
// of letting the request fail deep in the OIDC exchange.
func validateAuth(atmosConfig *schema.AtmosConfiguration) error {
	// Static token is sufficient.
	if atmosConfig.Settings.Pro.Token != "" {
		return nil
	}

	// OIDC path: need both OIDC env vars + workspace ID.
	oidc := atmosConfig.Settings.Pro.GithubOIDC
	if oidc.RequestURL == "" || oidc.RequestToken == "" {
		return errUtils.Build(errUtils.ErrNotInGitHubActions).
			WithHint("Atmos Pro authenticates via GitHub OIDC — run this command from a GitHub Actions workflow with `id-token: write` permission.").
			WithHint("Set `ATMOS_PRO_WORKSPACE_ID` (or `settings.pro.workspace_id`) to your Atmos Pro workspace ID.").
			WithHint("See https://atmos-pro.com/docs/configure/github-workflows for setup instructions.").
			Err()
	}

	if atmosConfig.Settings.Pro.WorkspaceID == "" {
		return errUtils.Build(errUtils.ErrOIDCWorkspaceIDRequired).
			WithHint("Set ATMOS_PRO_WORKSPACE_ID to your Atmos Pro workspace ID.").
			WithHint("Find your workspace ID in the Atmos Pro dashboard: https://app.atmos-pro.com").
			Err()
	}

	return nil
}

// buildChanges detects staged changes, validates paths, and collects file contents.
// Returns nil if there are no changes to commit.
func buildChanges() (*dtos.CommitChanges, error) {
	additions, deletions, err := detectChanges()
	if err != nil {
		return nil, err
	}

	if len(additions) == 0 && len(deletions) == 0 {
		ui.Info("No changes to commit")
		return nil, nil
	}

	if err := validateChangeCount(additions, deletions); err != nil {
		return nil, err
	}

	additions = filterPaths(additions)
	deletions = filterPaths(deletions)

	fileAdditions, err := collectFileContents(additions)
	if err != nil {
		return nil, err
	}

	fileDeletions := buildDeletions(deletions)

	if len(fileAdditions) == 0 && len(fileDeletions) == 0 {
		ui.Info("No changes to commit (all files were filtered out)")
		return nil, nil
	}

	return &dtos.CommitChanges{
		Additions: fileAdditions,
		Deletions: fileDeletions,
	}, nil
}

// submitCommit creates the API client and sends the commit request.
func submitCommit(atmosConfig *schema.AtmosConfiguration, branch, message, comment string, changes *dtos.CommitChanges) error {
	apiClient, err := NewAtmosProAPIClientFromEnv(atmosConfig)
	if err != nil {
		return err
	}

	req := &dtos.CommitRequest{
		Branch:        branch,
		Changes:       *changes,
		CommitMessage: message,
		Comment:       comment,
	}

	resp, err := apiClient.CreateCommit(req)
	if err != nil {
		return err
	}

	// Output raw SHA when piped (e.g., `atmos pro commit | jq`),
	// otherwise show human-friendly UI message.
	term := terminal.New()
	if term.IsPiped(terminal.Stdout) {
		if err := data.Writeln(resp.Data.SHA); err != nil {
			log.Debug("Failed to write commit SHA to stdout.", "error", err)
		}
	} else {
		ui.Successf("Commit created: %s", resp.Data.SHA)
	}

	return nil
}

// validateChangeCount checks that the total number of changes does not exceed the limit.
func validateChangeCount(additions, deletions []string) error {
	totalChanges := len(additions) + len(deletions)
	if totalChanges > maxChanges {
		return errUtils.Build(errUtils.ErrTooManyChanges).
			WithCausef("%d changed files exceeds the limit of %d", totalChanges, maxChanges).
			WithHint("Reduce the number of changed files or split the commit into smaller batches.").
			Err()
	}

	return nil
}

// buildDeletions converts a list of paths into CommitFileDeletion DTOs.
func buildDeletions(paths []string) []dtos.CommitFileDeletion {
	result := make([]dtos.CommitFileDeletion, 0, len(paths))
	for _, p := range paths {
		result = append(result, dtos.CommitFileDeletion{Path: p})
	}

	return result
}

// validateCommitInputs checks message and comment length constraints.
func validateCommitInputs(message, comment string) error {
	if message == "" {
		return errUtils.ErrCommitMessageRequired
	}

	if len(message) > maxCommitMessageLen {
		return errUtils.Build(errUtils.ErrCommitMessageTooLong).
			WithCausef("message is %d characters", len(message)).
			WithHint("Commit messages must be 500 characters or fewer.").
			Err()
	}

	if len(comment) > maxCommentLen {
		return errUtils.Build(errUtils.ErrCommentTooLong).
			WithCausef("comment is %d characters", len(comment)).
			WithHint("PR comments must be 2000 characters or fewer.").
			Err()
	}

	return nil
}

// validateBranch checks that the branch name matches the allowed pattern.
func validateBranch(branch string) error {
	if len(branch) > maxBranchLen {
		return errUtils.Build(errUtils.ErrBranchInvalid).
			WithCausef("branch name is %d characters (max %d)", len(branch), maxBranchLen).
			Err()
	}

	if !branchPattern.MatchString(branch) {
		return errUtils.Build(errUtils.ErrBranchInvalid).
			WithCausef("branch %q contains invalid characters", branch).
			WithHint("Branch names may only contain word characters, dots, hyphens, and slashes.").
			Err()
	}

	return nil
}

// stageFiles runs git add based on the provided flags.
func stageFiles(addPattern string, stageAll bool) error {
	if addPattern == "" && !stageAll {
		// No staging requested — use whatever is already in the index.
		return nil
	}

	var args []string
	if stageAll {
		args = []string{"add", "-A"}
	} else {
		args = []string{"add", addPattern}
	}

	log.Debug("Staging files.", "args", args)

	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%w: git add %s: %w", errUtils.ErrFailedToStageChanges, strings.Join(args[1:], " "), err)
	}

	return nil
}

// detectChanges returns lists of added/modified and deleted file paths from the git index.
func detectChanges() (additions []string, deletions []string, err error) {
	additions, err = gitDiffCachedNames("AM")
	if err != nil {
		return nil, nil, fmt.Errorf("%w: additions: %w", errUtils.ErrFailedToDetectChanges, err)
	}

	deletions, err = gitDiffCachedNames("D")
	if err != nil {
		return nil, nil, fmt.Errorf("%w: deletions: %w", errUtils.ErrFailedToDetectChanges, err)
	}

	return additions, deletions, nil
}

// gitDiffCachedNames runs git diff --cached --name-only with the given filter.
func gitDiffCachedNames(filter string) ([]string, error) {
	args := []string{"diff", "--cached", "--name-only", "--diff-filter=" + filter}
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	// Filter empty strings from split.
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" {
			result = append(result, line)
		}
	}

	return result, nil
}

// filterPaths removes invalid paths and logs warnings for skipped files.
func filterPaths(paths []string) []string {
	result := make([]string, 0, len(paths))

	for _, p := range paths {
		if err := validatePath(p); err != nil {
			ui.Warning(fmt.Sprintf("Skipping %s: %s", p, err))
			continue
		}
		result = append(result, p)
	}

	return result
}

// validatePath checks that a file path is safe for the commit API.
func validatePath(p string) error {
	// Reject absolute paths.
	if filepath.IsAbs(p) || strings.HasPrefix(p, "/") {
		return fmt.Errorf("%w: absolute path not allowed: %s", errUtils.ErrCommitInvalidFilePath, p)
	}

	// Reject path traversal.
	if strings.Contains(p, "..") {
		return fmt.Errorf("%w: path traversal not allowed: %s", errUtils.ErrCommitInvalidFilePath, p)
	}

	// Reject .github/ paths (case-insensitive) to prevent workflow injection.
	if strings.HasPrefix(strings.ToLower(p), ".github/") {
		return fmt.Errorf("%w: .github/ paths are not allowed: %s", errUtils.ErrCommitInvalidFilePath, p)
	}

	return nil
}

// collectFileContents reads each file and base64-encodes its contents.
// Files exceeding 2 MiB are skipped with a warning.
func collectFileContents(paths []string) ([]dtos.CommitFileAddition, error) {
	result := make([]dtos.CommitFileAddition, 0, len(paths))

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, fmt.Errorf("failed to stat file %s: %w", p, err)
		}

		if info.Size() > maxFileSizeBytes {
			ui.Warning(fmt.Sprintf("Skipping %s: file size %d bytes exceeds 2 MiB limit", p, info.Size()))
			continue
		}

		contents, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", p, err)
		}

		result = append(result, dtos.CommitFileAddition{
			Path:     p,
			Contents: base64.StdEncoding.EncodeToString(contents),
		})
	}

	return result, nil
}
