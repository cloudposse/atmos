//go:build mage

package build

import (
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// outputArgument returns the value following -o in the go build arguments.
func outputArgument(t *testing.T, args []string) string {
	t.Helper()
	for index, arg := range args {
		if arg == "-o" && index+1 < len(args) {
			return args[index+1]
		}
	}
	require.Fail(t, "go build arguments must include -o", "%v", args)
	return ""
}

func TestBuildBinaryOutputOverride(t *testing.T) {
	root := testutil.InitGitRepoFixture(t)
	t.Chdir(root)
	t.Setenv("GOCACHE", t.TempDir())

	t.Run("default output is build/atmos", func(t *testing.T) {
		argsFile := testutil.SetUpFakePathBinary(t, "go")
		t.Setenv("ATMOS_BUILD_OUTPUT", "")
		require.NoError(t, Build{}.Binary("linux", "test"))
		assert.Equal(t, outputPathFor(goosLinux), outputArgument(t, testutil.ReadFakeBinArgs(t, argsFile)))
	})

	t.Run("absolute override replaces the output", func(t *testing.T) {
		argsFile := testutil.SetUpFakePathBinary(t, "go")
		override := filepath.Join(t.TempDir(), "nested", "atmos-runner")
		t.Setenv("ATMOS_BUILD_OUTPUT", override)
		require.NoError(t, Build{}.Binary("linux", "test"))
		assert.Equal(t, override, outputArgument(t, testutil.ReadFakeBinArgs(t, argsFile)))
	})

	t.Run("relative override is passed through for go build to resolve from the repository root", func(t *testing.T) {
		argsFile := testutil.SetUpFakePathBinary(t, "go")
		t.Setenv("ATMOS_BUILD_OUTPUT", filepath.Join(".atmos", "bin", "atmos"))
		require.NoError(t, Build{}.Binary("linux", "test"))
		assert.Equal(t, filepath.Join(".atmos", "bin", "atmos"), outputArgument(t, testutil.ReadFakeBinArgs(t, argsFile)))
	})
}
