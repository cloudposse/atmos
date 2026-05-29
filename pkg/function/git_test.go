package function

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitFunctions_Metadata(t *testing.T) {
	tests := []struct {
		name string
		fn   Function
		tag  string
	}{
		{name: "git sha", fn: NewGitShaFunction(), tag: TagGitSha},
		{name: "git branch", fn: NewGitBranchFunction(), tag: TagGitBranch},
		{name: "git ref", fn: NewGitRefFunction(), tag: TagGitRef},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, tt.fn)
			assert.Equal(t, tt.tag, tt.fn.Name())
			assert.Equal(t, PreMerge, tt.fn.Phase())
			assert.Empty(t, tt.fn.Aliases())
		})
	}
}

func TestGitFunctions_ExecuteWithFallbackOutsideRepository(t *testing.T) {
	tmpDir := t.TempDir()
	t.Chdir(tmpDir)

	tests := []struct {
		name     string
		fn       Function
		args     string
		expected string
	}{
		{name: "git sha", fn: NewGitShaFunction(), args: "unknown", expected: "unknown"},
		{name: "git branch", fn: NewGitBranchFunction(), args: "detached", expected: "detached"},
		{name: "git ref", fn: NewGitRefFunction(), args: "unknown", expected: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.fn.Execute(context.Background(), tt.args, nil)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
