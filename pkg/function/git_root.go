package function

import (
	"context"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosGit "github.com/cloudposse/atmos/pkg/git"
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
			FunctionAliases: []string{"git-root", TagGitRoot},
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

	result, err := atmosGit.ProcessTagRoot(strings.TrimSpace(fmt.Sprintf("%s %s", atmosGit.YAMLFuncRepoRoot, args)))
	if err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrGitRoot, err)
	}

	log.Debug("Resolved repo-root", "path", result)

	return result, nil
}
