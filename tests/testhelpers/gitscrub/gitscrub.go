// Package gitscrub removes the environment variables that point git at a repository other than
// the one in its working directory. Tests that spawn git against a throwaway repository use it so
// a caller's GIT_DIR (a git hook, a worktree-aware wrapper) cannot redirect them.
//
// It imports only the standard library, so any package's tests can use it without an import cycle.
package gitscrub

import (
	"os"
	"strings"
)

// Environ returns the process environment without the repository-location variables (see Filter).
func Environ() []string {
	return Filter(os.Environ())
}

// Filter returns env without the variables that make git operate on a repository other than the
// one in its working directory. Names are matched case-insensitively because Windows resolves
// environment variable names case-insensitively, so a lower or mixed-case GIT_DIR would otherwise
// slip through and stay active for git. Authentication and configuration variables (GIT_CONFIG_*,
// GIT_SSH_COMMAND, GIT_ASKPASS, and so on) are kept. The input is not modified.
func Filter(env []string) []string {
	kept := make([]string, 0, len(env))
	for _, kv := range env {
		key, _, _ := strings.Cut(kv, "=")
		switch strings.ToUpper(key) {
		case "GIT_DIR",
			"GIT_WORK_TREE",
			"GIT_INDEX_FILE",
			"GIT_OBJECT_DIRECTORY",
			"GIT_ALTERNATE_OBJECT_DIRECTORIES",
			// GIT_COMMON_DIR can redirect the repository's non-worktree files outside the throwaway
			// repository, and GIT_NAMESPACE would make a receive-pack store main under
			// refs/namespaces/<ns>/, where a backend that does not inherit it cannot see it.
			"GIT_COMMON_DIR",
			"GIT_NAMESPACE",
			// GIT_TEMPLATE_DIR would seed the throwaway repositories with the host's template,
			// including any executable hooks that could fail or hang a test's own commit.
			"GIT_TEMPLATE_DIR":
			continue
		}
		kept = append(kept, kv)
	}
	return kept
}
