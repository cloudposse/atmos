package source

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	agentskills "github.com/cloudposse/atmos/agent-skills"
	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/schema"
)

// GitFetcher uses Atmos authentication and cancellation for remote Git sources.
type GitFetcher struct {
	Config     *schema.AtmosConfiguration
	Downloader downloader.ContextFileDownloader
}

func localPath(source, base string) (string, bool) {
	if strings.HasPrefix(source, "file://") {
		return strings.TrimPrefix(source, "file://"), true
	}
	if filepath.IsAbs(source) || strings.HasPrefix(source, ".") {
		return filepath.Join(base, source), true
	}
	p := filepath.Join(base, source)
	info, err := os.Stat(p)
	return p, err == nil && info.IsDir()
}

// Fetch stages exactly the requested commit, or resolves the declared ref once.
func (f *GitFetcher) Fetch(ctx context.Context, repo Repository, dest string) (Repository, error) {
	if name, ok := strings.CutPrefix(repo.Source, "bundled:"); ok {
		if err := fetchBundled(name, dest); err != nil {
			return repo, err
		}
		digest, err := treeDigest(dest)
		repo.Digest = digest
		return repo, err
	}
	if path, ok := localPath(repo.Source, f.Config.BasePath); ok {
		if filepath.IsAbs(repo.Source) {
			path = repo.Source
		}
		if err := copyTree(path, dest); err != nil {
			return repo, err
		}
		digest, err := treeDigest(dest)
		repo.Digest = digest
		return repo, err
	}
	return f.fetchGit(ctx, repo, dest)
}

func fetchBundled(name, dest string) error {
	if filepath.Base(name) != name || name == ".." {
		return ErrInvalid
	}
	content, err := fs.Sub(agentskills.Skills, "skills/"+name)
	if err != nil {
		return err
	}
	return fs.WalkDir(content, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dest, filepath.FromSlash(path))
		if d.IsDir() {
			return os.MkdirAll(target, directoryMode)
		}
		raw, err := fs.ReadFile(content, path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, raw, fileMode|info.Mode().Perm()&executableMode)
	})
}

func gitSourceURL(repo Repository) (string, error) {
	raw := repo.Source
	if !strings.Contains(raw, "://") && !strings.HasPrefix(raw, "git@") {
		if !strings.Contains(strings.Split(raw, "/")[0], ".") {
			raw = "github.com/" + raw
		}
		raw = "https://" + raw
	}
	if strings.HasPrefix(raw, "git@") {
		host, path, ok := strings.Cut(strings.TrimPrefix(raw, "git@"), ":")
		if !ok {
			return "", ErrInvalid
		}
		raw = "ssh://git@" + host + "/" + path
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if err := validateGitURL(u); err != nil {
		return "", err
	}
	ref := repo.Ref
	if repo.Commit != "" {
		ref = repo.Commit
	}
	q := u.Query()
	if ref != "" {
		q.Set("ref", ref)
	}
	u.RawQuery = q.Encode()
	return "git::" + u.String(), nil
}

func (f *GitFetcher) fetchGit(ctx context.Context, repo Repository, dest string) (Repository, error) {
	remote, err := gitSourceURL(repo)
	if err != nil {
		return repo, err
	}
	dl := f.Downloader
	if dl == nil {
		dl = downloader.NewGoGetterDownloader(f.Config).(downloader.ContextFileDownloader)
	}
	metadata, err := dl.FetchWithMetadataContext(ctx, remote, dest, downloader.ClientModeDir, 5*time.Minute)
	if err != nil {
		return repo, err
	}
	commit := metadata.GitCommit
	if commit == "" {
		artifact, err := downloader.ResolveArtifact(ctx, f.Config, repo.Source, dest)
		if err != nil {
			return repo, err
		}
		if artifact.Kind != "git" {
			return repo, fmt.Errorf("%w: source did not resolve to Git", ErrInvalid)
		}
		commit = artifact.Identity
	}
	if !immutableRef(commit) {
		return repo, fmt.Errorf("%w: invalid resolved Git commit", ErrInvalid)
	}
	if repo.Commit != "" && repo.Commit != commit {
		return repo, fmt.Errorf("%w: fetched commit differs from lock", ErrDrift)
	}
	repo.Commit = commit
	return repo, nil
}

func validateGitURL(u *url.URL) error {
	if (u.User != nil && u.Scheme != "ssh") || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%w: use Atmos authentication instead of credentials or query parameters in source URLs", ErrInvalid)
	}
	if u.User != nil {
		if _, password := u.User.Password(); password {
			return ErrInvalid
		}
	}
	if !contains([]string{"https", "http", "ssh", "git"}, u.Scheme) {
		return ErrInvalid
	}
	return nil
}
