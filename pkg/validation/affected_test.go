package validation

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAffectedFilesIncludesCommittedWorkingTreeAndUntrackedFiles(t *testing.T) {
	originalRunGit := runGit
	t.Cleanup(func() { runGit = originalRunGit })

	var calls [][]string
	runGit = func(args ...string) (string, error) {
		calls = append(calls, args)
		switch len(calls) {
		case 1:
			return "base-sha\n", nil
		case 2:
			return "committed.yaml\x00duplicate.yaml\x00", nil
		case 3:
			return "working.yaml\x00duplicate.yaml\x00", nil
		case 4:
			return "untracked.yaml\x00", nil
		default:
			return "", errors.New("unexpected git invocation")
		}
	}

	paths, err := AffectedFiles("main")
	require.NoError(t, err)
	assert.Equal(t, []string{"committed.yaml", "duplicate.yaml", "working.yaml", "untracked.yaml"}, paths)
	assert.Equal(t, []string{"merge-base", "HEAD", "main"}, calls[0])
	assert.Equal(t, []string{"diff", "--name-only", "-z", "--diff-filter=ACMRD", "base-sha...HEAD"}, calls[1])
}

func TestResolveAffectedBase(t *testing.T) {
	t.Setenv("GITHUB_BASE_REF", "main")

	base, explicit := resolveAffectedBase("abc123")
	assert.Equal(t, "abc123", base)
	assert.True(t, explicit)

	base, explicit = resolveAffectedBase("")
	assert.Equal(t, "origin/main", base)
	assert.False(t, explicit)
}
