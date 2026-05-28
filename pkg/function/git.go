package function

import (
	"context"
	"fmt"
	"strings"

	atmosGit "github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// GitShaFunction implements the git.sha function for getting the current HEAD SHA.
type GitShaFunction struct {
	BaseFunction
}

// NewGitShaFunction creates a new git.sha function handler.
func NewGitShaFunction() *GitShaFunction {
	defer perf.Track(nil, "function.NewGitShaFunction")()

	return &GitShaFunction{
		BaseFunction: BaseFunction{
			FunctionName:    TagGitSha,
			FunctionAliases: nil,
			FunctionPhase:   PreMerge,
		},
	}
}

// Execute processes the git.sha function.
func (f *GitShaFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.GitShaFunction.Execute")()

	log.Debug("Executing git.sha function")

	result, err := atmosGit.ProcessTagSHA(strings.TrimSpace(fmt.Sprintf("%s %s", atmosGit.YAMLFuncSHA, args)))
	if err != nil {
		return "", fmt.Errorf("failed to get Git SHA: %w", err)
	}

	return result, nil
}

// GitBranchFunction implements the git.branch function for getting the current branch name.
type GitBranchFunction struct {
	BaseFunction
}

// NewGitBranchFunction creates a new git.branch function handler.
func NewGitBranchFunction() *GitBranchFunction {
	defer perf.Track(nil, "function.NewGitBranchFunction")()

	return &GitBranchFunction{
		BaseFunction: BaseFunction{
			FunctionName:    TagGitBranch,
			FunctionAliases: nil,
			FunctionPhase:   PreMerge,
		},
	}
}

// Execute processes the git.branch function.
func (f *GitBranchFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.GitBranchFunction.Execute")()

	log.Debug("Executing git.branch function")

	result, err := atmosGit.ProcessTagBranch(strings.TrimSpace(fmt.Sprintf("%s %s", atmosGit.YAMLFuncBranch, args)))
	if err != nil {
		return "", fmt.Errorf("failed to get Git branch: %w", err)
	}

	return result, nil
}

// GitRefFunction implements the git.ref function for immutable source pinning.
type GitRefFunction struct {
	BaseFunction
}

// NewGitRefFunction creates a new git.ref function handler.
func NewGitRefFunction() *GitRefFunction {
	defer perf.Track(nil, "function.NewGitRefFunction")()

	return &GitRefFunction{
		BaseFunction: BaseFunction{
			FunctionName:    TagGitRef,
			FunctionAliases: nil,
			FunctionPhase:   PreMerge,
		},
	}
}

// Execute processes the git.ref function.
func (f *GitRefFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.GitRefFunction.Execute")()

	log.Debug("Executing git.ref function")

	result, err := atmosGit.ProcessTagRef(strings.TrimSpace(fmt.Sprintf("%s %s", atmosGit.YAMLFuncRef, args)))
	if err != nil {
		return "", fmt.Errorf("failed to get Git ref: %w", err)
	}

	return result, nil
}
