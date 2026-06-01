package git

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing/cache"
	formatconfig "github.com/go-git/go-git/v5/plumbing/format/config"
	"github.com/go-git/go-git/v5/storage"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/go-git/go-git/v5/storage/filesystem/dotgit"
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
		return openWorktreeConfigTolerantRepo(localPath, err)
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
	if err != nil {
		return openWorktreeConfigTolerantRepo(path, err)
	}

	return repo, nil
}

// worktreeConfigTolerantStorer strips unsupported worktree config extensions
// when reading repository config through go-git.
type worktreeConfigTolerantStorer struct {
	storage.Storer
}

// Config returns repository config after removing unsupported worktreeConfig metadata.
func (s worktreeConfigTolerantStorer) Config() (*config.Config, error) {
	cfg, err := s.Storer.Config()
	if err != nil {
		return nil, err
	}

	cfgCopy, err := cloneGitConfig(cfg)
	if err != nil {
		return nil, err
	}
	removeWorktreeConfigExtension(cfgCopy)
	return cfgCopy, nil
}

// SetConfig preserves worktreeConfig when callers persist a sanitized config.
func (s worktreeConfigTolerantStorer) SetConfig(cfg *config.Config) error {
	if cfg == nil {
		return s.Storer.SetConfig(cfg)
	}

	current, err := s.Storer.Config()
	if err != nil {
		return err
	}
	worktreeConfigValue, preserveWorktreeConfig := worktreeConfigExtensionValue(current)

	cfgCopy, err := cloneGitConfig(cfg)
	if err != nil {
		return err
	}
	if preserveWorktreeConfig && !hasWorktreeConfigExtension(cfgCopy) {
		if cfgCopy.Raw == nil {
			cfgCopy.Raw = formatconfig.New()
		}
		cfgCopy.Raw.SetOption("extensions", formatconfig.NoSubsection, "worktreeConfig", worktreeConfigValue)
	}

	return s.Storer.SetConfig(cfgCopy)
}

// openWorktreeConfigTolerantRepo retries repository open for worktrees using worktreeConfig.
func openWorktreeConfigTolerantRepo(path string, originalErr error) (*git.Repository, error) {
	if !isUnsupportedWorktreeConfigError(originalErr) {
		return nil, originalErr
	}

	repoRoot, gitDir, commonDir, err := gitRepositoryPaths(path)
	if err != nil {
		return nil, errors.Join(err, originalErr)
	}

	dotGitFs := osfs.New(gitDir)
	repositoryFs := dotGitFs
	if commonDir != "" && commonDir != gitDir {
		repositoryFs = dotgit.NewRepositoryFilesystem(dotGitFs, osfs.New(commonDir))
	}

	storer := filesystem.NewStorage(repositoryFs, cache.NewObjectLRUDefault())
	return git.Open(worktreeConfigTolerantStorer{Storer: storer}, osfs.New(repoRoot))
}

// isUnsupportedWorktreeConfigError reports go-git failures caused by worktreeConfig.
func isUnsupportedWorktreeConfigError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "repositoryformatversion") && strings.Contains(msg, "worktreeconfig")
}

// removeWorktreeConfigExtension removes the worktreeConfig extension from config.
func removeWorktreeConfigExtension(cfg *config.Config) {
	if cfg == nil || cfg.Raw == nil || !cfg.Raw.HasSection("extensions") {
		return
	}

	section := cfg.Raw.Section("extensions")
	keys := make([]string, 0, len(section.Options))
	for _, opt := range section.Options {
		if strings.EqualFold(opt.Key, "worktreeConfig") {
			keys = append(keys, opt.Key)
		}
	}
	for _, key := range keys {
		section.RemoveOption(key)
	}
}

func cloneGitConfig(cfg *config.Config) (*config.Config, error) {
	if cfg == nil {
		return nil, nil
	}

	cfgCopy := *cfg
	if cfg.Raw == nil {
		cfgCopy.Raw = nil
		return &cfgCopy, nil
	}

	var buf bytes.Buffer
	if err := formatconfig.NewEncoder(&buf).Encode(cfg.Raw); err != nil {
		return nil, err
	}
	rawCopy := formatconfig.New()
	if err := formatconfig.NewDecoder(&buf).Decode(rawCopy); err != nil {
		return nil, err
	}
	cfgCopy.Raw = rawCopy
	return &cfgCopy, nil
}

func hasWorktreeConfigExtension(cfg *config.Config) bool {
	_, ok := worktreeConfigExtensionValue(cfg)
	return ok
}

func worktreeConfigExtensionValue(cfg *config.Config) (string, bool) {
	if cfg == nil || cfg.Raw == nil || !cfg.Raw.HasSection("extensions") {
		return "", false
	}

	section := cfg.Raw.Section("extensions")
	for _, opt := range section.Options {
		if strings.EqualFold(opt.Key, "worktreeConfig") {
			return opt.Value, true
		}
	}
	return "", false
}

// gitRepositoryPaths returns the repository root, git dir, and common dir for path.
func gitRepositoryPaths(path string) (repoRoot, gitDir, commonDir string, err error) {
	out, err := exec.Command("git", "-C", path, "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-dir", "--git-common-dir").Output()
	if err != nil {
		return "", "", "", err
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 3 {
		return "", "", "", fmt.Errorf("%w: got %d lines: %q", errUtils.ErrUnexpectedGitRevParseOutput, len(lines), strings.TrimSpace(string(out)))
	}

	repoRoot = filepath.Clean(lines[0])
	gitDir = filepath.Clean(lines[1])
	commonDir = filepath.Clean(lines[2])
	return repoRoot, gitDir, commonDir, nil
}
