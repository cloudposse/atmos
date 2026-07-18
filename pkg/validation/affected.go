package validation

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	u "github.com/cloudposse/atmos/pkg/utils"
)

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
	if err != nil && !explicit && resolvedBase == "origin/HEAD" {
		mergeBase, err = runGit("merge-base", "HEAD", "HEAD~1")
	}
	if err != nil {
		return "", fmt.Errorf("resolve validation base %q: %w", resolvedBase, err)
	}
	return mergeBase, nil
}

// collectAffectedDiffOutputs gathers and concatenates the NUL-separated path
// lists that make up the affected set: committed changes since mergeBase,
// uncommitted changes, and untracked files.
func collectAffectedDiffOutputs(mergeBase string) (string, error) {
	output, err := runGit("diff", "--name-only", "-z", "--diff-filter=ACMRD", strings.TrimSpace(mergeBase)+"...HEAD")
	if err != nil {
		return "", fmt.Errorf("list files changed since %s: %w", strings.TrimSpace(mergeBase), err)
	}
	worktree, err := runGit("diff", "--name-only", "-z", "--diff-filter=ACMRD")
	if err != nil {
		return "", fmt.Errorf("list uncommitted changed files: %w", err)
	}
	untracked, err := runGit("ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return "", fmt.Errorf("list untracked files: %w", err)
	}
	return output + worktree + untracked, nil
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

var runGit = func(args ...string) (string, error) {
	output, err := exec.Command("git", args...).Output()
	if err != nil {
		return "", err
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
		relative, err := filepath.Rel(cwd, path)
		if err != nil {
			return "", fmt.Errorf("resolve validation path %q: %w", path, err)
		}
		path = relative
	}
	path = strings.ReplaceAll(filepath.ToSlash(filepath.Clean(path)), "\\", "/")
	return strings.TrimPrefix(path, "./"), nil
}
