package errors

import "errors"

// ErrGitNothingStaged distinguishes a dirty worktree from a clean no-op commit.
var ErrGitNothingStaged = errors.New("working tree has changes, but no changes are staged for commit")
