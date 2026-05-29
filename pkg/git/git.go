package git

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	giturl "github.com/kubescape/go-git-url"

	errUtils "github.com/cloudposse/atmos/errors"
)

// scpLikeURLPattern matches scp-style SSH Git URLs: `[user@]host:path`.
// Example: `git@gitlab.example.com:group/sub/repo.git`
//
// The leading user@ is required so the pattern doesn't false-match relative
// filesystem paths like `dir:file`.
var scpLikeURLPattern = regexp.MustCompile(`^[^@/]+@([^:/]+):(.+?)(?:\.git)?$`)

// ParseGenericGitURL is a fallback parser used when the canonical parser
// (github.com/kubescape/go-git-url) rejects a URL because the host is not
// one of the few it ships hardcoded support for (github.com, gitlab.com,
// azure DevOps). It handles the URL shapes Git itself supports:
//
//   - http(s)://[user[:pass]@]host[:port]/owner/repo[.git]
//   - ssh://[user@]host[:port]/owner/repo[.git]
//   - [user@]host:owner/repo[.git]                   (scp-style)
//
// On success returns (host, owner, name, true). On unrecognized shape returns
// ("", "", "", false) — the caller should treat this as a parse error and
// surface the original kubescape error to preserve existing semantics.
//
// `owner` is the first path segment and `name` is the remainder (with any
// trailing `.git` stripped). For GitLab-style nested groups
// (`group/subgroup/repo.git`) this assigns the top-level group as owner and
// `subgroup/repo` as name, matching kubescape's behavior.
func ParseGenericGitURL(repoUrl string) (host, owner, name string, ok bool) {
	if u, err := url.Parse(repoUrl); err == nil && u.Scheme != "" && u.Host != "" {
		host = u.Hostname()
		path := strings.TrimPrefix(u.Path, "/")
		path = strings.TrimSuffix(path, ".git")
		if i := strings.Index(path, "/"); i > 0 {
			return host, path[:i], path[i+1:], true
		}
	}
	if m := scpLikeURLPattern.FindStringSubmatch(repoUrl); m != nil {
		host = m[1]
		path := strings.TrimSuffix(m[2], ".git")
		if i := strings.Index(path, "/"); i > 0 {
			return host, path[:i], path[i+1:], true
		}
	}
	return "", "", "", false
}

func GetLocalRepo() (*git.Repository, error) {
	localPath := "."

	localRepo, err := git.PlainOpenWithOptions(localPath, &git.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: true, // Enable worktree support
	})
	if err != nil {
		return nil, err
	}

	return localRepo, nil
}

func GetRepoConfig(repo *git.Repository) (*config.Config, error) {
	repoConfig, err := repo.Config()
	if err != nil {
		return nil, err
	}

	core := repoConfig.Raw.Section("core")

	// Remove the untrackedCache option if it exists
	if coreOption := core.Option("untrackedCache"); coreOption != "" {
		core.RemoveOption("untrackedCache")

		// Save the updated configuration
		err = repo.Storer.SetConfig(repoConfig)
		if err != nil {
			return nil, err
		}
	}

	return repoConfig, nil
}

type RepoInfo struct {
	LocalRepoPath     string
	LocalWorktree     *git.Worktree
	LocalWorktreePath string
	RepoUrl           string
	RepoOwner         string
	RepoName          string
	RepoHost          string
}

func GetRepoInfo(localRepo *git.Repository) (RepoInfo, error) {
	localRepoConfig, err := GetRepoConfig(localRepo)
	if err != nil {
		return RepoInfo{}, err
	}

	localRepoWorktree, err := localRepo.Worktree()
	if err != nil {
		return RepoInfo{}, err
	}

	localRepoPath := localRepoWorktree.Filesystem.Root()

	// Get the remotes of the local repo
	keys := []string{}
	for k := range localRepoConfig.Remotes {
		keys = append(keys, k)
	}

	if len(keys) == 0 {
		return RepoInfo{}, nil
	}

	// Get the URL of the repo
	remoteUrls := localRepoConfig.Remotes[keys[0]].URLs
	if len(remoteUrls) == 0 {
		return RepoInfo{}, nil
	}

	repoUrl := remoteUrls[0]
	if repoUrl == "" {
		return RepoInfo{}, nil
	}

	// kubescape/go-git-url ships hardcoded support for github.com,
	// gitlab.com, and azure DevOps. Self-hosted instances (GitHub
	// Enterprise Server, GitLab self-managed, Bitbucket Server, etc.)
	// produce parse errors here. When that happens we fall through to
	// a generic parser that handles the URL shapes Git itself supports.
	// If both fail, surface the canonical error so genuinely-malformed
	// URLs still propagate as before.
	if gitURL, gerr := giturl.NewGitURL(repoUrl); gerr == nil {
		return RepoInfo{
			LocalRepoPath:     localRepoPath,
			LocalWorktree:     localRepoWorktree,
			LocalWorktreePath: localRepoWorktree.Filesystem.Root(),
			RepoUrl:           repoUrl,
			RepoOwner:         gitURL.GetOwnerName(),
			RepoName:          gitURL.GetRepoName(),
			RepoHost:          gitURL.GetHostName(),
		}, nil
	} else if host, owner, name, ok := ParseGenericGitURL(repoUrl); ok {
		return RepoInfo{
			LocalRepoPath:     localRepoPath,
			LocalWorktree:     localRepoWorktree,
			LocalWorktreePath: localRepoWorktree.Filesystem.Root(),
			RepoUrl:           repoUrl,
			RepoOwner:         owner,
			RepoName:          name,
			RepoHost:          host,
		}, nil
	} else {
		return RepoInfo{}, gerr
	}
}

// GitRepoInterface defines the interface for git repository operations.
type GitRepoInterface interface {
	GetLocalRepoInfo() (*RepoInfo, error)
	GetRepoInfo(repo *git.Repository) (RepoInfo, error)
	GetCurrentCommitSHA() (string, error)
}

// DefaultGitRepo is the default implementation of GitRepoInterface.
type DefaultGitRepo struct{}

// NewDefaultGitRepo creates a new instance of DefaultGitRepo.
func NewDefaultGitRepo() GitRepoInterface {
	return &DefaultGitRepo{}
}

// GetLocalRepoInfo returns information about the local git repository.
func (d *DefaultGitRepo) GetLocalRepoInfo() (*RepoInfo, error) {
	repo, err := GetLocalRepo()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrFailedToGetLocalRepo, err)
	}
	info, err := GetRepoInfo(repo)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrFailedToGetRepoInfo, err)
	}
	return &info, nil
}

// GetRepoInfo returns the repository information for the given git.Repository.
func (d *DefaultGitRepo) GetRepoInfo(repo *git.Repository) (RepoInfo, error) {
	info, err := GetRepoInfo(repo)
	if err != nil {
		// Get repository path for context
		var repoPath string
		if worktree, worktreeErr := repo.Worktree(); worktreeErr == nil {
			repoPath = worktree.Filesystem.Root()
		} else {
			repoPath = "unknown"
		}
		cause := fmt.Errorf("GetRepoInfo failed for repo %s: %w", repoPath, err)
		return RepoInfo{}, errors.Join(errUtils.ErrFailedToGetRepoInfo, cause)
	}
	return info, nil
}

// GetCurrentCommitSHA returns the SHA of the current HEAD commit.
func (d *DefaultGitRepo) GetCurrentCommitSHA() (string, error) {
	repo, err := GetLocalRepo()
	if err != nil {
		return "", fmt.Errorf("%w: failed to get local repository: %s", errUtils.ErrLocalRepoFetch, err)
	}

	ref, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("%w: failed to get HEAD reference: %s", errUtils.ErrHeadLookup, err)
	}

	return ref.Hash().String(), nil
}

// GetCurrentCommitSHA returns the SHA of the current HEAD commit.
func GetCurrentCommitSHA() (string, error) {
	return NewDefaultGitRepo().GetCurrentCommitSHA()
}

// GetCurrentBranch returns the current Git branch name.
func GetCurrentBranch() (string, error) {
	repo, err := GetLocalRepo()
	if err != nil {
		return "", fmt.Errorf("%w: failed to get local repository: %w", errUtils.ErrLocalRepoFetch, err)
	}

	ref, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("%w: failed to get HEAD reference: %w", errUtils.ErrHeadLookup, err)
	}

	if !ref.Name().IsBranch() {
		return "", errUtils.ErrDetachedHead
	}

	branch := ref.Name().Short()
	if branch == "" {
		return "", errUtils.ErrEmptyBranchName
	}

	return branch, nil
}

// GetRoot returns the absolute root path of the current Git worktree.
func GetRoot() (string, error) {
	repo, err := GetLocalRepo()
	if err != nil {
		return "", fmt.Errorf("%w: failed to get local repository: %w", errUtils.ErrLocalRepoFetch, err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrGitWorktree, err)
	}

	rootPath, err := filepath.Abs(worktree.Filesystem.Root())
	if err != nil {
		return "", fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrPathResolution, err)
	}

	return rootPath, nil
}

// OpenWorktreeAwareRepo opens a Git repository at the given path,
// handling both regular repositories and worktrees correctly.
// It uses EnableDotGitCommonDir to properly support worktrees with
// access to the main repository's config, remotes, and references.
func OpenWorktreeAwareRepo(path string) (*git.Repository, error) {
	// Always try with EnableDotGitCommonDir first
	// This works for both regular repos and worktrees
	repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{
		DetectDotGit:          false, // We want exact path, not parent search
		EnableDotGitCommonDir: true,  // Enable worktree support for config/remotes
	})

	return repo, err
}
