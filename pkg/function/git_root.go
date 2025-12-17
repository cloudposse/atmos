package function

import (
	"context"
	"os/exec"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// GitRootFunction implements the repo-root function for getting the git repository root.
type GitRootFunction struct {
	BaseFunction
}

// NewGitRootFunction creates a new repo-root function handler.
func NewGitRootFunction() *GitRootFunction {
	defer perf.Track(nil, "function.NewGitRootFunction")()

	return &GitRootFunction{
		BaseFunction: BaseFunction{
			FunctionName:    TagRepoRoot,
			FunctionAliases: []string{"git-root"},
			FunctionPhase:   PreMerge,
		},
	}
}

// Execute processes the repo-root function.
// Usage:
//
//	!repo-root   - Returns the absolute path to the git repository root
//
// Returns an error if not in a git repository.
func (f *GitRootFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.GitRootFunction.Execute")()

	log.Debug("Executing repo-root function")

	//nolint:gosec // intentional subprocess call for git
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel")

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	result := strings.TrimSpace(string(output))
	log.Debug("Resolved repo-root", "path", result)

	return result, nil
}
