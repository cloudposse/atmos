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
	"regexp"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/ui"

	// Blank import registers the "cli" git provider (the only v1 backend) so
	// atmosgit.NewProvider("cli") resolves wherever the git target is compiled in.
	_ "github.com/cloudposse/atmos/pkg/git/providers/cli"
)

const (
	dirPerm  = 0o755
	filePerm = 0o600
)

// newProvider resolves the git.Provider implementation for a resolved repository's
// provider name. It is a package-level var (not a direct atmosgit.NewProvider call at
// each use site) so tests can install a deterministic double via setTestProvider,
// without touching the real git registry or invoking the git binary.
var newProvider = atmosgit.NewProvider

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
	// Split selects file-vs-directory semantics for Path: true fans out one file
	// per manifest under Path (a directory); false writes Path as a single
	// multi-document YAML file. nil defers to resolveSplit's extension inference.
	Split *bool
}

// manifestPathRE matches a manifest-looking filename in the last path segment,
// used by resolveSplit to infer single-file mode when Split is left unset.
var manifestPathRE = regexp.MustCompile(`(?i)\.(ya?ml|json)$`)

// resolveSplit implements the Split tri-state: an explicit target-config value
// wins; otherwise the last path segment is matched against manifestPathRE — a
// match defaults to single-file mode, no match preserves the unconditional
// directory-fan-out default every existing configuration already relies on.
func resolveSplit(split *bool, path string) bool {
	if split != nil {
		return *split
	}
	return !manifestPathRE.MatchString(filepath.Base(path))
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

	cfg, err := parseConfig(in.TargetConfig)
	if err != nil {
		return err
	}

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

	provider, err := newProvider(resolved.Provider)
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

	if err := writeArtifact(resolved.Workdir, cfg.Path, &in.Artifact, resolveSplit(cfg.Split, cfg.Path)); err != nil {
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

	cfg, err := parseConfig(in.TargetConfig)
	if err != nil {
		return target.ProvisionArtifact{}, err
	}

	resolved, err := atmosgit.ResolveRepository(&in.AtmosConfig.Git, cfg.Repository)
	if err != nil {
		return target.ProvisionArtifact{}, err
	}

	env, err := atmosgit.ComposeEnvironment(ctx, os.Environ(), identityFor(&cfg, resolved), in.EnvProvider)
	if err != nil {
		return target.ProvisionArtifact{}, err
	}

	provider, err := newProvider(resolved.Provider)
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
	stderr, err := atmosgit.CaptureStderr(s.provider, func() error {
		return s.provider.Clone(ctx, &atmosgit.CloneOptions{
			RepoContext:  s.rc,
			URI:          s.resolved.URI,
			Depth:        s.resolved.Clone.Depth,
			Filter:       s.resolved.Clone.Filter,
			SingleBranch: s.resolved.Clone.SingleBranch,
			Submodules:   s.resolved.Clone.Submodules,
		})
	})
	return atmosgit.WrapOperationError(
		"clone/reconcile Git repository",
		s.rc.Workdir,
		stderr,
		err,
		"Confirm the configured branch exists and has commits, and that the resolved identity has read access.",
	)
}

// commitAndPush stages the managed path, commits any changes, and pushes when a
// commit was created.
func commitAndPush(ctx context.Context, s *repoSession, cfg *config, artifact *target.ProvisionArtifact) error {
	var result atmosgit.CommitResult
	stderr, err := atmosgit.CaptureStderr(s.provider, func() error {
		res, commitErr := s.provider.Commit(ctx, &atmosgit.CommitOptions{
			RepoContext: s.rc,
			Message:     cfg.CommitMessage,
			Paths:       []string{cfg.Path},
			Signing:     signingMode(cfg, s.resolved),
			Author:      s.resolved.Author,
			Trailers:    trailers(artifact),
		})
		if res != nil {
			result = *res
		}
		return commitErr
	})
	if err != nil {
		return atmosgit.WrapOperationError("commit Git changes", s.rc.Workdir, stderr, err, "")
	}
	if !result.Committed {
		// Nothing changed in the managed path; a no-op is a clean success.
		return nil
	}

	stderr, err = atmosgit.CaptureStderr(s.provider, func() error {
		return s.provider.Push(ctx, &atmosgit.PushOptions{
			RepoContext: s.rc,
			Retries:     s.resolved.PushRetries,
		})
	})
	return atmosgit.WrapOperationError(
		"push Git repository",
		s.rc.Workdir,
		stderr,
		err,
		"Run 'atmos git status' and 'atmos git pull' on the configured repository before retrying.",
	)
}

// writeArtifact replaces the managed path <workdir>/<path> with the artifact
// files, so removals propagate deterministically. When split is true, path is
// a directory root fanned out into one file per artifact entry (unchanged,
// historical behavior). When split is false, path is the exact output file: all
// artifact entries are merged into a single multi-document YAML file.
func writeArtifact(workdir, path string, artifact *target.ProvisionArtifact, split bool) error {
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
	warnOnSplitModeFlip(absPath, path, split)
	if err := os.RemoveAll(absPath); err != nil {
		return fmt.Errorf("%w: clearing managed path %q: %w", errUtils.ErrGitArtifactWrite, path, err)
	}

	if !split {
		return writeSingleArtifactFile(absPath, path, artifact)
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

// warnOnSplitModeFlip surfaces a warning when the existing content at absPath
// (a file left by a prior split:false delivery, or a directory left by a prior
// split:true delivery) doesn't match the delivery mode about to be written.
// writeArtifact always replaces whatever is at path unconditionally (by
// design: it's the managed path, and the result is git-committed and
// revertable), but a silent recursive delete of a whole directory tree on a
// one-line config change is easy to miss without a log line calling it out.
func warnOnSplitModeFlip(absPath, path string, split bool) {
	info, err := os.Stat(absPath)
	if err != nil {
		// Nothing there yet (or unreadable) -- no prior mode to flip from.
		return
	}
	existingIsDir := info.IsDir()
	if existingIsDir == split {
		return
	}
	if existingIsDir {
		ui.Warningf("replacing existing managed directory %q with a single file: split mode changed to false", path)
	} else {
		ui.Warningf("replacing existing managed file %q with a directory: split mode changed to true", path)
	}
}

// writeSingleArtifactFile merges every artifact file (in deterministic order)
// into one multi-document YAML stream and writes it to absPath (repo-relative
// path, for error messages) as a single file.
func writeSingleArtifactFile(absPath, path string, artifact *target.ProvisionArtifact) error {
	keys := sortedFileKeys(artifact.Files)
	docs := make([][]byte, 0, len(keys))
	for _, rel := range keys {
		docs = append(docs, artifact.Files[rel])
	}
	merged := target.MergeYAMLDocuments(docs)

	if err := os.MkdirAll(filepath.Dir(absPath), dirPerm); err != nil {
		return fmt.Errorf("%w: creating directory for %q: %w", errUtils.ErrGitArtifactWrite, path, err)
	}
	if err := os.WriteFile(absPath, merged, filePerm); err != nil {
		return fmt.Errorf("%w: writing %q: %w", errUtils.ErrGitArtifactWrite, path, err)
	}
	return nil
}

// parseConfig extracts the git target settings from the merged target block.
// A "split" key present with a non-bool value is a fail-closed error rather
// than a silent ignore: kubernetes validate/apply/deploy resolve this config
// directly and never go through the stricter Atmos manifest JSON Schema check
// that atmos validate stacks/describe stacks run, so this is the only gate
// that catches a typo'd (e.g. quoted) split value on that path.
func parseConfig(block map[string]any) (config, error) {
	cfg := config{
		Repository: stringField(block, "repository"),
		Path:       stringField(block, "path"),
	}
	if raw, present := block["split"]; present {
		split, ok := raw.(bool)
		if !ok {
			return config{}, fmt.Errorf("%w: got %T", errUtils.ErrGitTargetSplitInvalid, raw)
		}
		cfg.Split = &split
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
	return cfg, nil
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
