package validation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// nullSeparatedFlag makes git separate listed paths with NUL so unusual file names survive.
const nullSeparatedFlag = "-z"

var errValidationExcludePatternEmpty = errors.New("validation exclude pattern cannot be empty")

// AffectedFiles returns repository-relative files changed between HEAD and its
// merge-base with base. When base is empty, it uses the GitHub event's base SHA
// (when available), then the GitHub base branch, and finally origin/HEAD.
//
// Deleted paths are intentionally retained. Callers can use them to decide
// whether a dependent validator must run, while skipping files that can no
// longer be read.
func AffectedFiles(base string) ([]string, error) {
	defer perf.Track(nil, "validation.AffectedFiles")()

	mergeBase, err := resolveAffectedMergeBase(base)
	if err != nil {
		return nil, err
	}

	combined, err := collectAffectedDiffOutputs(mergeBase)
	if err != nil {
		return nil, err
	}

	return dedupeAffectedPaths(combined), nil
}

// resolveAffectedMergeBase resolves the requested base to a merge-base commit,
// falling back to HEAD~1 when origin/HEAD cannot be resolved (for example, a
// shallow checkout with no remote-tracking branch).
func resolveAffectedMergeBase(base string) (string, error) {
	resolvedBase, explicit := resolveAffectedBase(base)
	mergeBase, err := runGit("merge-base", "HEAD", resolvedBase)
	if err != nil && isNotGitRepository(err) {
		return "", notInGitRepositoryError()
	}
	if err != nil && !explicit && resolvedBase == "origin/HEAD" {
		// Report the failure of the base the user would recognize, not of the fallback.
		var fallbackErr error
		if mergeBase, fallbackErr = runGit("merge-base", "HEAD", "HEAD~1"); fallbackErr == nil {
			err = nil
		}
	}
	if err != nil {
		return "", unresolvedBaseError(resolvedBase, explicit, err)
	}
	return mergeBase, nil
}

// notInGitRepositoryError explains that --affected needs Git history. The message carries the
// guidance because machine-readable reports include the message but not the hints.
func notInGitRepositoryError() error {
	const guidance = "--affected compares against Git history; run inside a repository or drop --affected"
	return errUtils.Build(fmt.Errorf("%w (%s)", errUtils.ErrNotInGitRepository, guidance)).
		WithHint(guidance).
		Err()
}

// unresolvedBaseError reports a base that Git cannot resolve, keeping the base name and Git's own
// explanation. A default base (origin/HEAD) gets the fix for the usual cause; an explicit --base
// is the user's own input, so it is only named.
func unresolvedBaseError(base string, explicit bool, cause error) error {
	if explicit {
		return errUtils.Build(fmt.Errorf("%w %q: %w", errUtils.ErrAffectedBaseUnresolved, base, cause)).
			WithHint("Check that --base names an existing branch, tag, or commit; fetch it first if it only exists on the remote").
			Err()
	}
	return errUtils.Build(fmt.Errorf("%w %q (pass --base=<ref>): %w", errUtils.ErrAffectedBaseUnresolved, base, cause)).
		WithHint("Pass --base=<ref> to choose the branch, tag, or commit to compare against").
		WithHint("Run git remote set-head origin --auto to set origin/HEAD").
		Err()
}

// collectAffectedDiffOutputs gathers and concatenates the NUL-separated path
// lists that make up the affected set: committed changes since mergeBase,
// unstaged changes, staged changes, and untracked files. Every list is
// repository-root relative regardless of the current working directory.
func collectAffectedDiffOutputs(mergeBase string) (string, error) {
	output, err := runGit("diff", "--name-only", nullSeparatedFlag, "--diff-filter=ACMRD", strings.TrimSpace(mergeBase)+"...HEAD")
	if err != nil {
		return "", fmt.Errorf("list files changed since %s: %w", strings.TrimSpace(mergeBase), err)
	}
	worktree, err := runGit("diff", "--name-only", nullSeparatedFlag, "--diff-filter=ACMRD")
	if err != nil {
		return "", fmt.Errorf("list uncommitted changed files: %w", err)
	}
	staged, err := runGit("diff", "--cached", "--name-only", nullSeparatedFlag, "--diff-filter=ACMRD")
	if err != nil {
		return "", fmt.Errorf("list staged changed files: %w", err)
	}
	// ls-files is cwd-scoped and cwd-relative by default; --full-name makes the
	// paths repository-root relative and the ":/" pathspec makes it cover the
	// whole repository even when run from a subdirectory.
	untracked, err := runGit("ls-files", "--others", "--exclude-standard", "--full-name", nullSeparatedFlag, "--", ":/")
	if err != nil {
		return "", fmt.Errorf("list untracked files: %w", err)
	}
	return output + worktree + staged + untracked, nil
}

// dedupeAffectedPaths splits a NUL-separated path list, normalizes separators,
// and removes duplicates while preserving first-seen order.
func dedupeAffectedPaths(combined string) []string {
	paths := strings.Split(combined, "\x00")
	result := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		path = filepath.ToSlash(path)
		if _, ok := seen[path]; !ok {
			seen[path] = struct{}{}
			result = append(result, path)
		}
	}
	return result
}

// gitCommandError is a failed Git invocation together with what Git printed to stderr, which holds
// the actual reason that "exit status 128" does not.
type gitCommandError struct {
	err    error
	stderr string
}

func (e *gitCommandError) Error() string {
	defer perf.Track(nil, "validation.gitCommandError.Error")()

	if e.stderr == "" {
		return e.err.Error()
	}
	// Git's explanation replaces "exit status 128", which only says that Git failed.
	return e.stderr
}

func (e *gitCommandError) Unwrap() error {
	defer perf.Track(nil, "validation.gitCommandError.Unwrap")()

	return e.err
}

// isNotGitRepository reports whether Git failed because the working directory is not in a repository.
func isNotGitRepository(err error) bool {
	var gitErr *gitCommandError
	return errors.As(err, &gitErr) && strings.Contains(gitErr.stderr, "not a git repository")
}

var runGit = func(args ...string) (string, error) {
	var stderr bytes.Buffer
	cmd := exec.Command("git", args...)
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		return "", &gitCommandError{err: err, stderr: strings.TrimSpace(stderr.String())}
	}
	return string(output), nil
}

func resolveAffectedBase(base string) (string, bool) {
	if base = strings.TrimSpace(base); base != "" {
		return base, true
	}
	if sha := githubEventBaseSHA(); sha != "" {
		return sha, false
	}
	if branch := strings.TrimSpace(os.Getenv("GITHUB_BASE_REF")); branch != "" { //nolint:forbidigo // GitHub Actions' own ambient env var, not an Atmos-owned flag/config value to bind via viper -- this package has no cobra command to bind against.
		return "origin/" + branch, false
	}
	return "origin/HEAD", false
}

func githubEventBaseSHA() string {
	path := strings.TrimSpace(os.Getenv("GITHUB_EVENT_PATH")) //nolint:forbidigo // GitHub Actions' own ambient env var, not an Atmos-owned flag/config value to bind via viper -- this package has no cobra command to bind against.
	if path == "" {
		return ""
	}
	body, err := os.ReadFile(path) //nolint:gosec // path is GITHUB_EVENT_PATH, provided by the GitHub Actions runner itself, not user input.
	if err != nil {
		return ""
	}
	var event struct {
		Before      string `json:"before"`
		PullRequest struct {
			Base struct {
				SHA string `json:"sha"`
			} `json:"base"`
		} `json:"pull_request"`
	}
	if err := json.Unmarshal(body, &event); err != nil {
		return ""
	}
	if event.PullRequest.Base.SHA != "" {
		return event.PullRequest.Base.SHA
	}
	return event.Before
}

// IsAtmosConfigPath reports whether path is one of the project-local Atmos
// configuration files read by the CLI configuration loader.
func IsAtmosConfigPath(path string) bool {
	defer perf.Track(nil, "validation.IsAtmosConfigPath")()

	path = filepath.ToSlash(filepath.Clean(path))
	if path == "atmos.yaml" || path == "atmos.yml" || path == ".atmos.yaml" || path == ".atmos.yml" {
		return true
	}
	for _, directory := range []string{"atmos.d/", ".atmos.d/", "profiles/", ".atmos/profiles/"} {
		if strings.HasPrefix(path, directory) {
			return true
		}
	}
	return false
}

// ExcludePaths removes paths matching any repository-relative glob pattern.
// Patterns use forward slashes and support doublestar (for example,
// "tests/fixtures/**") on every platform.
//
//nolint:revive // Path normalization and matching must remain together to preserve exclusion semantics.
func ExcludePaths(paths []string, patterns []string) ([]string, error) {
	defer perf.Track(nil, "validation.ExcludePaths")()
	if len(patterns) == 0 {
		return paths, nil
	}

	normalizedPatterns := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = strings.ReplaceAll(filepath.ToSlash(filepath.Clean(strings.TrimSpace(pattern))), "\\", "/")
		pattern = strings.TrimPrefix(pattern, "./")
		if pattern == "" || pattern == "." {
			return nil, errValidationExcludePatternEmpty
		}
		// Validate the glob even when the current path set is empty.
		if _, err := u.PathMatch(pattern, ""); err != nil {
			return nil, fmt.Errorf("invalid validation exclude pattern %q: %w", pattern, err)
		}
		normalizedPatterns = append(normalizedPatterns, pattern)
	}

	filtered := make([]string, 0, len(paths))
	for _, path := range paths {
		normalizedPath, err := validationRepositoryPath(path)
		if err != nil {
			return nil, err
		}
		excluded := false
		for _, pattern := range normalizedPatterns {
			match, err := u.PathMatch(pattern, normalizedPath)
			if err != nil {
				return nil, fmt.Errorf("match validation exclude pattern %q: %w", pattern, err)
			}
			if match {
				excluded = true
				break
			}
		}
		if !excluded {
			filtered = append(filtered, path)
		}
	}
	return filtered, nil
}

func validationRepositoryPath(path string) (string, error) {
	if filepath.IsAbs(path) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working directory for validation excludes: %w", err)
		}
		// filepath.Rel errors when path and cwd are on different Windows volumes
		// (e.g. a t.TempDir() on C: while the repo is checked out on D:, as GitHub
		// Actions Windows runners do). That path is outside the repo either way, so
		// no repo-relative exclude glob can legitimately match it -- fall back to
		// the absolute path unchanged rather than failing the whole exclude call.
		if relative, err := filepath.Rel(cwd, path); err == nil {
			path = relative
		}
	}
	path = strings.ReplaceAll(filepath.ToSlash(filepath.Clean(path)), "\\", "/")
	return strings.TrimPrefix(path, "./"), nil
}
