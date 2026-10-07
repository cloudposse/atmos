package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestInitOccupiedTargetFailsBeforeConfigLoading(t *testing.T) {
	t.Chdir(t.TempDir())
	require.NoError(t, os.Mkdir("pacts", 0o755))
	require.NoError(t, os.Mkdir("target", 0o755))
	require.NoError(t, os.WriteFile(filepath.Join("target", "file"), []byte("keep"), 0o600))
	require.NoError(t, os.WriteFile("atmos.yaml", []byte("invalid: ["), 0o600))
	originalArgs := os.Args
	os.Args = []string{"atmos", "init", "pacts", "target"}
	t.Cleanup(func() { os.Args = originalArgs })
	require.ErrorIs(t, Execute(), errUtils.ErrTargetDirectoryNotEmpty)
}

func TestShowInitConfigProgress(t *testing.T) {
	originalArgs := os.Args
	os.Args = []string{"atmos"}
	t.Cleanup(func() { os.Args = originalArgs })
	for _, tc := range []struct {
		args []string
		want bool
	}{
		{[]string{"init", "examples/quick-start-simple"}, true},
		{[]string{"--chdir", "/tmp", "init", "./source"}, true},
		{[]string{"init", "--help"}, false},
		{[]string{"help", "init"}, false},
		{[]string{"git", "init"}, false},
		{[]string{"terraform", "init"}, false},
		{[]string{"list", "stacks"}, false},
		{[]string{"version"}, false},
		{[]string{"__complete", "init", ""}, false},
	} {
		assert.Equal(t, tc.want, showInitConfigProgress(RootCmd, tc.args), "%v", tc.args)
	}
}
