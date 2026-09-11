//go:build mage

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShuffleRacePackages(t *testing.T) {
	packages := []string{"a", "b", "c", "d", "e", "f", "g", "h"}

	t.Run("same seed always produces the same order", func(t *testing.T) {
		first := shuffleRacePackages(packages, "run-123")
		second := shuffleRacePackages(packages, "run-123")
		assert.Equal(t, first, second)
	})

	t.Run("different seeds can produce different orders", func(t *testing.T) {
		first := shuffleRacePackages(packages, "run-123")
		second := shuffleRacePackages(packages, "run-456")
		assert.NotEqual(t, first, second)
	})

	t.Run("shuffle is a permutation, not a lossy transform", func(t *testing.T) {
		got := shuffleRacePackages(packages, "run-123")
		assert.ElementsMatch(t, packages, got)
	})

	t.Run("does not mutate the input slice", func(t *testing.T) {
		original := append([]string(nil), packages...)
		shuffleRacePackages(packages, "run-123")
		assert.Equal(t, original, packages)
	})
}

func TestRaceShardMatrix(t *testing.T) {
	packages := []string{
		"github.com/cloudposse/atmos/cmd",
		"github.com/cloudposse/atmos/pkg/toolchain",
		"github.com/cloudposse/atmos/pkg/store",
		"github.com/cloudposse/atmos/internal/exec",
		"github.com/cloudposse/atmos/pkg/schema",
	}

	entries := raceShardMatrix(packages, 3, "run-123")
	require.Len(t, entries, 3)

	var allAssigned []string
	for index, entry := range entries {
		assert.Equal(t, index+1, entry.Shard)
		allAssigned = append(allAssigned, strings.Fields(entry.Packages)...)
	}
	assert.ElementsMatch(t, packages, allAssigned)
}

func TestTestRaceMatrix(t *testing.T) {
	t.Run("propagates repo-root resolution failure", func(t *testing.T) {
		t.Chdir(t.TempDir())
		err := Test{}.RaceMatrix()
		require.ErrorIs(t, err, errMageRepoRootNotFound)
	})

	t.Run("missing RACE_SHARD_COUNT is an error", func(t *testing.T) {
		root := initGitRepoFixture(t)
		t.Chdir(root)
		setUpFakePathBinary(t, "go")
		t.Setenv(raceShardCountEnv, "")

		err := Test{}.RaceMatrix()
		require.ErrorIs(t, err, errInvalidRaceShardCount)
	})

	t.Run("invalid RACE_SHARD_COUNT is an error", func(t *testing.T) {
		root := initGitRepoFixture(t)
		t.Chdir(root)
		setUpFakePathBinary(t, "go")
		t.Setenv(raceShardCountEnv, "0")

		err := Test{}.RaceMatrix()
		require.ErrorIs(t, err, errInvalidRaceShardCount)
	})

	t.Run("writes a shard/packages matrix to GITHUB_OUTPUT", func(t *testing.T) {
		root := initGitRepoFixture(t)
		t.Chdir(root)
		setUpFakePathBinary(t, "go")
		t.Setenv("ATMOS_MAGEFILES_FAKE_BIN_STDOUT",
			"github.com/cloudposse/atmos/cmd\ngithub.com/cloudposse/atmos/pkg/toolchain\ngithub.com/cloudposse/atmos/tests\n")
		t.Setenv(raceShardCountEnv, "2")
		t.Setenv(raceShardSeedEnv, "run-123")

		outputFile := filepath.Join(t.TempDir(), "github_output")
		t.Setenv("GITHUB_OUTPUT", outputFile)

		require.NoError(t, Test{}.RaceMatrix())

		content, err := os.ReadFile(outputFile)
		require.NoError(t, err)

		matrixJSON := extractMatrixJSON(t, string(content))
		var payload struct {
			Include []raceShardEntry `json:"include"`
		}
		require.NoError(t, json.Unmarshal([]byte(matrixJSON), &payload))
		require.Len(t, payload.Include, 2)

		var allAssigned []string
		for _, entry := range payload.Include {
			allAssigned = append(allAssigned, strings.Fields(entry.Packages)...)
		}
		// The tests/ package is filtered out by racePackages before RaceMatrix
		// ever sees it (same exclusion Test.Race relies on).
		assert.ElementsMatch(t, []string{
			"github.com/cloudposse/atmos/cmd",
			"github.com/cloudposse/atmos/pkg/toolchain",
		}, allAssigned)
	})
}

// extractMatrixJSON pulls the JSON payload out of a "matrix=<json>\n..."
// $GITHUB_OUTPUT-formatted file (see pkg/matrix.writeToFile).
func extractMatrixJSON(t *testing.T, content string) string {
	t.Helper()
	for _, line := range strings.Split(content, "\n") {
		if rest, ok := strings.CutPrefix(line, "matrix="); ok {
			return rest
		}
	}
	t.Fatalf("no matrix= line found in %q", content)
	return ""
}
