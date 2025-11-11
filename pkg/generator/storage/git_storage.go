package storage

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	errUtils "github.com/cloudposse/atmos/errors"
)

// GitBaseStorage provides git-based storage for retrieving base file versions.
// It uses git references (branches, tags, commits) to access historical file content
// without needing to store duplicate copies on disk.
type GitBaseStorage struct {
	repo    *git.Repository
	baseRef string // Git reference: "main", "v1.0.0", commit hash, etc.
}

// NewGitBaseStorage creates a new git-based storage using the specified repository and base reference.
// The baseRef parameter can be:
//   - Branch name: "main", "develop"
//   - Tag: "v1.0.0"
//   - Commit hash: "abc123def..."
//   - Special ref: "HEAD"
func NewGitBaseStorage(repo *git.Repository, baseRef string) *GitBaseStorage {
	return &GitBaseStorage{
		repo:    repo,
		baseRef: baseRef,
	}
}

// LoadBase retrieves the content of a file at the base reference.
// The filePath should be relative to the repository root.
//
// Returns:
//   - File content as string if the file exists at the base ref
//   - Empty string and nil error if the file doesn't exist at the base ref
//   - Error if the base ref is invalid or git operation fails
func (s *GitBaseStorage) LoadBase(filePath string) (string, bool, error) {
	// Resolve the base reference to a commit hash
	hash, err := s.repo.ResolveRevision(plumbing.Revision(s.baseRef))
	if err != nil {
		return "", false, fmt.Errorf("%w: failed to resolve git ref %q: %w", errUtils.ErrGitRefNotFound, s.baseRef, err)
	}

	// Get the commit object
	commit, err := s.repo.CommitObject(*hash)
	if err != nil {
		return "", false, fmt.Errorf("failed to get commit for ref %q: %w", s.baseRef, err)
	}

	// Get the tree (file structure) at this commit
	tree, err := commit.Tree()
	if err != nil {
		return "", false, fmt.Errorf("failed to get tree for commit %s: %w", hash.String(), err)
	}

	// Clean the file path (remove leading ./ if present)
	cleanPath := filepath.Clean(filePath)

	// Normalize path separators to forward slashes for go-git
	// go-git expects forward slashes regardless of OS
	normalizedPath := filepath.ToSlash(cleanPath)

	// Try to get the file from the tree
	file, err := tree.File(normalizedPath)
	if err != nil {
		// File doesn't exist at this ref - this is not an error, just means no base version
		if errors.Is(err, object.ErrFileNotFound) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("failed to read file %q at ref %q: %w", filePath, s.baseRef, err)
	}

	// Read the file content
	reader, err := file.Reader()
	if err != nil {
		return "", false, fmt.Errorf("failed to open file %q: %w", filePath, err)
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		return "", false, fmt.Errorf("failed to read file %q: %w", filePath, err)
	}

	return string(content), true, nil
}

// ValidateBaseRef checks if the base reference exists in the repository.
// Returns nil if the ref is valid, or an error with helpful guidance if invalid.
func (s *GitBaseStorage) ValidateBaseRef() error {
	_, err := s.repo.ResolveRevision(plumbing.Revision(s.baseRef))
	if err != nil {
		return fmt.Errorf(
			"%w: base ref %q not found in git repository\n"+
				"This usually means:\n"+
				"  - The tag/branch was deleted\n"+
				"  - Wrong ref specified in .atmos/init/metadata.yaml\n"+
				"\n"+
				"Solutions:\n"+
				"  1. Run: atmos init --force (regenerate from template)\n"+
				"  2. Update .atmos/init/metadata.yaml with correct base_ref\n"+
				"  3. Specify manually: atmos init --update --base-ref=main",
			errUtils.ErrGitRefNotFound,
			s.baseRef,
		)
	}
	return nil
}

// GetMergeBase returns the merge-base (common ancestor) between two refs.
// This is useful for finding the original point where files were generated.
//
// Example: GetMergeBase("HEAD", "main") finds the commit where the current
// branch diverged from main - the ideal base for 3-way merges.
func (s *GitBaseStorage) GetMergeBase(ref1, ref2 string) (string, error) {
	// Resolve both refs to commit hashes
	hash1, err := s.repo.ResolveRevision(plumbing.Revision(ref1))
	if err != nil {
		return "", fmt.Errorf("failed to resolve ref %q: %w", ref1, err)
	}

	hash2, err := s.repo.ResolveRevision(plumbing.Revision(ref2))
	if err != nil {
		return "", fmt.Errorf("failed to resolve ref %q: %w", ref2, err)
	}

	// Get commit objects
	commit1, err := s.repo.CommitObject(*hash1)
	if err != nil {
		return "", fmt.Errorf("failed to get commit for %q: %w", ref1, err)
	}

	commit2, err := s.repo.CommitObject(*hash2)
	if err != nil {
		return "", fmt.Errorf("failed to get commit for %q: %w", ref2, err)
	}

	// Find common ancestors
	ancestors, err := commit1.MergeBase(commit2)
	if err != nil {
		return "", fmt.Errorf("failed to find merge base: %w", err)
	}

	if len(ancestors) == 0 {
		return "", fmt.Errorf("no common ancestor found between %q and %q", ref1, ref2)
	}

	// Return the first (most recent) common ancestor
	return ancestors[0].Hash.String(), nil
}

// SetBaseRef updates the base reference for this storage.
// Useful when you want to use a different ref without creating a new storage instance.
func (s *GitBaseStorage) SetBaseRef(baseRef string) {
	s.baseRef = baseRef
}

// GetBaseRef returns the current base reference.
func (s *GitBaseStorage) GetBaseRef() string {
	return s.baseRef
}
