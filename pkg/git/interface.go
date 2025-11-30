package git

import (
	git "github.com/go-git/go-git/v5"
)

// RepositoryOperations defines operations for working with git repositories.
// This interface allows mocking of git operations in tests.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type RepositoryOperations interface {
	// GetLocalRepo opens the local git repository.
	GetLocalRepo() (*git.Repository, error)

	// GetRepoInfo extracts repository information (URL, name, owner, host).
	GetRepoInfo(localRepo *git.Repository) (RepoInfo, error)
}

// DefaultRepositoryOperations implements RepositoryOperations using real git operations.
type DefaultRepositoryOperations struct{}

// GetLocalRepo opens the local git repository.
func (d *DefaultRepositoryOperations) GetLocalRepo() (*git.Repository, error) {
	return GetLocalRepo()
}

// GetRepoInfo extracts repository information.
func (d *DefaultRepositoryOperations) GetRepoInfo(localRepo *git.Repository) (RepoInfo, error) {
	return GetRepoInfo(localRepo)
}
