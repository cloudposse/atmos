package vendoring

//go:generate mockgen -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE

import (
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// GitOperations defines the interface for Git operations used by vendor commands.
// This interface allows for mocking Git operations in tests.
type GitOperations interface {
	// GetRemoteTags fetches all tags from a remote Git repository.
	GetRemoteTags(gitURI string) ([]string, error)

	// CheckRef verifies that a Git reference exists in a remote repository.
	CheckRef(gitURI string, ref string) (bool, error)

	// GetDiffBetweenRefs generates a diff between two Git refs.
	GetDiffBetweenRefs(atmosConfig *schema.AtmosConfiguration, gitURI string, fromRef string, toRef string, contextLines int, noColor bool) ([]byte, error)
}

// realGitOperations implements GitOperations using actual git commands.
type realGitOperations struct{}

// NewGitOperations creates a new GitOperations implementation.
func NewGitOperations() GitOperations {
	return &realGitOperations{}
}

// GetRemoteTags implements GitOperations.GetRemoteTags.
func (g *realGitOperations) GetRemoteTags(gitURI string) ([]string, error) {
	defer perf.Track(nil, "exec.GetRemoteTags")()

	return getGitRemoteTags(gitURI)
}

// CheckRef implements GitOperations.CheckRef.
func (g *realGitOperations) CheckRef(gitURI string, ref string) (bool, error) {
	defer perf.Track(nil, "exec.CheckRef")()

	return checkGitRef(gitURI, ref)
}

// GetDiffBetweenRefs implements GitOperations.GetDiffBetweenRefs.
//
//nolint:revive // Six parameters needed for Git diff configuration.
func (g *realGitOperations) GetDiffBetweenRefs(atmosConfig *schema.AtmosConfiguration, gitURI string, fromRef string, toRef string, contextLines int, noColor bool) ([]byte, error) {
	defer perf.Track(atmosConfig, "exec.GetDiffBetweenRefs")()

	return getGitDiffBetweenRefs(atmosConfig, gitURI, fromRef, toRef, contextLines, noColor)
}
