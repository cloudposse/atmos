package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/retry"
	"github.com/cloudposse/atmos/pkg/schema"
)

const worktreeSubdir = "worktree"

// gitRefNotFoundPatterns are case-insensitive substrings in `git worktree add`
// stderr that indicate the failure was caused by the target ref/SHA not being
// present in the local object DB (as opposed to other failures like path
// conflicts, permission errors, or corrupted repo state). Keeping this list
// narrow lets CreateWorktreeWithFetchRecovery gate its self-heal path on
// only true missing-ref failures.
var gitRefNotFoundPatterns = []string{
	"invalid reference",
	"unknown revision",
	"not a commit",
	"not a valid object name",
	"ambiguous argument",
}

// isGitRefNotFound returns true if git's stderr indicates the target ref/SHA
// is unknown to the local object DB.
func isGitRefNotFound(output string) bool {
	lower := strings.ToLower(output)
	for _, p := range gitRefNotFoundPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// classifyWorktreeAddError wraps a `git worktree add` failure with the
// appropriate sentinel: ErrGitRefNotFound only for true missing-ref cases
// (so the recovery path can target them), ErrGitWorktreeAdd for everything
// else. The "make sure the ref is correct" hint is bound to the missing-ref
// case so it doesn't mislead users hitting unrelated failures.
func classifyWorktreeAddError(cause error, ref, output string) error {
	if isGitRefNotFound(output) {
		return errUtils.Build(errUtils.ErrGitRefNotFound).
			WithCause(cause).
			WithContext("ref", ref).
			WithContext("output", output).
			WithHint("Make sure the ref is correct and was cloned by Git from the remote, or use the '--clone-target-ref=true' flag to clone it.").
			WithHint("Refer to https://atmos.tools/cli/commands/describe/affected for more details.").
			Err()
	}
	return errUtils.Build(errUtils.ErrGitWorktreeAdd).
		WithCause(cause).
		WithContext("ref", ref).
		WithContext("output", output).
		Err()
}

// CreateWorktree creates a new git worktree at the specified path, checked out to the given target ref or SHA.
// This uses `git worktree add --detach` to create an isolated worktree that shares the repository's
// object database but has its own HEAD, allowing checkout operations without affecting the main worktree.
// The repoDir should be the path to any directory in the repository (main worktree or any linked worktree).
// Returns the path to the created worktree directory.
func CreateWorktree(repoDir, targetCommit string) (string, error) {
	defer perf.Track(nil, "git.CreateWorktree")()

	// Create a temp dir for the worktree.
	// Note: We create a parent temp dir and use a subdirectory because
	// git worktree add requires the target directory to not exist.
	tempParentDir, err := os.MkdirTemp("", "atmos-worktree-")
	if err != nil {
		return "", err
	}
	worktreePath := filepath.Join(tempParentDir, worktreeSubdir)

	// Use git worktree add to create an isolated worktree.
	// The --detach flag creates the worktree with a detached HEAD, which is what we want
	// since we're checking out a specific ref/sha for comparison purposes.
	// This works correctly whether we're in a regular repo or already in a worktree.
	cmd := exec.Command("git", "worktree", "add", "--detach", worktreePath, targetCommit)
	cmd.Dir = repoDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		// Clean up the temp parent directory on error.
		os.RemoveAll(tempParentDir)
		return "", classifyWorktreeAddError(err, targetCommit, string(output))
	}

	log.Debug("Created git worktree", "dir", worktreePath, "target", targetCommit)

	return worktreePath, nil
}

// CreateWorktreeWithFetchRecovery creates a worktree at targetCommit, with a
// one-shot self-heal: if the initial CreateWorktree call fails because the
// target commit is missing from the local object DB AND a non-empty
// targetBranch is provided, the function performs a targeted
// `git fetch origin <targetBranch>` and retries.
//
// This is the common shallow-clone CI scenario: actions/checkout@v6 with the
// default fetch-depth=1 only pulls the PR head, so a base SHA resolved from
// the GitHub event payload (event.pull_request.base.sha) often is not in the
// local object DB. A targeted fetch of the target branch is enough to make
// the SHA available without paying for a full unshallow.
//
// Recovery is gated to ErrGitRefNotFound so that unrelated failures (temp
// directory creation, repo state corruption, permissions, etc.) propagate
// directly instead of being misdiagnosed as "target commit not available
// locally" and noisily attempting an unrelated fetch.
//
// On final failure, the original CreateWorktree error is preserved (joined
// with the fetch error if the fetch also failed) so the caller can still
// surface its hints to the user.
func CreateWorktreeWithFetchRecovery(repoDir, targetCommit, targetBranch string) (string, error) {
	defer perf.Track(nil, "git.CreateWorktreeWithFetchRecovery")()

	worktreePath, err := CreateWorktree(repoDir, targetCommit)
	if err == nil || targetBranch == "" || !errors.Is(err, errUtils.ErrGitRefNotFound) {
		return worktreePath, err
	}

	log.Info("Target commit not available locally, fetching base branch", "branch", targetBranch)
	if fetchErr := FetchRef(repoDir, targetBranch); fetchErr != nil {
		log.Debug("Auto-fetch failed during worktree creation", "branch", targetBranch, "error", fetchErr)
		return "", errors.Join(err, fetchErr)
	}
	return CreateWorktree(repoDir, targetCommit)
}

const (
	removeWorktreeInitialDelay   = 200 * time.Millisecond
	removeWorktreeMaxDelay       = 2 * time.Second
	removeWorktreeMultiplier     = 2.0
	removeWorktreeMaxElapsedTime = 20 * time.Second
)

// removeWorktreeRetryConfig bounds the transient-failure retry in RemoveWorktree by elapsed time
// rather than attempt count: on Windows CI runners the lock observed in practice is most likely an
// antivirus/real-time-scan hold on the worktree's administrative directory, which can outlast a
// handful of sub-second attempts (a fixed 5-attempt budget was observed to exhaust in ~3s and still
// fail in CI). Retrying every couple of seconds for up to removeWorktreeMaxElapsedTime gives that
// scan time to finish without materially slowing down the common case, which succeeds immediately.
func removeWorktreeRetryConfig() *schema.RetryConfig {
	initialDelay := removeWorktreeInitialDelay
	maxDelay := removeWorktreeMaxDelay
	multiplier := removeWorktreeMultiplier
	maxElapsedTime := removeWorktreeMaxElapsedTime
	return &schema.RetryConfig{
		BackoffStrategy: schema.BackoffExponential,
		InitialDelay:    &initialDelay,
		MaxDelay:        &maxDelay,
		Multiplier:      &multiplier,
		MaxElapsedTime:  &maxElapsedTime,
	}
}

// isTransientWorktreeRemoveError reports whether output from a failed `git worktree remove`
// indicates a lingering file handle rather than a genuine failure -- observed on Windows CI
// runners, where a just-exited git subprocess can still hold the worktree's administrative
// directory (.git/worktrees/<name>) open for a short window, causing git itself to fail deleting
// it with "Invalid argument" (Windows ERROR_INVALID_PARAMETER surfacing through git-for-windows)
// or the OS to report the path as still in use.
func isTransientWorktreeRemoveError(output string) bool {
	lower := strings.ToLower(output)
	transientPatterns := []string{
		"invalid argument",
		"being used by another process",
		"resource busy",
		"device or resource busy",
	}
	for _, p := range transientPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

type worktreeCommandRunner func(repoDir string, args ...string) (string, error)

func runWorktreeCommand(repoDir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = repoDir
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// removeWorktree retries removal failures caused by transient filesystem locks. If Git has already
// removed the worktree directory but cannot remove its administrative metadata, it falls back to
// `git worktree prune --expire now` to clear that stale registration. This occurs on Windows CI
// when a scanner briefly holds a handle in .git/worktrees/<name> after the worktree command exits.
func removeWorktree(repoDir, worktreePath string, run worktreeCommandRunner, config *schema.RetryConfig) (string, error) {
	var removeOutput string
	removeErr := retry.WithPredicate(context.Background(), config, func() error {
		output, err := run(repoDir, "worktree", "remove", "--force", worktreePath)
		removeOutput = output
		return err
	}, func(error) bool { return isTransientWorktreeRemoveError(removeOutput) })
	if removeErr == nil || !isTransientWorktreeRemoveError(removeOutput) {
		return removeOutput, removeErr
	}

	var pruneOutput string
	pruneErr := retry.WithPredicate(context.Background(), config, func() error {
		output, err := run(repoDir, "worktree", "prune", "--expire", "now")
		pruneOutput = output
		return err
	}, func(error) bool { return isTransientWorktreeRemoveError(pruneOutput) })
	if pruneErr == nil {
		return pruneOutput, nil
	}
	return "worktree remove:\n" + removeOutput + "\nworktree prune:\n" + pruneOutput, errors.Join(removeErr, pruneErr)
}

// RemoveWorktree removes a git worktree using `git worktree remove`.
// This properly unregisters the worktree from git's tracking in addition to removing the directory.
// The repoDir parameter should be the path to any directory in the repository (main worktree or any linked worktree).
func RemoveWorktree(repoDir, worktreePath string) {
	defer perf.Track(nil, "git.RemoveWorktree")()

	lastOutput, err := removeWorktree(repoDir, worktreePath, runWorktreeCommand, removeWorktreeRetryConfig())
	if err != nil {
		log.Warn("Failed to remove git worktree", "path", worktreePath, "error", err.Error(), "output", lastOutput)
	}
}

// GetWorktreeParentDir returns the parent directory of a worktree path.
// This is useful for cleanup since CreateWorktree creates a parent temp dir containing the worktree.
func GetWorktreeParentDir(worktreePath string) string {
	defer perf.Track(nil, "git.GetWorktreeParentDir")()

	// The worktree is always created as a "worktree" subdirectory of the parent temp dir.
	// So parent is worktreePath minus the worktree suffix.
	suffix := string(filepath.Separator) + worktreeSubdir
	if strings.HasSuffix(worktreePath, suffix) {
		return strings.TrimSuffix(worktreePath, suffix)
	}
	// Fallback: just return the path itself.
	return worktreePath
}
