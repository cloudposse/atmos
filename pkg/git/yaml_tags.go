package git

import (
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	YAMLFuncRepoRoot = "!repo-root"
	YAMLFuncRoot     = "!git.root"
	YAMLFuncSHA      = "!git.sha"
	YAMLFuncBranch   = "!git.branch"
	YAMLFuncRef      = "!git.ref"
)

// ProcessTagRoot returns the current Git worktree root for !repo-root and !git.root tags.
func ProcessTagRoot(input string) (string, error) {
	defer perf.Track(nil, "git.ProcessTagRoot")()

	defaultValue := strings.TrimSpace(trimTagPrefix(input, YAMLFuncRepoRoot, YAMLFuncRoot))

	rootPath, err := GetRoot()
	if err != nil {
		if defaultValue != "" {
			log.Debug("failed to resolve Git root, returning default value", "error", err)
			return defaultValue, nil
		}
		return "", err
	}
	return rootPath, nil
}

// ProcessTagSHA returns the current Git HEAD commit SHA for !git.sha and !git.ref tags.
func ProcessTagSHA(input string) (string, error) {
	defer perf.Track(nil, "git.ProcessTagSHA")()

	defaultValue := strings.TrimSpace(trimTagPrefix(input, YAMLFuncSHA, YAMLFuncRef))

	hash, err := GetCurrentCommitSHA()
	if err != nil {
		if defaultValue != "" {
			log.Debug("failed to resolve Git SHA, returning default value", "error", err)
			return defaultValue, nil
		}
		return "", err
	}
	return hash, nil
}

// ProcessTagRef returns the immutable Git ref used for source pinning.
// It currently aliases ProcessTagSHA.
func ProcessTagRef(input string) (string, error) {
	defer perf.Track(nil, "git.ProcessTagRef")()

	return ProcessTagSHA(input)
}

// ProcessTagBranch returns the current Git branch name for !git.branch tags.
func ProcessTagBranch(input string) (string, error) {
	defer perf.Track(nil, "git.ProcessTagBranch")()

	defaultValue := strings.TrimSpace(trimTagPrefix(input, YAMLFuncBranch))

	branch, err := GetCurrentBranch()
	if err != nil {
		if defaultValue != "" {
			log.Debug("failed to resolve Git branch, returning default value", "error", err)
			return defaultValue, nil
		}
		return "", err
	}

	return branch, nil
}

func trimTagPrefix(input string, tags ...string) string {
	trimmed := strings.TrimSpace(input)
	for _, tag := range tags {
		if strings.HasPrefix(trimmed, tag) {
			return strings.TrimPrefix(trimmed, tag)
		}
	}
	return input
}
