package git

import (
	"fmt"

	"github.com/go-git/go-git/v5/plumbing"
)

// ErrNoCommonAncestor is returned when no merge-base exists between two commits.
var ErrNoCommonAncestor = fmt.Errorf("no common ancestor found")

// ErrHeadOnTargetBranch is returned when HEAD is already on the target branch
// (e.g., the merge commit was checked out), making merge-base equal to HEAD itself.
var ErrHeadOnTargetBranch = fmt.Errorf("HEAD is on target branch (merge-base equals HEAD)")

// MergeBase computes the common ancestor between HEAD and origin/<targetBranch>.
// This is the gold standard for determining the fork point of a PR branch,
// regardless of what commit is checked out or which merge strategy was used.
//
// Returns an error if:
//   - the local repo cannot be opened
//   - origin/<targetBranch> does not exist (e.g., shallow checkout)
//   - no common ancestor exists between the two commits
//   - the merge-base equals HEAD (HEAD is on the target branch)
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
