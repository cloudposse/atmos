// Package git implements the "git" provision target: it publishes a rendered
// ProvisionArtifact to a managed Git deployment repository by writing the files
// under a configured path, committing the scoped path, and pushing.
//
// It is a thin delivery layer over the reusable pkg/git service (clone-reconcile,
// path-scoped commit, push-with-retry) and never renders artifacts itself.
package git

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/target"

	// Blank import registers the "cli" git provider (the only v1 backend) so
	// atmosgit.NewProvider("cli") resolves wherever the git target is compiled in.
	_ "github.com/cloudposse/atmos/pkg/git/providers/cli"
)

const (
	dirPerm  = 0o755
	filePerm = 0o600
)

func init() {
	target.Register(target.KindGit, &gitProvisioner{})
}

// gitProvisioner publishes artifacts to a managed Git repository.
type gitProvisioner struct{}

// config is the parsed provision.targets.<name> block for a git target.
type config struct {
	Repository    string
	Path          string
	Identity      string
	CommitMessage string
	Signing       string
	PullRequest   bool
}

// repoSession bundles the resolved repository and its execution context for a
// single delivery, so helpers stay within the argument limit.
type repoSession struct {
	provider atmosgit.Provider
	rc       atmosgit.RepoContext
	resolved *atmosgit.ResolvedRepository
}

// Deliver writes the artifact files into the deployment repository's configured
// path, commits the scoped path, and pushes. Clone is reconcile (clone-if-absent,
// otherwise fetch + fast-forward). A no-op (no changes) is not an error.
func (g *gitProvisioner) Deliver(ctx context.Context, in *target.DeliverInput) error {
	defer perf.Track(in.AtmosConfig, "target.git.Deliver")()

	cfg := parseConfig(in.TargetConfig)

	if cfg.PullRequest {
		return fmt.Errorf("%w: target %q", errUtils.ErrGitPullRequestNotSupported, in.TargetName)
	}

	resolved, err := atmosgit.ResolveRepository(&in.AtmosConfig.Git, cfg.Repository)
	if err != nil {
		return err
	}

	env, err := atmosgit.ComposeEnvironment(ctx, os.Environ(), identityFor(&cfg, resolved), in.EnvProvider)
	if err != nil {
		return err
	}

	provider, err := atmosgit.NewProvider(resolved.Provider)
	if err != nil {
		return err
	}

	session := &repoSession{
		provider: provider,
		rc: atmosgit.RepoContext{
			Workdir: resolved.Workdir,
			Remote:  resolved.Remote,
			Branch:  resolved.Branch,
			Env:     env,
		},
		resolved: resolved,
	}

	if err := reconcile(ctx, session); err != nil {
		return err
	}

	if err := writeArtifact(resolved.Workdir, cfg.Path, &in.Artifact); err != nil {
		return err
	}

	return commitAndPush(ctx, session, &cfg, &in.Artifact)
}

// Fetch reconciles the deployment repository and reads the files currently
// committed under the configured path, so a producer can diff a fresh render
// against the live GitOps state. It never writes, commits, or pushes. A path that
// does not exist yet yields an artifact with no Files (an empty baseline).
func (g *gitProvisioner) Fetch(ctx context.Context, in *target.FetchInput) (target.ProvisionArtifact, error) {
	defer perf.Track(in.AtmosConfig, "target.git.Fetch")()

	cfg := parseConfig(in.TargetConfig)

	resolved, err := atmosgit.ResolveRepository(&in.AtmosConfig.Git, cfg.Repository)
	if err != nil {
		return target.ProvisionArtifact{}, err
	}

	env, err := atmosgit.ComposeEnvironment(ctx, os.Environ(), identityFor(&cfg, resolved), in.EnvProvider)
	if err != nil {
		return target.ProvisionArtifact{}, err
	}

	provider, err := atmosgit.NewProvider(resolved.Provider)
	if err != nil {
		return target.ProvisionArtifact{}, err
	}

	session := &repoSession{
		provider: provider,
		rc: atmosgit.RepoContext{
			Workdir: resolved.Workdir,
			Remote:  resolved.Remote,
			Branch:  resolved.Branch,
			Env:     env,
		},
		resolved: resolved,
	}

	if err := reconcile(ctx, session); err != nil {
		return target.ProvisionArtifact{}, err
	}

	files, err := readManagedTree(resolved.Workdir, cfg.Path)
	if err != nil {
		return target.ProvisionArtifact{}, err
	}

	return target.ProvisionArtifact{
		Kind:   target.ArtifactKindKubernetesManifests,
		Format: target.FormatYAML,
		Files:  files,
		Metadata: target.ArtifactMetadata{
			Target: in.TargetName,
		},
	}, nil
}

// readManagedTree reads every file under <workdir>/<path> into a map keyed by the
// path-relative filename. A missing managed path returns an empty map (the path
// has not been published yet), never an error.
func readManagedTree(workdir, path string) (map[string][]byte, error) {
	absPath, err := atmosgit.ValidateRepoRelativePath(workdir, path)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(absPath)
	if os.IsNotExist(err) {
		return map[string][]byte{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: reading managed path %q: %w", errUtils.ErrGitArtifactRead, path, err)
	}
	if !info.IsDir() {
		data, readErr := os.ReadFile(absPath)
		if readErr != nil {
			return nil, fmt.Errorf("%w: reading %q: %w", errUtils.ErrGitArtifactRead, path, readErr)
		}
		return map[string][]byte{filepath.Base(absPath): data}, nil
	}

	return walkManagedDir(absPath)
}

// walkManagedDir reads every file under root into a map keyed by the
// forward-slash path relative to root.
func walkManagedDir(root string) (map[string][]byte, error) {
	files := map[string][]byte{}
	walkErr := filepath.WalkDir(root, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return relErr
		}
		data, readErr := os.ReadFile(p)
		if readErr != nil {
			return fmt.Errorf("%w: reading %q: %w", errUtils.ErrGitArtifactRead, rel, readErr)
		}
		files[filepath.ToSlash(rel)] = data
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return files, nil
}

// reconcile clones the repository if absent, otherwise fetches and fast-forwards.
func reconcile(ctx context.Context, s *repoSession) error {
	return s.provider.Clone(ctx, &atmosgit.CloneOptions{
		RepoContext:  s.rc,
		URI:          s.resolved.URI,
		Depth:        s.resolved.Clone.Depth,
		Filter:       s.resolved.Clone.Filter,
		SingleBranch: s.resolved.Clone.SingleBranch,
		Submodules:   s.resolved.Clone.Submodules,
	})
}

// commitAndPush stages the managed path, commits any changes, and pushes when a
// commit was created.
func commitAndPush(ctx context.Context, s *repoSession, cfg *config, artifact *target.ProvisionArtifact) error {
	result, err := s.provider.Commit(ctx, &atmosgit.CommitOptions{
		RepoContext: s.rc,
		Message:     cfg.CommitMessage,
		Paths:       []string{cfg.Path},
		Signing:     signingMode(cfg, s.resolved),
		Author:      s.resolved.Author,
		Trailers:    trailers(artifact),
	})
	if err != nil {
		return err
	}
	if !result.Committed {
		// Nothing changed in the managed path; a no-op is a clean success.
		return nil
	}

	return s.provider.Push(ctx, &atmosgit.PushOptions{
		RepoContext: s.rc,
		Retries:     s.resolved.PushRetries,
	})
}

// writeArtifact replaces the managed subtree under <workdir>/<path> with the
// artifact files, so removals propagate deterministically.
func writeArtifact(workdir, path string, artifact *target.ProvisionArtifact) error {
	// Guard against deleting the worktree root: ValidateRepoRelativePath resolves
	// root-equivalent paths ("", ".", "./", "a/..") to the worktree root, and a
	// subsequent os.RemoveAll there would destroy the entire repository (including
	// .git). filepath.Clean normalizes all such variants to "." before the check.
	if trimmed := strings.TrimSpace(path); filepath.Clean(trimmed) == "." {
		return fmt.Errorf("%w: %q", errUtils.ErrGitTargetPathInvalid, path)
	}

	absPath, err := atmosgit.ValidateRepoRelativePath(workdir, path)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(absPath); err != nil {
		return fmt.Errorf("%w: clearing managed path %q: %w", errUtils.ErrGitArtifactWrite, path, err)
	}

	for _, rel := range sortedFileKeys(artifact.Files) {
		repoRel := filepath.Join(path, rel)
		abs, err := atmosgit.ValidateRepoRelativePath(workdir, repoRel)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(abs), dirPerm); err != nil {
			return fmt.Errorf("%w: creating directory for %q: %w", errUtils.ErrGitArtifactWrite, repoRel, err)
		}
		if err := os.WriteFile(abs, artifact.Files[rel], filePerm); err != nil {
			return fmt.Errorf("%w: writing %q: %w", errUtils.ErrGitArtifactWrite, repoRel, err)
		}
	}
	return nil
}

// parseConfig extracts the git target settings from the merged target block.
func parseConfig(block map[string]any) config {
	cfg := config{
		Repository: stringField(block, "repository"),
		Path:       stringField(block, "path"),
	}
	if auth, ok := block["auth"].(map[string]any); ok {
		cfg.Identity = stringField(auth, "identity")
	}
	if commit, ok := block["commit"].(map[string]any); ok {
		cfg.CommitMessage = stringField(commit, "message")
		cfg.Signing = stringField(commit, "signing")
	}
	if pr, ok := block["pull_request"].(map[string]any); ok {
		cfg.PullRequest, _ = pr["enabled"].(bool)
	}
	return cfg
}

// identityFor resolves the auth identity: the target override, else the repository default.
func identityFor(cfg *config, resolved *atmosgit.ResolvedRepository) string {
	if cfg.Identity != "" {
		return cfg.Identity
	}
	return resolved.Identity
}

// signingMode resolves the commit signing mode: the target override, else the repository default.
func signingMode(cfg *config, resolved *atmosgit.ResolvedRepository) atmosgit.SigningMode {
	if cfg.Signing != "" {
		return atmosgit.SigningMode(cfg.Signing)
	}
	return resolved.Signing
}

// trailers builds provenance commit trailers from the artifact metadata.
func trailers(artifact *target.ProvisionArtifact) map[string]string {
	out := make(map[string]string, 2)
	if artifact.Metadata.Stack != "" {
		out["Atmos-Stack"] = artifact.Metadata.Stack
	}
	if artifact.Metadata.Component != "" {
		out["Atmos-Component"] = artifact.Metadata.Component
	}
	return out
}

// sortedFileKeys returns the artifact file keys in deterministic order.
func sortedFileKeys(files map[string][]byte) []string {
	keys := make([]string, 0, len(files))
	for key := range files {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// stringField returns a string value from a config map, or "" when absent.
func stringField(block map[string]any, key string) string {
	value, _ := block[key].(string)
	return value
}
