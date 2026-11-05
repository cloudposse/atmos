//go:build mage

package test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/magefiles/internal/mageutil"
	"github.com/cloudposse/atmos/magefiles/internal/testutil"

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

// The failed CI run used this seed and packed expensive suites onto one runner.
func TestRaceShardMatrix_BalancesExpensiveSuites(t *testing.T) {
	expensive := []string{
		"github.com/cloudposse/atmos/internal/exec",
		"github.com/cloudposse/atmos/cmd",
		"github.com/cloudposse/atmos/pkg/toolchain",
		"github.com/cloudposse/atmos/pkg/describe",
	}
	packages := append([]string(nil), expensive...)
	for index := range 17 {
		packages = append(packages, fmt.Sprintf("example.com/package%d", index))
	}
	original := append([]string(nil), packages...)
	assert.NotEqual(t, raceShardMatrix(packages, 4, "40128500401"), raceShardMatrix(packages, 4, "40125786432"))
	for _, shardCount := range []int{1, 2, 3, 4, 6, 25} {
		t.Run(fmt.Sprintf("%d shards", shardCount), func(t *testing.T) {
			entries := raceShardMatrix(packages, shardCount, "40128500401")
			require.Len(t, entries, shardCount)
			assert.Equal(t, entries, raceShardMatrix(packages, shardCount, "40128500401"))
			assert.Equal(t, original, packages, "planning must not mutate the package inventory")
			var assigned, counts, expensiveCounts []int
			var allPackages []string
			for index, entry := range entries {
				assert.Equal(t, index+1, entry.Shard)
				bucket := strings.Fields(entry.Packages)
				allPackages = append(allPackages, bucket...)
				counts = append(counts, len(bucket))
				heavyCount := 0
				for _, pkg := range bucket {
					if slices.Contains(expensive, pkg) {
						heavyCount++
					}
				}
				expensiveCounts = append(expensiveCounts, heavyCount)
				if heavyCount > 0 {
					assigned = append(assigned, index)
				}
			}
			assert.ElementsMatch(t, packages, allPackages, "every package must run exactly once")
			assert.LessOrEqual(t, slices.Max(counts)-slices.Min(counts), 1)
			assert.LessOrEqual(t, slices.Max(expensiveCounts)-slices.Min(expensiveCounts), 1)
			assert.Len(t, assigned, min(shardCount, len(expensive)), "spread expensive suites over available runners")
		})
	}
}

func TestTestRaceMatrix(t *testing.T) {
	t.Run("propagates repo-root resolution failure", func(t *testing.T) {
		t.Chdir(t.TempDir())
		err := Test{}.RaceMatrix()
		require.ErrorIs(t, err, mageutil.ErrRepoRootNotFound)
	})

	t.Run("propagates package listing failure", func(t *testing.T) {
		root := testutil.InitGitRepoFixture(t)
		t.Chdir(root)
		testutil.SetUpFakePathBinary(t, "go")
		t.Setenv("ATMOS_MAGEFILES_FAKE_BIN_EXIT", "1")

		err := Test{}.RaceMatrix()
		require.ErrorContains(t, err, "mage: go list ./...")
	})

	invalidShardCounts := []struct {
		name  string
		value string
	}{
		{name: "missing RACE_SHARD_COUNT is an error", value: ""},
		{name: "invalid RACE_SHARD_COUNT is an error", value: "0"},
	}
	for _, testCase := range invalidShardCounts {
		t.Run(testCase.name, func(t *testing.T) {
			root := testutil.InitGitRepoFixture(t)
			t.Chdir(root)
			testutil.SetUpFakePathBinary(t, "go")
			t.Setenv(raceShardCountEnv, testCase.value)

			err := Test{}.RaceMatrix()
			require.ErrorIs(t, err, errInvalidRaceShardCount)
		})
	}

	t.Run("writes a shard/packages matrix to GITHUB_OUTPUT", func(t *testing.T) {
		root := testutil.InitGitRepoFixture(t)
		t.Chdir(root)
		testutil.SetUpFakePathBinary(t, "go")
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
