package planfile

import (
	"fmt"
	"os"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	"github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// resolvedContext holds SHA and branch resolved from environment or git.
type resolvedContext struct {
	SHA    string
	Branch string
}

// resolveContext resolves CI context (SHA, branch) from environment or git.
// Returns empty SHA if allFlag is true (meaning no SHA filter).
// SHA resolution order: ATMOS_CI_SHA → GIT_COMMIT → CI_COMMIT_SHA → COMMIT_SHA → git HEAD.
func resolveContext(allFlag bool) (*resolvedContext, error) {
	defer perf.Track(nil, "planfile.resolveContext")()

	if allFlag {
		return &resolvedContext{}, nil
	}

	sha := getFirstEnvValue("ATMOS_CI_SHA", "GIT_COMMIT", "CI_COMMIT_SHA", "COMMIT_SHA")
	if sha == "" {
		// Fall back to git HEAD.
		gitRepo := git.NewDefaultGitRepo()
		var err error
		sha, err = gitRepo.GetCurrentCommitSHA()
		if err != nil {
			log.Debug("Failed to get git HEAD SHA", "error", err)
			return nil, fmt.Errorf("%w: could not resolve SHA from environment or git: %w", errUtils.ErrPlanfileStoreInvalidArgs, err)
		}
	}

	branch := getFirstEnvValue("ATMOS_CI_BRANCH", "GIT_BRANCH", "CI_COMMIT_REF_NAME", "BRANCH_NAME")

	return &resolvedContext{
		SHA:    sha,
		Branch: branch,
	}, nil
}

// resolveKey generates the storage key from component, stack, and SHA.
func resolveKey(component, stack, sha string) (string, error) {
	defer perf.Track(nil, "planfile.resolveKey")()

	keyPattern := planfile.DefaultKeyPattern()
	key, err := keyPattern.GenerateKey(&planfile.KeyContext{
		Stack:     stack,
		Component: component,
		SHA:       sha,
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate planfile key: %w", err)
	}
	return key, nil
}

// buildQuery constructs a query from optional filter dimensions.
// Empty values are omitted from the query. If all are empty and allFlag is true,
// returns a query that matches all planfiles.
func buildQuery(component, stack, sha string) planfile.Query {
	defer perf.Track(nil, "planfile.buildQuery")()

	q := planfile.Query{}

	if component != "" {
		q.Components = []string{component}
	}
	if stack != "" {
		q.Stacks = []string{stack}
	}
	if sha != "" {
		q.SHAs = []string{sha}
	}

	// If no filters are set, match all.
	if component == "" && stack == "" && sha == "" {
		q.All = true
	}

	return q
}

// getFirstEnvValue returns the value of the first set environment variable.
func getFirstEnvValue(keys ...string) string {
	for _, key := range keys {
		if value := os.Getenv(key); value != "" {
			return value
		}
	}
	return ""
}
