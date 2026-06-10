package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// Minimum porcelain line length ("XY path").
	statusEntryMinLen = 4
	// Newline separates porcelain entries and trailer lines.
	newline = "\n"
)

// Status reports porcelain status, optionally scoped to paths.
func (p *Provider) Status(ctx context.Context, opts *atmosgit.StatusOptions) (*atmosgit.StatusResult, error) {
	defer perf.Track(nil, "cli.Provider.Status")()

	// --untracked-files=all lists individual untracked files instead of
	// collapsing wholly-untracked directories (e.g. "clusters/"), which would
	// defeat path-scoped containment checks.
	args := []string{"status", "--porcelain", "--untracked-files=all"}
	if len(opts.Paths) > 0 {
		args = append(args, "--")
		args = append(args, opts.Paths...)
	}

	result, err := p.run(ctx, opts.Workdir, opts.Env, args...)
	if err != nil {
		return nil, classify(err, result, "status")
	}

	entries := parsePorcelain(result.Stdout)
	return &atmosgit.StatusResult{Clean: len(entries) == 0, Entries: entries}, nil
}

// Diff reports the difference between the worktree and HEAD: a unified diff
// of tracked changes plus the list of untracked files. This is the
// read-before-write step for GitOps publishing.
func (p *Provider) Diff(ctx context.Context, opts *atmosgit.DiffOptions) (*atmosgit.DiffResult, error) {
	defer perf.Track(nil, "cli.Provider.Diff")()

	status, err := p.Status(ctx, (*atmosgit.StatusOptions)(opts))
	if err != nil {
		return nil, err
	}

	diff := &atmosgit.DiffResult{HasChanges: !status.Clean}
	for _, entry := range status.Entries {
		if entry.Code == "??" {
			diff.Untracked = append(diff.Untracked, entry.Path)
		}
	}

	if !p.hasHead(ctx, opts.RepoContext) {
		return diff, nil
	}

	args := []string{"diff", "HEAD"}
	if len(opts.Paths) > 0 {
		args = append(args, "--")
		args = append(args, opts.Paths...)
	}
	result, err := p.run(ctx, opts.Workdir, opts.Env, args...)
	if err != nil {
		return nil, classify(err, result, "diff")
	}
	diff.Output = result.Stdout

	return diff, nil
}

// Commit stages managed paths (when given) and creates a commit. A commit
// with nothing to commit is a clean no-op: Committed=false, nil error.
func (p *Provider) Commit(ctx context.Context, opts *atmosgit.CommitOptions) (*atmosgit.CommitResult, error) {
	defer perf.Track(nil, "cli.Provider.Commit")()

	if len(opts.Paths) > 0 {
		if err := p.stageManagedPaths(ctx, opts); err != nil {
			return nil, err
		}
	}

	staged, err := p.hasStagedChanges(ctx, opts.RepoContext)
	if err != nil {
		return nil, err
	}
	if !staged {
		return &atmosgit.CommitResult{Committed: false}, nil
	}

	args := buildCommitArgs(opts)
	result, err := p.run(ctx, opts.Workdir, opts.Env, args...)
	if err != nil {
		return nil, classify(err, result, "commit")
	}

	sha, err := p.headSHA(ctx, opts.RepoContext)
	if err != nil {
		return nil, err
	}

	return &atmosgit.CommitResult{Committed: true, SHA: sha}, nil
}

// stageManagedPaths verifies no dirty files exist outside the managed paths
// (the path-scoped commit safety rule), then stages the managed paths.
func (p *Provider) stageManagedPaths(ctx context.Context, opts *atmosgit.CommitOptions) error {
	status, err := p.Status(ctx, &atmosgit.StatusOptions{RepoContext: opts.RepoContext})
	if err != nil {
		return err
	}

	outside := entriesOutsidePaths(status.Entries, opts.Paths)
	if len(outside) > 0 {
		return fmt.Errorf("%w: %s", errUtils.ErrGitDirtyUnmanagedFiles, strings.Join(outside, ", "))
	}

	args := append([]string{"add", "--"}, opts.Paths...)
	result, err := p.run(ctx, opts.Workdir, opts.Env, args...)
	if err != nil {
		return classify(err, result, "add")
	}
	return nil
}

// hasStagedChanges reports whether the index differs from HEAD.
func (p *Provider) hasStagedChanges(ctx context.Context, rc atmosgit.RepoContext) (bool, error) {
	if !p.hasHead(ctx, rc) {
		// Initial commit: anything staged counts; ls-files reports the index.
		result, err := p.run(ctx, rc.Workdir, rc.Env, "ls-files", "--cached")
		if err != nil {
			return false, classify(err, result, "ls-files")
		}
		return strings.TrimSpace(result.Stdout) != "", nil
	}

	result, err := p.run(ctx, rc.Workdir, rc.Env, "diff", "--cached", "--quiet")
	if err == nil {
		return false, nil
	}
	if result.ExitCode == 1 {
		return true, nil
	}
	return false, classify(err, result, "diff --cached")
}

// hasHead reports whether the repository has any commit.
func (p *Provider) hasHead(ctx context.Context, rc atmosgit.RepoContext) bool {
	_, err := p.run(ctx, rc.Workdir, rc.Env, "rev-parse", "--verify", "HEAD")
	return err == nil
}

// headSHA returns the current HEAD commit SHA.
func (p *Provider) headSHA(ctx context.Context, rc atmosgit.RepoContext) (string, error) {
	result, err := p.run(ctx, rc.Workdir, rc.Env, "rev-parse", "HEAD")
	if err != nil {
		return "", classify(err, result, "rev-parse")
	}
	return strings.TrimSpace(result.Stdout), nil
}

// buildCommitArgs assembles the git commit invocation: author injection via
// per-invocation -c (never mutating repo or global config), signing flags,
// and the message with provenance trailers appended.
func buildCommitArgs(opts *atmosgit.CommitOptions) []string {
	var args []string
	if opts.Author != nil {
		args = append(
			args,
			"-c", "user.name="+opts.Author.Name,
			"-c", "user.email="+opts.Author.Email,
		)
	}

	args = append(args, "commit", "-m", messageWithTrailers(opts.Message, opts.Trailers))

	switch opts.Signing {
	case atmosgit.SigningAlways:
		args = append(args, "-S")
	case atmosgit.SigningNever:
		args = append(args, "--no-gpg-sign")
	case atmosgit.SigningAuto, "":
		// Git config decides.
	}

	return args
}

// messageWithTrailers appends sorted "Key: value" trailer lines to the message.
func messageWithTrailers(message string, trailers map[string]string) string {
	if len(trailers) == 0 {
		return message
	}

	keys := make([]string, 0, len(trailers))
	for key := range trailers {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(strings.TrimRight(message, newline))
	b.WriteString(newline)
	for _, key := range keys {
		b.WriteString(newline)
		b.WriteString(key)
		b.WriteString(": ")
		b.WriteString(trailers[key])
	}

	return b.String()
}

// parsePorcelain parses `git status --porcelain` output.
func parsePorcelain(out string) []atmosgit.StatusEntry {
	var entries []atmosgit.StatusEntry
	for _, line := range strings.Split(out, newline) {
		if len(line) < statusEntryMinLen {
			continue
		}
		entries = append(entries, atmosgit.StatusEntry{
			Code: line[:2],
			Path: strings.TrimSpace(line[3:]),
		})
	}
	return entries
}

// entriesOutsidePaths returns the paths of status entries not contained in
// any managed path (slash-normalized prefix match).
func entriesOutsidePaths(entries []atmosgit.StatusEntry, managed []string) []string {
	var outside []string
	for _, entry := range entries {
		if !pathWithinAny(entry.Path, managed) {
			outside = append(outside, entry.Path)
		}
	}
	return outside
}

// pathWithinAny reports whether p equals or is under any of the given paths.
func pathWithinAny(p string, paths []string) bool {
	normalized := filepath.ToSlash(p)
	for _, managed := range paths {
		prefix := strings.TrimSuffix(filepath.ToSlash(managed), "/")
		if normalized == prefix || strings.HasPrefix(normalized, prefix+"/") {
			return true
		}
	}
	return false
}
