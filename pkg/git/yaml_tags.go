package git

import (
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	YAMLFuncRepoRoot   = "!repo-root"
	YAMLFuncRoot       = "!git.root"
	YAMLFuncSHA        = "!git.sha"
	YAMLFuncBranch     = "!git.branch"
	YAMLFuncRef        = "!git.ref"
	YAMLFuncRepository = "!git.repository"
	YAMLFuncOwner      = "!git.owner"
	YAMLFuncName       = "!git.name"
	YAMLFuncHost       = "!git.host"
	YAMLFuncURL        = "!git.url"

	// The repositorySeparator joins the owner and repo name into the `<owner>/<repo>` slug.
	repositorySeparator = "/"
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

// processRepoInfoTag resolves a repository-metadata tag by extracting a field from
// the local repository info. It follows the same default-on-error / empty-fallback
// behavior as the other Git tags: when the value cannot be resolved (or is empty),
// the trailing default value is returned if provided, otherwise an error is returned.
func processRepoInfoTag(input string, extract func(RepoInfo) string, tags ...string) (string, error) {
	defaultValue := strings.TrimSpace(trimTagPrefix(input, tags...))

	info, err := NewDefaultGitRepo().GetLocalRepoInfo()
	if err != nil {
		if defaultValue != "" {
			log.Debug("failed to resolve Git repository info, returning default value", "error", err)
			return defaultValue, nil
		}
		return "", err
	}

	value := extract(*info)
	if value == "" {
		if defaultValue != "" {
			log.Debug("Git repository info field is empty, returning default value", "tags", tags)
			return defaultValue, nil
		}
		return "", errUtils.ErrFailedToGetRepoInfo
	}

	return value, nil
}

// ProcessTagRepository returns the `<owner>/<repo>` slug for the !git.repository tag.
func ProcessTagRepository(input string) (string, error) {
	defer perf.Track(nil, "git.ProcessTagRepository")()

	return processRepoInfoTag(input, func(info RepoInfo) string {
		if info.RepoOwner == "" || info.RepoName == "" {
			return ""
		}
		return info.RepoOwner + repositorySeparator + info.RepoName
	}, YAMLFuncRepository)
}

// ProcessTagOwner returns the repository owner/org for the !git.owner tag.
func ProcessTagOwner(input string) (string, error) {
	defer perf.Track(nil, "git.ProcessTagOwner")()

	return processRepoInfoTag(input, func(info RepoInfo) string {
		return info.RepoOwner
	}, YAMLFuncOwner)
}

// ProcessTagName returns the bare repository name for the !git.name tag.
func ProcessTagName(input string) (string, error) {
	defer perf.Track(nil, "git.ProcessTagName")()

	return processRepoInfoTag(input, func(info RepoInfo) string {
		return info.RepoName
	}, YAMLFuncName)
}

// ProcessTagHost returns the repository host (e.g. `github.com`) for the !git.host tag.
func ProcessTagHost(input string) (string, error) {
	defer perf.Track(nil, "git.ProcessTagHost")()

	return processRepoInfoTag(input, func(info RepoInfo) string {
		return info.RepoHost
	}, YAMLFuncHost)
}

// ProcessTagURL returns the repository remote URL for the !git.url tag.
func ProcessTagURL(input string) (string, error) {
	defer perf.Track(nil, "git.ProcessTagURL")()

	return processRepoInfoTag(input, func(info RepoInfo) string {
		return info.RepoUrl
	}, YAMLFuncURL)
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
