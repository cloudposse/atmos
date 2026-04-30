package git

import (
	"errors"
	"fmt"

	"github.com/go-git/go-git/v5/plumbing"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ErrNoCommonAncestor is returned when no merge-base exists between two commits.
var ErrNoCommonAncestor = fmt.Errorf("no common ancestor found")

// ErrHeadOnTargetBranch is returned when HEAD is already on the target branch
// (e.g., the merge commit was checked out), making merge-base equal to HEAD itself.
var ErrHeadOnTargetBranch = fmt.Errorf("HEAD is on target branch (merge-base equals HEAD)")

// deepenStep is the number of commits to deepen the shallow clone by when the
// initial fetch was not deep enough to reach the fork point. 200 commits is
// a reasonable balance between coverage for typical PR branches and the
// transfer cost of a single deepen.
const deepenStep = 200

// logKeyBranch is the structured-log key used for branch names in this file.
const logKeyBranch = "branch"

// MergeBase computes the common ancestor between HEAD and origin/<targetBranch>.
// This is the gold standard for determining the fork point of a PR branch,
// regardless of what commit is checked out or which merge strategy was used.
//
// Returns an error if:
//   - the local repo cannot be opened
//   - origin/<targetBranch> does not exist (e.g., shallow checkout)
//   - no common ancestor exists between the two commits
//   - the merge-base equals HEAD (HEAD is on the target branch)
//
// MergeBase is a pure read operation. It will not modify the local object
// database. Callers running in CI shallow checkouts should prefer
// MergeBaseWithAutoFetch, which transparently fetches the target branch
// and deepens history when needed.
func MergeBase(targetBranch string) (string, error) {
	repo, err := GetLocalRepo()
	if err != nil {
		return "", fmt.Errorf("opening local repo: %w", err)
	}

	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("getting HEAD: %w", err)
	}

	headCommit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return "", fmt.Errorf("getting HEAD commit: %w", err)
	}

	// Resolve origin/<targetBranch> to a commit.
	targetRefName := plumbing.NewRemoteReferenceName("origin", targetBranch)
	targetRef, err := repo.Reference(targetRefName, true)
	if err != nil {
		return "", fmt.Errorf("resolving %s: %w", targetRefName, err)
	}

	targetCommit, err := repo.CommitObject(targetRef.Hash())
	if err != nil {
		return "", fmt.Errorf("getting target commit: %w", err)
	}

	bases, err := headCommit.MergeBase(targetCommit)
	if err != nil {
		return "", fmt.Errorf("computing merge-base: %w", err)
	}

	if len(bases) == 0 {
		return "", ErrNoCommonAncestor
	}

	// If merge-base == HEAD, HEAD is on the target branch (e.g., merge commit checkout).
	// The caller should fall back to another strategy (e.g., HEAD~1).
	if bases[0].Hash == head.Hash() {
		return "", ErrHeadOnTargetBranch
	}

	return bases[0].Hash.String(), nil
}

// MergeBaseWithAutoFetch is a CI-aware wrapper around MergeBase that
// transparently recovers from shallow-clone failures by fetching the
// target branch and, if needed, deepening history.
//
// RepoDir is a path inside the repository (used to scope the fetch).
//
// Behavior:
//  1. Run MergeBase. If it succeeds, return the SHA.
//  2. If it fails because origin/<targetBranch> is not present locally,
//     call FetchRef(repoDir, targetBranch) and retry MergeBase.
//  3. If MergeBase still returns ErrNoCommonAncestor (the shallow boundary
//     does not reach the fork point), call DeepenFetch with deepenStep
//     and retry once. This is bounded — we deepen at most once.
//  4. If recovery is impossible (no network, target branch does not exist
//     remotely, etc.), return the original MergeBase error so the caller
//     can fall through to its own fallback chain.
//
// ErrHeadOnTargetBranch is propagated unchanged (no fetch can fix it).
func MergeBaseWithAutoFetch(repoDir, targetBranch string) (string, error) {
	defer perf.Track(nil, "git.MergeBaseWithAutoFetch")()

	sha, err := MergeBase(targetBranch)
	if err == nil {
		return sha, nil
	}

	// HEAD is already on the target branch — no fetch will help.
	if errors.Is(err, ErrHeadOnTargetBranch) {
		return "", err
	}

	// Heuristic: only attempt to fetch when the failure looks recoverable.
	// "ref not found" → fetch the branch; "no common ancestor" → deepen.
	refMissing := errors.Is(err, plumbing.ErrReferenceNotFound)
	noAncestor := errors.Is(err, ErrNoCommonAncestor)

	if refMissing {
		log.Debug("merge-base: target ref missing, fetching", logKeyBranch, targetBranch)
		if fetchErr := FetchRef(repoDir, targetBranch); fetchErr != nil {
			log.Debug("merge-base auto-fetch failed", logKeyBranch, targetBranch, "error", fetchErr)
			return "", err
		}
		sha, err = MergeBase(targetBranch)
		if err == nil {
			return sha, nil
		}
		if errors.Is(err, ErrHeadOnTargetBranch) {
			return "", err
		}
		noAncestor = errors.Is(err, ErrNoCommonAncestor)
	}

	if noAncestor {
		log.Debug("merge-base: no common ancestor, deepening shallow clone", logKeyBranch, targetBranch, "depth", deepenStep)
		if deepenErr := DeepenFetch(repoDir, targetBranch, deepenStep); deepenErr != nil {
			log.Debug("merge-base auto-deepen failed", logKeyBranch, targetBranch, "error", deepenErr)
			return "", err
		}
		sha, err = MergeBase(targetBranch)
		if err == nil {
			return sha, nil
		}
	}

	return "", err
}
