package git

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
)

// ValidatePublish checks raw-file targets without opening a Git repository.
func (g *gitProvisioner) ValidatePublish(in *target.PublishInput) error {
	defer perf.Track(in.AtmosConfig, "target.git.ValidatePublish")()
	cfg, err := parseConfig(in.TargetConfig)
	if err != nil {
		return err
	}
	if cfg.Split != nil || cfg.PullRequest {
		return fmt.Errorf("%w: publish does not support split or pull_request", errUtils.ErrPublishTarget)
	}
	if err := validatePublishSettings(in.TargetConfig); err != nil {
		return err
	}
	if cfg.Repository == "" || cfg.Path == "" || filepath.Clean(cfg.Path) == "." {
		return fmt.Errorf("%w: git requires repository and a non-root path", errUtils.ErrPublishTarget)
	}
	if err := target.ValidatePublishPath(cfg.Path); err != nil {
		return err
	}
	if in.AtmosConfig == nil {
		return errUtils.ErrPublishTarget
	}
	_, err = atmosgit.ResolveRepository(&in.AtmosConfig.Git, cfg.Repository)
	return err
}

const publishCommitField = "commit"

func validatePublishSettings(settings map[string]any) error {
	for key, value := range settings {
		if err := validatePublishSetting(key, value); err != nil {
			return err
		}
	}
	return nil
}

func validatePublishSetting(key string, value any) error {
	switch key {
	case "kind", "repository", "path":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%w: %s must be a string", errUtils.ErrPublishTarget, key)
		}
	case publishCommitField:
		return validatePublishCommit(value)
	case "auth":
	case "pull_request":
		return validatePublishPullRequest(value)
	default:
		return fmt.Errorf("%w: unsupported git setting %q", errUtils.ErrPublishTarget, key)
	}
	return nil
}

func validatePublishCommit(value any) error {
	commit, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: commit must be a mapping", errUtils.ErrPublishTarget)
	}
	for key, value := range commit {
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%w: commit.%s must be a string", errUtils.ErrPublishTarget, key)
		}
		switch key {
		case "message":
		case "signing":
			switch atmosgit.SigningMode(text) {
			case "", atmosgit.SigningAuto, atmosgit.SigningAlways, atmosgit.SigningNever:
			default:
				return fmt.Errorf("%w: invalid commit signing mode %q", errUtils.ErrPublishTarget, text)
			}
		default:
			return fmt.Errorf("%w: unsupported commit setting %q", errUtils.ErrPublishTarget, key)
		}
	}
	return nil
}

func validatePublishPullRequest(value any) error {
	block, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%w: pull_request must be a mapping", errUtils.ErrPublishTarget)
	}
	for key, value := range block {
		if key != "enabled" {
			return fmt.Errorf("%w: unsupported pull_request setting %q", errUtils.ErrPublishTarget, key)
		}
		enabled, ok := value.(bool)
		if !ok || enabled {
			return fmt.Errorf("%w: publish requires pull_request.enabled to be false", errUtils.ErrPublishTarget)
		}
	}
	return nil
}

// Publish writes raw files without manifest conversion or destination pruning.
func (g *gitProvisioner) Publish(ctx context.Context, in *target.PublishInput) (*target.PublishResult, error) {
	defer perf.Track(in.AtmosConfig, "target.git.Publish")()
	if err := g.ValidatePublish(in); err != nil {
		return nil, err
	}
	cfg, err := parseConfig(in.TargetConfig)
	if err != nil {
		return nil, err
	}
	session, err := publishSession(ctx, in, &cfg)
	if err != nil {
		return nil, err
	}
	if err := reconcile(ctx, session); err != nil {
		return nil, err
	}
	pendingPush, err := hasUnpushedCommits(ctx, session)
	if err != nil {
		return nil, err
	}
	result := &target.PublishResult{Metadata: map[string]any{"repository": cfg.Repository, "branch": session.resolved.Branch}}
	if err := writePublishFiles(ctx, session, cfg.Path, in.Files, result); err != nil {
		return nil, err
	}
	if len(in.Files) == 0 && !pendingPush {
		return result, nil
	}
	commit, err := commitAndPushResult(ctx, session, &cfg, &target.ProvisionArtifact{Metadata: in.Metadata}, pendingPush)
	if err != nil {
		return nil, err
	}
	result.Metadata[publishCommitField] = commit
	return result, nil
}

func hasUnpushedCommits(ctx context.Context, session *repoSession) (bool, error) {
	checker, ok := session.provider.(atmosgit.UnpushedCommitChecker)
	if !ok {
		return false, fmt.Errorf("%w: Git provider cannot inspect unpublished commits", errUtils.ErrPublishTarget)
	}
	return checker.HasUnpushedCommits(ctx, session.rc)
}

func writePublishFiles(ctx context.Context, session *repoSession, root string, files []target.PublishFile, result *target.PublishResult) error {
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		changed, err := writePublishFile(session.rc.Workdir, root, file)
		if err != nil {
			return fmt.Errorf("%w: %s: %w", errUtils.ErrPublishFailed, file.Name, err)
		}
		result.Locations = append(result.Locations, filepath.ToSlash(filepath.Join(root, file.Name)))
		if changed {
			result.Changed++
		} else {
			result.Unchanged++
		}
	}
	return nil
}

func publishSession(ctx context.Context, in *target.PublishInput, cfg *config) (*repoSession, error) {
	resolved, err := atmosgit.ResolveRepository(&in.AtmosConfig.Git, cfg.Repository)
	if err != nil {
		return nil, err
	}
	env := make([]string, 0, len(in.Env))
	for key, value := range in.Env {
		env = append(env, key+"="+value)
	}
	env, err = atmosgit.ComposeEnvironment(ctx, env, identityFor(cfg, resolved), in.EnvProvider)
	if err != nil {
		return nil, err
	}
	provider, err := newProvider(resolved.Provider)
	if err != nil {
		return nil, err
	}
	return &repoSession{provider: provider, resolved: resolved, rc: atmosgit.RepoContext{Workdir: resolved.Workdir, Remote: resolved.Remote, Branch: resolved.Branch, Env: env}}, nil
}

func writePublishFile(workdir, root string, file target.PublishFile) (bool, error) {
	if err := target.ValidatePublishPath(file.Name); err != nil {
		return false, err
	}
	relative := filepath.Join(root, filepath.FromSlash(file.Name))
	dest, err := atmosgit.ValidateRepoRelativePath(workdir, relative)
	if err != nil {
		return false, err
	}
	if err := rejectPublishSymlinks(workdir, relative); err != nil {
		return false, err
	}
	reader, err := file.Open()
	if err != nil {
		return false, err
	}
	defer reader.Close()
	same, err := samePublishedFile(dest, reader)
	if err != nil || same {
		return false, err
	}
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return false, err
	}
	if err := replacePublishedFile(dest, reader); err != nil {
		return false, err
	}
	return true, nil
}

func replacePublishedFile(dest string, reader io.Reader) error {
	if err := os.MkdirAll(filepath.Dir(dest), dirPerm); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(dest), ".atmos-publish-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	_, copyErr := io.Copy(temp, reader)
	if err := errors.Join(copyErr, temp.Close()); err != nil {
		return err
	}
	return os.Rename(temp.Name(), dest)
}

func rejectPublishSymlinks(workdir, relative string) error {
	current := workdir
	for _, part := range strings.Split(relative, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%w: symlink destination %s", errUtils.ErrPublishTarget, current)
		}
	}
	return nil
}

func samePublishedFile(dest string, reader io.Reader) (bool, error) {
	existing, err := os.Open(dest)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer existing.Close()
	info, err := existing.Stat()
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("%w: destination is not a regular file", errUtils.ErrPublishTarget)
	}
	localHash, remoteHash := sha256.New(), sha256.New()
	if _, err := io.Copy(localHash, reader); err != nil {
		return false, err
	}
	if _, err := io.Copy(remoteHash, existing); err != nil {
		return false, err
	}
	return string(localHash.Sum(nil)) == string(remoteHash.Sum(nil)), nil
}
