package cli

import (
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
)

// authPatterns identify authentication failures in git stderr.
// Matching is case-insensitive.
var authPatterns = []string{
	"authentication failed",
	"could not read username",
	"could not read password",
	"permission denied",
	"access denied",
	"invalid username or password",
	"403",
	"401",
}

// rejectedPatterns identify non-fast-forward push rejections in git stderr.
var rejectedPatterns = []string{
	"non-fast-forward",
	"fetch first",
	"[rejected]",
}

// classify translates a raw runner error into a named sentinel using the
// bounded stderr tail. The tail itself is never embedded in the returned
// error: it may contain secrets and bypasses writer-level masking.
func classify(err error, result atmosgit.RunResult, op string) error {
	if err == nil {
		return nil
	}
	if matchesAny(result.StderrTail, authPatterns) {
		return fmt.Errorf("%w: during git %s: %w", errUtils.ErrGitAuthFailed, op, err)
	}
	return err
}

// isRejectedPush reports whether stderr indicates a non-fast-forward rejection.
func isRejectedPush(result atmosgit.RunResult) bool {
	return matchesAny(result.StderrTail, rejectedPatterns)
}

// matchesAny reports whether s contains any pattern (case-insensitive).
func matchesAny(s string, patterns []string) bool {
	lower := strings.ToLower(s)
	for _, pattern := range patterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return false
}
