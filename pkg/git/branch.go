package git

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"path"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// PrepareBranchOptions describes a safe feature-branch checkout. It never
// checks out or pushes the base branch and never force-pushes.
type PrepareBranchOptions struct {
	Workdir string
	Remote  string
	Base    string
	Branch  string
}

// PrepareBranch verifies that the caller's checkout is clean, fetches the base
// branch, and creates/reuses a local feature branch. A remote feature branch is
// preferred when present so a scheduled retry continues an open PR.
//
//nolint:revive // The safety checks are intentionally linear and explicit.
func PrepareBranch(ctx context.Context, opts PrepareBranchOptions) error {
	defer perf.Track(nil, "git.PrepareBranch")()

	if opts.Remote == "" {
		opts.Remote = "origin"
	}
	if opts.Base == "" || opts.Branch == "" {
		return fmt.Errorf("%w: base branch and feature branch are required", errUtils.ErrComponentUpdaterConfig)
	}
	if err := gitRun(ctx, opts.Workdir, "rev-parse", "--is-inside-work-tree"); err != nil {
		return fmt.Errorf("%w: run from a Git repository: %w", errUtils.ErrComponentUpdaterConfig, err)
	}
	status, err := gitOutput(ctx, opts.Workdir, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) != "" {
		return errUtils.ErrComponentUpdaterDirtyWorktree
	}
	if err := gitRun(ctx, opts.Workdir, "fetch", opts.Remote, opts.Base); err != nil {
		return fmt.Errorf("%w: fetching %s/%s: %w", errUtils.ErrGitFetchFailed, opts.Remote, opts.Base, err)
	}
	remoteBranch := opts.Remote + "/" + opts.Branch
	if gitRun(ctx, opts.Workdir, "ls-remote", "--exit-code", "--heads", opts.Remote, opts.Branch) == nil {
		if err := gitRun(ctx, opts.Workdir, "fetch", opts.Remote, opts.Branch); err != nil {
			return fmt.Errorf("%w: fetching existing feature branch: %w", errUtils.ErrGitFetchFailed, err)
		}
		if err := gitRun(ctx, opts.Workdir, "checkout", "-B", opts.Branch, remoteBranch); err != nil {
			return fmt.Errorf("%w: checking out existing feature branch: %w", errUtils.ErrGitCheckoutFailed, err)
		}
		return nil
	}
	if err := gitRun(ctx, opts.Workdir, "checkout", "-B", opts.Branch, opts.Remote+"/"+opts.Base); err != nil {
		return fmt.Errorf("%w: creating feature branch from %s: %w", errUtils.ErrGitCheckoutFailed, opts.Base, err)
	}
	return nil
}

// DefaultBranch resolves the remote's advertised default branch without
// assuming main or master.
func DefaultBranch(ctx context.Context, workdir, remote string) (string, error) {
	defer perf.Track(nil, "git.DefaultBranch")()

	if remote == "" {
		remote = "origin"
	}
	out, err := gitOutput(ctx, workdir, "ls-remote", "--symref", remote, "HEAD")
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrGitDefaultBranchResolution, err)
	}
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "ref:" && strings.HasPrefix(fields[1], "refs/heads/") {
			return strings.TrimPrefix(fields[1], "refs/heads/"), nil
		}
	}
	return "", fmt.Errorf("%w: remote %q did not advertise a default branch", errUtils.ErrComponentUpdaterConfig, remote)
}

// GitHubRepository parses an origin URL into the owner/repository pair used by
// GitHub's API. SSH and HTTPS remote forms are supported.
func GitHubRepository(ctx context.Context, workdir, remote string) (string, string, error) {
	defer perf.Track(nil, "git.GitHubRepository")()

	if remote == "" {
		remote = "origin"
	}
	remoteURL, err := gitOutput(ctx, workdir, "remote", "get-url", remote)
	if err != nil {
		return "", "", err
	}
	remoteURL = strings.TrimSuffix(strings.TrimSpace(remoteURL), ".git")
	repoPath, ok := githubRepositoryPath(remoteURL)
	if !ok {
		return "", "", fmt.Errorf("%w: GitHub PR publishing requires a github.com remote, got %q", errUtils.ErrComponentUpdaterConfig, remoteURL)
	}
	owner, repo := path.Split(repoPath)
	owner = strings.TrimSuffix(owner, "/")
	if owner == "" || repo == "" || strings.Contains(owner, "/") {
		return "", "", fmt.Errorf("%w: unable to parse GitHub remote %q", errUtils.ErrComponentUpdaterConfig, remoteURL)
	}
	return owner, repo, nil
}

func githubRepositoryPath(remoteURL string) (string, bool) {
	if strings.HasPrefix(remoteURL, "git@github.com:") {
		return strings.TrimPrefix(remoteURL, "git@github.com:"), true
	}
	parsed, err := url.Parse(remoteURL)
	if err != nil || !strings.EqualFold(parsed.Hostname(), "github.com") {
		return "", false
	}
	return strings.TrimPrefix(parsed.Path, "/"), true
}

func gitRun(ctx context.Context, workdir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workdir
	return cmd.Run()
}

func gitOutput(ctx context.Context, workdir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = workdir
	out, err := cmd.Output()
	return string(out), err
}
