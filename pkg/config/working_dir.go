package config

import (
	"os"
	"path/filepath"
	"strings"

	atmosGit "github.com/cloudposse/atmos/pkg/git"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// repoRootTagPrefix is the YAML-function prefix whose remainder is the default returned when no
// repository is found.
const repoRootTagPrefix = "!repo-root"

// workingDirOrProcess returns wd when it is set, otherwise the process working directory.
// Every shared path helper takes an optional working directory so that callers such as
// ResolveConfigCandidates can inspect another project without changing the process state.
func workingDirOrProcess(wd string) (string, error) {
	if wd != "" {
		return wd, nil
	}
	return osGetwd()
}

// absoluteFrom resolves path against wd. An empty wd means the process working directory.
func absoluteFrom(wd, path string) (string, error) {
	if wd != "" && !filepath.IsAbs(path) {
		path = filepath.Join(wd, path)
	}
	return filepath.Abs(path)
}

// processGitRoot resolves the Git worktree root for wd with the semantics of u.ProcessTagGitRoot:
// the text after the tag is the default returned when no repository is found, and
// TEST_GIT_ROOT overrides detection. An empty wd uses the process working directory (and the
// invocation-scoped cache that the YAML functions share).
func processGitRoot(wd, input string) (string, error) {
	if wd == "" {
		return u.ProcessTagGitRoot(input)
	}
	//nolint:forbidigo // TEST_GIT_ROOT is specifically for test isolation, not application configuration.
	if testGitRoot := os.Getenv("TEST_GIT_ROOT"); testGitRoot != "" {
		return testGitRoot, nil
	}
	root, err := atmosGit.GetRootAt(wd)
	if err != nil {
		if fallback := strings.TrimSpace(strings.TrimPrefix(input, repoRootTagPrefix)); fallback != "" {
			return fallback, nil
		}
		return "", err
	}
	return root, nil
}
