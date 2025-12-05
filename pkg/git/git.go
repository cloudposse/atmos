package git

import (
	"errors"
	"fmt"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	giturl "github.com/kubescape/go-git-url"

	errUtils "github.com/cloudposse/atmos/errors"
)

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

	gitURL, err := giturl.NewGitURL(repoUrl)
	if err != nil {
		return RepoInfo{}, err
	}

	response := RepoInfo{
		LocalRepoPath:     localRepoPath,
		LocalWorktree:     localRepoWorktree,
		LocalWorktreePath: localRepoWorktree.Filesystem.Root(),
		RepoUrl:           repoUrl,
		RepoOwner:         gitURL.GetOwnerName(),
		RepoName:          gitURL.GetRepoName(),
		RepoHost:          gitURL.GetHostName(),
	}

	return response, nil
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
		return nil, fmt.Errorf("%w: failed to get local repository: %w", errUtils.ErrFailedToGetLocalRepo, err)
	}
	info, err := GetRepoInfo(repo)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to get repository info: %w", errUtils.ErrFailedToGetRepoInfo, err)
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
		return "", fmt.Errorf("%w: failed to get local repository: %w", errUtils.ErrLocalRepoFetch, err)
	}

	ref, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("%w: failed to get HEAD reference: %w", errUtils.ErrHeadLookup, err)
	}

	return ref.Hash().String(), nil
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
