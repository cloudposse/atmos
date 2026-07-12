package archive

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Reproducible timestamp modes. Both modes also normalize file permission
// bits (and, for tar, zero the owner/group fields) — that half of the
// problem is independent of which timestamp strategy is used, and is the
// actual root cause behind Terraform's archive_file provider still
// producing non-reproducible output years after it shipped: umask differs
// across environments, so the same content gets different permission bits
// baked into the archive on different machines.
const (
	// ReproducibleEpoch applies one timestamp to every entry: the most
	// recent commit that touched anything under Source, so output is
	// identical across checkouts/machines regardless of real file mtimes.
	ReproducibleEpoch = "epoch"
	// ReproducibleGit resolves each entry's own most recent commit,
	// falling back to the epoch value (same lookup ReproducibleEpoch uses)
	// for files git has no history for — the common case for this step,
	// since it typically packages build output (node_modules, compiled
	// binaries) rather than tracked source.
	ReproducibleGit = "git"
)

// reproducibleFallbackEpoch is the reference timestamp used when
// reproducible archiving is requested but Source isn't inside a git
// repository, or has no commit history yet. 1980-01-01 is the earliest
// date the zip format's DOS timestamp field can represent; using it for
// tar too keeps zip/tar/tgz output consistent regardless of format.
var reproducibleFallbackEpoch = time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)

// Normalized permission bits applied to every entry when reproducibility is
// enabled, replacing whatever mode the source file actually has (which
// varies with each environment's umask). Only the executable bit is
// preserved from the source; everything else collapses to these two values.
const (
	reproducibleFileMode = 0o644
	reproducibleExecMode = 0o755
	executableBits       = 0o111
)

func isReproducibleMode(mode string) bool {
	switch mode {
	case "", ReproducibleEpoch, ReproducibleGit:
		return true
	default:
		return false
	}
}

func validateReproducibleMode(mode string) error {
	if isReproducibleMode(mode) {
		return nil
	}
	return errUtils.Build(errUtils.ErrArchiveInvalidReproducibleMode).
		WithExplanationf("%q is not a supported reproducible mode", mode).
		WithHint("Use one of: epoch, git").
		WithContext("reproducible", mode).
		Err()
}

// normalizeMode collapses mode to one of two fixed values, keeping only
// whether the source was executable. Group/other write bits and any
// environment-specific umask influence are discarded.
func normalizeMode(mode os.FileMode) os.FileMode {
	if mode&executableBits != 0 {
		return reproducibleExecMode
	}
	return reproducibleFileMode
}

// reproducibleTimestamps resolves deterministic entry mtimes sourced from
// git, so identical content produces byte-identical archive output
// regardless of checkout time or machine. A nil *reproducibleTimestamps (or
// one with an empty mode) means "don't override anything" — callers check
// this before consulting modTimeFor.
type reproducibleTimestamps struct {
	mode  string
	epoch time.Time
	repo  *git.Repository // nil when source is not inside a git repository (or has no history for it)
	root  string          // repo worktree root, for making fsPaths relative to git's tree paths
}

// newReproducibleTimestamps resolves the epoch (fallback/base) timestamp
// eagerly, since both modes need it — "git" mode uses it as the per-file
// fallback. Never returns an error: any failure to locate a git repository
// or commit history for source just falls back to reproducibleFallbackEpoch,
// since reproducible archiving must degrade gracefully outside a git
// checkout (a temp build directory, a shallow clone, and so on) rather than
// fail the whole archive operation.
func newReproducibleTimestamps(mode, source string) *reproducibleTimestamps {
	defer perf.Track(nil, "archive.newReproducibleTimestamps")()

	rt := &reproducibleTimestamps{mode: mode, epoch: reproducibleFallbackEpoch}
	if mode == "" {
		return rt
	}

	absSource, err := resolvedAbs(source)
	if err != nil {
		return rt
	}
	repo, err := git.PlainOpenWithOptions(absSource, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return rt
	}
	worktree, err := repo.Worktree()
	if err != nil {
		return rt
	}
	// worktree.Filesystem.Root() is itself symlink-resolved (go-git opens
	// the .git dir via its real path), so absSource must be resolved the
	// same way — otherwise filepath.Rel produces garbage on any system
	// where the source path crosses a symlink (e.g. macOS's /var ->
	// /private/var, which every os.TempDir()-based path goes through).
	root := worktree.Filesystem.Root()
	relSource, err := filepath.Rel(root, absSource)
	if err != nil {
		return rt
	}
	relSource = filepath.ToSlash(relSource)

	commit, err := lastCommitForPrefix(repo, relSource)
	if err != nil {
		return rt
	}

	rt.repo = repo
	rt.root = root
	rt.epoch = commit.Committer.When.UTC()
	return rt
}

// modTimeFor returns the deterministic mtime for the entry read from
// fsPath, or the zero time.Time if reproducibility isn't enabled (callers
// use IsZero to mean "use the real source mtime instead").
func (rt *reproducibleTimestamps) modTimeFor(fsPath string) time.Time {
	if rt == nil || rt.mode == "" {
		return time.Time{}
	}
	if rt.mode != ReproducibleGit || rt.repo == nil {
		return rt.epoch
	}

	absPath, err := resolvedAbs(fsPath)
	if err != nil {
		return rt.epoch
	}
	relPath, err := filepath.Rel(rt.root, absPath)
	if err != nil {
		return rt.epoch
	}
	relPath = filepath.ToSlash(relPath)

	commit, err := lastCommitForFile(rt.repo, relPath)
	if err != nil {
		return rt.epoch
	}
	return commit.Committer.When.UTC()
}

// resolvedAbs makes path absolute and resolves any symlinks in it, matching
// how go-git resolves a worktree's filesystem root. Without this, comparing
// an unresolved absolute path against go-git's root (via filepath.Rel)
// produces a garbage relative path on any system where the path crosses a
// symlink — e.g. macOS, where /var is a symlink to /private/var and every
// os.TempDir()-based path goes through it.
func resolvedAbs(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	return resolved, nil
}

// lastCommitForFile returns the most recent commit that touched the exact
// file at relPath (a path relative to the repository root).
func lastCommitForFile(repo *git.Repository, relPath string) (*object.Commit, error) {
	head, err := repo.Head()
	if err != nil {
		return nil, err
	}
	iter, err := repo.Log(&git.LogOptions{From: head.Hash(), FileName: &relPath})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	return iter.Next()
}

// lastCommitForPrefix returns the most recent commit that touched relPrefix
// itself or anything nested under it. The FileName option in go-git only
// exact-matches a single file, so a directory subtree needs the more general
// PathFilter predicate instead.
func lastCommitForPrefix(repo *git.Repository, relPrefix string) (*object.Commit, error) {
	head, err := repo.Head()
	if err != nil {
		return nil, err
	}

	// filepath.Rel returns "." when relPrefix is the repository root itself;
	// no git path is ever literally "." or "./"-prefixed, so the general
	// pathFilter below would never match anything and silently fall through
	// to the epoch fallback. Match every path instead.
	pathFilter := func(_ string) bool { return true }
	if relPrefix != "." {
		dirPrefix := relPrefix + "/"
		pathFilter = func(p string) bool {
			return p == relPrefix || strings.HasPrefix(p, dirPrefix)
		}
	}

	iter, err := repo.Log(&git.LogOptions{From: head.Hash(), PathFilter: pathFilter})
	if err != nil {
		return nil, err
	}
	defer iter.Close()
	return iter.Next()
}
