//go:build mage

package main

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterRacePackages(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect []string
	}{
		{
			name:   "empty input",
			input:  "",
			expect: nil,
		},
		{
			name:  "drops the tests package and its subpackages, keeps everything else",
			input: "github.com/cloudposse/atmos/cmd\ngithub.com/cloudposse/atmos/tests\ngithub.com/cloudposse/atmos/tests/testhelpers\ngithub.com/cloudposse/atmos/pkg/toolchain\n",
			expect: []string{
				"github.com/cloudposse/atmos/cmd",
				"github.com/cloudposse/atmos/pkg/toolchain",
			},
		},
		{
			// A sibling package that merely starts with the same prefix
			// (e.g. a hypothetical .../testsomething) must not be treated as
			// a tests/ subpackage -- the prefix check requires the "/"
			// boundary, not just a string prefix.
			name:   "does not drop a package that only shares the tests prefix textually",
			input:  "github.com/cloudposse/atmos/testsomething\n",
			expect: []string{"github.com/cloudposse/atmos/testsomething"},
		},
		{
			name:   "skips blank lines and trims whitespace",
			input:  "\n  github.com/cloudposse/atmos/cmd  \n\n",
			expect: []string{"github.com/cloudposse/atmos/cmd"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, filterRacePackages(tt.input))
		})
	}
}

func TestRacePackages(t *testing.T) {
	argsFile := setUpFakePathBinary(t, "go")
	t.Setenv("ATMOS_MAGEFILES_FAKE_BIN_STDOUT",
		"github.com/cloudposse/atmos/cmd\ngithub.com/cloudposse/atmos/tests\n")

	root := t.TempDir()
	packages, err := racePackages(root)
	require.NoError(t, err)
	assert.Equal(t, []string{"github.com/cloudposse/atmos/cmd"}, packages)

	args := readFakeBinArgs(t, argsFile)
	assert.Equal(t, []string{"-C", root, "list", "./..."}, args)
}

func TestRacePackagesFromEnv(t *testing.T) {
	t.Run("TEST override bypasses go list entirely", func(t *testing.T) {
		setUpFakePathBinary(t, "go")
		t.Setenv("ATMOS_MAGEFILES_FAKE_BIN_EXIT", "1") // go list would fail if it were invoked.
		t.Setenv("TEST", "./cmd/... ./pkg/toolchain/...")

		packages, err := racePackagesFromEnv(t.TempDir())
		require.NoError(t, err)
		assert.Equal(t, []string{"./cmd/...", "./pkg/toolchain/..."}, packages)
	})

	t.Run("empty TEST falls through to go list", func(t *testing.T) {
		argsFile := setUpFakePathBinary(t, "go")
		t.Setenv("ATMOS_MAGEFILES_FAKE_BIN_STDOUT", "github.com/cloudposse/atmos/cmd\n")
		t.Setenv("TEST", "")

		packages, err := racePackagesFromEnv(t.TempDir())
		require.NoError(t, err)
		assert.Equal(t, []string{"github.com/cloudposse/atmos/cmd"}, packages)
		assert.NotEmpty(t, readFakeBinArgs(t, argsFile), "go list should have been invoked")
	})
}

// TestTestRace exercises the full Race() orchestration with TEST set, so
// exactly one `go` invocation happens (the final `go test` call) and its
// full argument list is directly inspectable.
func TestTestRace(t *testing.T) {
	t.Run("propagates repo-root resolution failure", func(t *testing.T) {
		t.Chdir(t.TempDir())
		err := Test{}.Race()
		require.ErrorIs(t, err, errMageRepoRootNotFound)
	})

	t.Run("builds go test args from TEST and TESTARGS", func(t *testing.T) {
		root := initGitRepoFixture(t)
		t.Chdir(root)
		argsFile := setUpFakePathBinary(t, "go")
		t.Setenv("TEST", "./cmd/... ./pkg/toolchain/...")
		t.Setenv("TESTARGS", "-run TestFoo -v")

		require.NoError(t, Test{}.Race())

		args := readFakeBinArgs(t, argsFile)
		assert.Equal(t, []string{
			"test", "-race", "-shuffle=on",
			"./cmd/...", "./pkg/toolchain/...",
			"-run", "TestFoo", "-v",
			"-timeout", raceTestTimeout,
		}, args)

		// Resolve both sides before comparing: t.TempDir() and the recorded
		// subprocess cwd can each observe /tmp through a different symlink
		// form (e.g. macOS's /tmp -> /private/tmp), independent of which one
		// os.Getwd() happens to return.
		wantDir, err := filepath.EvalSymlinks(root)
		require.NoError(t, err)
		gotDir, err := filepath.EvalSymlinks(readFakeBinCwd(t, argsFile))
		require.NoError(t, err)
		assert.Equal(t, wantDir, gotDir)
	})

	t.Run("propagates go test failure", func(t *testing.T) {
		root := initGitRepoFixture(t)
		t.Chdir(root)
		setUpFakePathBinary(t, "go")
		t.Setenv("TEST", "./cmd/...")
		t.Setenv("ATMOS_MAGEFILES_FAKE_BIN_EXIT", "1")

		err := Test{}.Race()
		require.Error(t, err)
	})
}
