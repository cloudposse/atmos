package git

import (
	"fmt"
	"os/exec"
	"regexp"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ErrCommitHasNoParents is returned when a commit has no parents (initial commit),
// so a first-parent lookup is impossible.
var ErrCommitHasNoParents = fmt.Errorf("commit has no parents")

// ErrInvalidCommitSHA indicates a string is not a plausible hex commit SHA.
var ErrInvalidCommitSHA = fmt.Errorf("invalid commit SHA")

// shaPattern matches a full or abbreviated hex commit SHA. Used to validate
// SHAs before passing them to the git CLI (defense against argument injection).
var shaPattern = regexp.MustCompile(`^[0-9a-fA-F]{4,64}$`)

// openRepo opens the repository containing repoDir, tolerating worktrees.
func openRepo(repoDir string) (*gogit.Repository, error) {
	repo, err := gogit.PlainOpenWithOptions(repoDir, &gogit.PlainOpenOptions{
		DetectDotGit:          true,
		EnableDotGitCommonDir: true,
	})
	if err != nil {
		return nil, fmt.Errorf("opening local repo: %w", err)
	}
	return repo, nil
}

// resolveCommit resolves a revision ("HEAD" or a commit SHA) to a commit object.
func resolveCommit(repo *gogit.Repository, revision string) (*object.Commit, error) {
	hash, err := repo.ResolveRevision(plumbing.Revision(revision))
	if err != nil {
		return nil, fmt.Errorf("resolving revision %q: %w", revision, err)
	}
	commit, err := repo.CommitObject(*hash)
	if err != nil {
		return nil, fmt.Errorf("getting commit %s: %w", hash, err)
	}
	return commit, nil
}

// CommitParents resolves a revision ("HEAD" or a commit SHA) in the repository
// at repoDir and returns its full SHA together with the SHAs of its parents
// (in order — index 0 is the first parent). An initial commit returns an empty
// parents slice with a nil error; a revision that cannot be resolved locally
// returns an error.
func CommitParents(repoDir, revision string) (string, []string, error) {
	defer perf.Track(nil, "git.CommitParents")()

	repo, err := openRepo(repoDir)
	if err != nil {
		return "", nil, err
	}

	commit, err := resolveCommit(repo, revision)
	if err != nil {
		return "", nil, err
	}

	parents := make([]string, 0, commit.NumParents())
	for _, parentHash := range commit.ParentHashes {
		parents = append(parents, parentHash.String())
	}

	return commit.Hash.String(), parents, nil
}

// MergeBaseSHAs computes the merge-base (common ancestor) of two revisions
// ("HEAD" or commit SHAs) in the repository at repoDir. Unlike MergeBase, it
// does not require either side to be a remote-tracking branch ref, so it can
// anchor on payload-provided commits (e.g., a merged PR's merge commit parent)
// instead of the moving tip of a branch.
func MergeBaseSHAs(repoDir, revA, revB string) (string, error) {
	defer perf.Track(nil, "git.MergeBaseSHAs")()

	repo, err := openRepo(repoDir)
	if err != nil {
		return "", err
	}

	commitA, err := resolveCommit(repo, revA)
	if err != nil {
		return "", err
	}
	commitB, err := resolveCommit(repo, revB)
	if err != nil {
		return "", err
	}

	bases, err := commitA.MergeBase(commitB)
	if err != nil {
		return "", fmt.Errorf("computing merge-base: %w", err)
	}
	if len(bases) == 0 {
		return "", ErrNoCommonAncestor
	}

	return bases[0].Hash.String(), nil
}

// FetchCommit fetches a single commit (and its reachable history, subject to
// the clone's shallow boundary) from the "origin" remote by SHA. GitHub and
// other major forges allow fetching arbitrary reachable SHAs. Used to
// materialize payload-provided commits (e.g., a merged PR's merge commit)
// that a narrow head-SHA checkout did not fetch.
func FetchCommit(repoDir, sha string) error {
	defer perf.Track(nil, "git.FetchCommit")()

	if !shaPattern.MatchString(sha) {
		return fmt.Errorf("%w: %q", ErrInvalidCommitSHA, sha)
	}

	cmd := exec.Command("git", "fetch", "origin", sha, "--no-tags")
	cmd.Dir = repoDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s: %w\n%s", errUtils.ErrFetchOrigin, sha, err, string(output))
	}

	log.Debug("Fetched commit from origin", "sha", sha)

	return nil
}
