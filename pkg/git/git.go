package git

import (
	"os"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	giturl "github.com/kubescape/go-git-url"
)

func GetLocalRepo() (*git.Repository, error) {
	localPath := "."

	localRepo, err := git.PlainOpenWithOptions(localPath, &git.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: false,
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

// OpenWorktreeAwareRepo opens a Git repository at the given path,
// handling both regular repositories and worktrees correctly.
// It first tries to open with DetectDotGit: false for exact path matching,
// then falls back to worktree-aware options if needed.
func OpenWorktreeAwareRepo(path string) (*git.Repository, error) {
	// First try to open with DetectDotGit: false for exact path
	repo, err := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{
		DetectDotGit:          false,
		EnableDotGitCommonDir: false,
	})
	if err == nil {
		return repo, nil
	}

	// Check if there's a .git file (worktree) at the path
	gitPath := filepath.Join(path, ".git")
	info, statErr := os.Stat(gitPath)

	if statErr == nil && !info.IsDir() {
		// It's a .git file (worktree)
		// For worktrees, go-git has issues with config reading
		// Try with EnableDotGitCommonDir which helps with worktree support
		repo, worktreeErr := git.PlainOpenWithOptions(path, &git.PlainOpenOptions{
			DetectDotGit:          true, // Let it detect the .git file
			EnableDotGitCommonDir: true, // Enable worktree support
		})
		if worktreeErr == nil {
			return repo, nil
		}

		// If that didn't work, try just opening with basic support
		repo, basicErr := git.PlainOpen(path)
		if basicErr == nil {
			return repo, nil
		}
	}

	// Return the original error
	return nil, err
}
