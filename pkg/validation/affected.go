package validation

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// AffectedFiles returns repository-relative files changed between HEAD and its
// merge-base with base. When base is empty, it uses the GitHub event's base SHA
// (when available), then the GitHub base branch, and finally origin/HEAD.
//
// Deleted paths are intentionally retained. Callers can use them to decide
// whether a dependent validator must run, while skipping files that can no
// longer be read.
func AffectedFiles(base string) ([]string, error) {
	base, explicit := resolveAffectedBase(base)
	mergeBase, err := runGit("merge-base", "HEAD", base)
	if err != nil && !explicit && base == "origin/HEAD" {
		mergeBase, err = runGit("merge-base", "HEAD", "HEAD~1")
	}
	if err != nil {
		return nil, fmt.Errorf("resolve validation base %q: %w", base, err)
	}

	output, err := runGit("diff", "--name-only", "-z", "--diff-filter=ACMRD", strings.TrimSpace(mergeBase)+"...HEAD")
	if err != nil {
		return nil, fmt.Errorf("list files changed since %s: %w", strings.TrimSpace(mergeBase), err)
	}
	worktree, err := runGit("diff", "--name-only", "-z", "--diff-filter=ACMRD")
	if err != nil {
		return nil, fmt.Errorf("list uncommitted changed files: %w", err)
	}
	untracked, err := runGit("ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, fmt.Errorf("list untracked files: %w", err)
	}

	paths := strings.Split(output+worktree+untracked, "\x00")
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
	return result, nil
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
	if branch := strings.TrimSpace(os.Getenv("GITHUB_BASE_REF")); branch != "" {
		return "origin/" + branch, false
	}
	return "origin/HEAD", false
}

func githubEventBaseSHA() string {
	path := strings.TrimSpace(os.Getenv("GITHUB_EVENT_PATH"))
	if path == "" {
		return ""
	}
	body, err := os.ReadFile(path)
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
