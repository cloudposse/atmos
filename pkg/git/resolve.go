package git

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const (
	// DefaultProviderName is the universal, host-agnostic execution backend.
	DefaultProviderName = "cli"
	// DefaultRemote is the default Git remote name.
	DefaultRemote = "origin"
	// DefaultPushRetries bounds the push retry loop on non-fast-forward rejection.
	DefaultPushRetries = 3
	// Permission for automatically created workdir parents.
	workdirPerm = 0o755
)

// ResolvedRepository is a managed repository with all defaults applied.
type ResolvedRepository struct {
	Name        string
	URI         string
	Provider    string
	Branch      string
	Remote      string
	Workdir     string
	Clone       schema.GitCloneConfig
	Identity    string
	Signing     SigningMode
	Author      *Author
	PushRetries int
}

// ResolveRepository looks up a named repository under git.repositories and
// applies defaults (provider=cli, remote=origin, signing=auto, retries=3,
// automatic XDG workdir).
func ResolveRepository(cfg *schema.GitConfig, name string) (*ResolvedRepository, error) {
	defer perf.Track(nil, "git.ResolveRepository")()

	repo, ok := lookupRepository(cfg, name)
	if !ok {
		return nil, fmt.Errorf("%w: %q (configured: %s)", errUtils.ErrGitRepositoryNotFound, name, strings.Join(ConfiguredRepositoryNames(cfg), ", "))
	}

	workdir := repo.Workdir
	if workdir == "" {
		var err error
		workdir, err = DefaultWorkdir(name)
		if err != nil {
			return nil, err
		}
	}

	resolved := &ResolvedRepository{
		Name:        name,
		URI:         repo.URI,
		Provider:    stringOrDefault(repo.Provider, DefaultProviderName),
		Branch:      repo.Branch,
		Remote:      stringOrDefault(repo.Remote, DefaultRemote),
		Workdir:     workdir,
		Clone:       repo.Clone,
		Identity:    repo.Auth.Identity,
		Signing:     SigningMode(stringOrDefault(repo.Commit.Signing, string(SigningAuto))),
		Author:      resolveAuthor(repo.Commit.Author),
		PushRetries: resolveRetries(repo.Push.Retries),
	}

	return resolved, nil
}

// ConfiguredRepositoryNames returns the sorted logical names under git.repositories.
func ConfiguredRepositoryNames(cfg *schema.GitConfig) []string {
	if cfg == nil {
		return nil
	}
	names := make([]string, 0, len(cfg.Repositories))
	for name := range cfg.Repositories {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// DefaultWorkdir resolves the automatic XDG workdir for a named repository:
// $XDG_CACHE_HOME/atmos/git/repositories/<name>. Placing workdirs under the
// XDG cache root lets the native CI cache capture and restore managed clones;
// clone reconciles restored (possibly stale) workdirs.
func DefaultWorkdir(name string) (string, error) {
	defer perf.Track(nil, "git.DefaultWorkdir")()

	dir, err := xdg.GetXDGCacheDir(filepath.Join("git", "repositories", name), workdirPerm)
	if err != nil {
		return "", fmt.Errorf("resolving automatic workdir for git repository %q: %w", name, err)
	}
	return dir, nil
}

// ValidateRepoRelativePath ensures a repo-relative path stays inside the
// worktree after cleaning, returning its absolute path. Path traversal out of
// the worktree returns ErrGitPathEscapesWorktree.
func ValidateRepoRelativePath(worktree, rel string) (string, error) {
	defer perf.Track(nil, "git.ValidateRepoRelativePath")()

	absWorktree, err := filepath.Abs(worktree)
	if err != nil {
		return "", fmt.Errorf("resolving worktree path %q: %w", worktree, err)
	}

	abs := filepath.Clean(filepath.Join(absWorktree, rel))
	relToWorktree, err := filepath.Rel(absWorktree, abs)
	if err != nil || relToWorktree == ".." || strings.HasPrefix(relToWorktree, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q", errUtils.ErrGitPathEscapesWorktree, rel)
	}

	return abs, nil
}

// IdentityEnvironmentProvider supplies the composed identity environment for
// an Atmos Auth identity (including linked auto-provision integrations such
// as github/sts, which materializes GIT_CONFIG_* variables).
// The Manager in pkg/auth satisfies this interface.
type IdentityEnvironmentProvider interface {
	EnsureIdentityEnvironment(ctx context.Context, identityName string) (map[string]string, error)
}

// ComposeEnvironment merges the identity environment for the given identity
// over the base environment list. An empty identity returns base unchanged
// (ambient credentials and the developer's own Git config still apply).
func ComposeEnvironment(ctx context.Context, base []string, identity string, envProvider IdentityEnvironmentProvider) ([]string, error) {
	defer perf.Track(nil, "git.ComposeEnvironment")()

	if identity == "" || envProvider == nil {
		return base, nil
	}

	identityEnv, err := envProvider.EnsureIdentityEnvironment(ctx, identity)
	if err != nil {
		return nil, fmt.Errorf("composing identity environment for %q: %w", identity, err)
	}

	return mergeEnv(base, identityEnv), nil
}

// mergeEnv overlays kv pairs onto an environment list, replacing existing keys.
func mergeEnv(base []string, overlay map[string]string) []string {
	if len(overlay) == 0 {
		return base
	}

	merged := make([]string, 0, len(base)+len(overlay))
	for _, entry := range base {
		key, _, found := strings.Cut(entry, "=")
		if found {
			if _, ok := overlay[key]; ok {
				continue
			}
		}
		merged = append(merged, entry)
	}

	keys := make([]string, 0, len(overlay))
	for key := range overlay {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		merged = append(merged, key+"="+overlay[key])
	}

	return merged
}

// lookupRepository finds a repository by name in the config.
func lookupRepository(cfg *schema.GitConfig, name string) (schema.GitRepository, bool) {
	if cfg == nil {
		return schema.GitRepository{}, false
	}
	repo, ok := cfg.Repositories[name]
	return repo, ok
}

// resolveAuthor converts a schema author into the provider type; an empty
// author resolves to nil so Git's own config applies for local use.
func resolveAuthor(author schema.GitAuthorConfig) *Author {
	if author.Name == "" && author.Email == "" {
		return nil
	}
	return &Author{Name: author.Name, Email: author.Email}
}

// resolveRetries applies the default push retry count.
func resolveRetries(retries *int) int {
	if retries == nil {
		return DefaultPushRetries
	}
	if *retries < 0 {
		return 0
	}
	return *retries
}

// stringOrDefault returns s, or def when s is empty.
func stringOrDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
