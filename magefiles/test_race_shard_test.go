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

	"github.com/cloudposse/atmos/internal/ci/testshard"
)

func TestRaceShardExecution(t *testing.T) {
	root := initGitRepoFixture(t)
	t.Chdir(root)
	argsFile := setUpFakePathBinary(t, "go")
	t.Setenv("ATMOS_TEST_RACE_SHARD", "2")
	t.Setenv(raceShardCountEnv, "2")
	t.Setenv("TEST", "")
	t.Setenv("TESTARGS", "-v")
	t.Setenv("ATMOS_MAGEFILES_FAKE_BIN_STDOUT", "TestA\nTestB\nTestC\n")
	report := t.TempDir()
	t.Setenv("ATMOS_TEST_RACE_REPORT_DIR", report)
	require.NoError(t, Test{}.Race())
	args := readFakeBinArgs(t, argsFile)
	assert.Contains(t, args, "^(TestB)$")
	assert.Contains(t, args, "-race")
	assert.Contains(t, args, "-count=1")
	assert.Equal(t, raceToolchainPackage, args[len(args)-1])
	data, err := os.ReadFile(filepath.Join(report, "plan-2.json"))
	require.NoError(t, err)
	var plan struct{ Discovered, Selected []string }
	require.NoError(t, json.Unmarshal(data, &plan))
	assert.Equal(t, []string{"TestA", "TestB", "TestC"}, plan.Discovered)
	assert.Equal(t, []string{"TestB"}, plan.Selected)
	assert.FileExists(t, filepath.Join(report, "toolchain.json"))
}

func TestRaceShardStillRunsOtherPackagesAfterDiscoveryFailure(t *testing.T) {
	root := initGitRepoFixture(t)
	t.Chdir(root)
	argsFile := setUpFakePathBinary(t, "go")
	t.Setenv("ATMOS_TEST_RACE_SHARD", "1")
	t.Setenv(raceShardCountEnv, "4")
	t.Setenv("TEST", "./pkg/other")
	t.Setenv("TESTARGS", "")
	t.Setenv("ATMOS_MAGEFILES_FAKE_BIN_EXIT", "1")
	err := Test{}.Race()
	require.ErrorContains(t, err, "list toolchain race tests")
	args := readFakeBinArgs(t, argsFile)
	assert.Contains(t, args, "./pkg/other")
	assert.NotContains(t, args, "-run")
}

func TestRaceShardRejectsIncompleteSelection(t *testing.T) {
	for _, args := range []string{"-run TestA", "-short", "-skip=TestA", "-args", "'"} {
		t.Run(args, func(t *testing.T) {
			t.Setenv("TESTARGS", args)
			_, err := raceShardArgs()
			require.Error(t, err)
		})
	}
	t.Setenv("ATMOS_TEST_RACE_SHARD", "5")
	t.Setenv(raceShardCountEnv, "4")
	require.Error(t, runRaceShard(t.TempDir()))
	t.Setenv("ATMOS_TEST_RACE_SHARD", "1")
	t.Setenv("TESTARGS", "")
	t.Setenv("TEST", raceToolchainPackage)
	require.ErrorIs(t, runRaceShard(t.TempDir()), testshard.ErrPlan)
}

func TestRaceTimingHints(t *testing.T) {
	root := t.TempDir()
	weights, err := readRaceWeights(root)
	require.NoError(t, err)
	assert.Nil(t, weights)
	directory := filepath.Join(root, ".github", "test-timings")
	require.NoError(t, os.MkdirAll(directory, 0o755))
	path := filepath.Join(directory, "toolchain.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"seconds":{"TestA":42}}`), 0o600))
	weights, err = readRaceWeights(root)
	require.NoError(t, err)
	assert.Equal(t, map[string]float64{"TestA": 42}, weights)
	require.NoError(t, os.WriteFile(path, []byte("["), 0o600))
	_, err = readRaceWeights(root)
	require.ErrorContains(t, err, "parse race timing hints")
}

func TestRaceMatrixExcludesOnlyToolchainRoot(t *testing.T) {
	entries := raceShardMatrix([]string{raceToolchainPackage, raceToolchainPackage + "/installer"}, 4, "seed")
	var packages []string
	for _, entry := range entries {
		packages = append(packages, strings.Fields(entry.Packages)...)
	}
	assert.Equal(t, []string{raceToolchainPackage + "/installer"}, packages)
}

func TestRaceCommandFailureKeepsTimingArtifacts(t *testing.T) {
	root := t.TempDir()
	setUpFakePathBinary(t, "go")
	t.Setenv(fakeBinExitEnv, "1")
	t.Setenv(fakeBinStdoutEnv, `{"Action":"fail","Package":"fixture","Test":"TestFailure","Elapsed":2}`+"\n")
	reports := t.TempDir()
	t.Setenv("ATMOS_TEST_RACE_REPORT_DIR", reports)
	require.ErrorContains(t, runRaceCommand(root, "toolchain", []string{"test", "-race"}), "race toolchain")
	data, err := os.ReadFile(filepath.Join(reports, "toolchain.json"))
	require.NoError(t, err)
	var summary testshard.Timings
	require.NoError(t, json.Unmarshal(data, &summary))
	assert.Equal(t, map[string]float64{"TestFailure": 2}, summary.Tests["fixture"])
	assert.FileExists(t, filepath.Join(reports, "toolchain.jsonl"))
}

func TestRaceEmptyAssignmentDoesNotRunAllTests(t *testing.T) {
	argsFile := setUpFakePathBinary(t, "go")
	t.Setenv(fakeBinStdoutEnv, "TestOnly\n")
	t.Setenv("ATMOS_TEST_RACE_SHARD", "4")
	t.Setenv(raceShardCountEnv, "4")
	t.Setenv("TEST", "")
	t.Setenv("TESTARGS", "")
	require.NoError(t, runRaceShard(t.TempDir()))
	args := readFakeBinArgs(t, argsFile)
	assert.Contains(t, args, "-list", "discovery must remain the last command; no empty -run")
	assert.NotContains(t, args, "-run")
}
