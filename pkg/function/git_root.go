package function

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	git "github.com/go-git/go-git/v5"

	"github.com/cloudposse/atmos/pkg/perf"
)

// GitRootFunction implements the !repo-root YAML function.
// It returns the root directory of the Git repository.
type GitRootFunction struct {
	BaseFunction
	// testGitRoot is used for testing to override git root detection.
	testGitRoot string
}

// NewGitRootFunction creates a new GitRootFunction.
func NewGitRootFunction() *GitRootFunction {
	defer perf.Track(nil, "function.NewGitRootFunction")()

	return &GitRootFunction{
		BaseFunction: BaseFunction{
			FunctionName:    "repo-root",
			FunctionAliases: nil,
			FunctionPhase:   PreMerge,
		},
	}
}

// NewGitRootFunctionWithTestRoot creates a GitRootFunction with a test override.
func NewGitRootFunctionWithTestRoot(testRoot string) *GitRootFunction {
	defer perf.Track(nil, "function.NewGitRootFunctionWithTestRoot")()

	fn := NewGitRootFunction()
	fn.testGitRoot = testRoot
	return fn
}

// Execute processes the !repo-root function.
// Syntax: !repo-root [default_value]
// Returns the git repository root, or the default value if not in a git repo.
func (f *GitRootFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.GitRootFunction.Execute")()

	defaultValue := strings.TrimSpace(args)

	// Check for test override first.
	if f.testGitRoot != "" {
		return f.testGitRoot, nil
	}

	// Check environment variable for test isolation.
	//nolint:forbidigo // TEST_GIT_ROOT is specifically for test isolation, not application configuration.
	if testGitRoot := os.Getenv("TEST_GIT_ROOT"); testGitRoot != "" {
		return testGitRoot, nil
	}

	// Find the git root path.
	rootPath, err := f.findGitRoot(execCtx)
	if err != nil {
		return f.handleError(defaultValue, err)
	}

	return rootPath, nil
}

// findGitRoot locates the git repository root from the current context.
func (f *GitRootFunction) findGitRoot(execCtx *ExecutionContext) (string, error) {
	// Get starting path.
	startPath, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current working directory: %w", err)
	}

	// Override with execution context working directory if available.
	if execCtx != nil && execCtx.WorkingDir != "" {
		startPath = execCtx.WorkingDir
	}

	// Open the repository.
	repo, err := git.PlainOpenWithOptions(startPath, &git.PlainOpenOptions{
		DetectDotGit: true,
	})
	if err != nil {
		return "", fmt.Errorf("failed to open Git repository: %w", err)
	}

	// Get the worktree.
	worktree, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %w", err)
	}

	// Return the absolute path to the root directory.
	rootPath, err := filepath.Abs(worktree.Filesystem.Root())
	if err != nil {
		return "", fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	return rootPath, nil
}

// handleError returns the default value if provided, otherwise wraps the error.
func (f *GitRootFunction) handleError(defaultValue string, err error) (any, error) {
	if defaultValue != "" {
		return defaultValue, nil
	}
	return "", fmt.Errorf("%w: %w", ErrExecutionFailed, err)
}
