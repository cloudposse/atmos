package tests

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDescribeAffectedGreenfieldBase verifies that `atmos describe affected` succeeds (and reports
// all HEAD components as affected) when the BASE does not contain any Atmos configuration at all.
//
// This is the "greenfield" scenario: a branch that introduces both Atmos and its CI/CD workflows
// for the first time.  The base ref has no `atmos.yaml` or stacks directory, so processing it
// previously returned an unhelpful `Error: failed to find import` instead of treating the absence
// of Atmos configuration as an empty baseline.
func TestDescribeAffectedGreenfieldBase(t *testing.T) {
	basePath := filepath.Join("tests", "fixtures", "scenarios", "atmos-describe-affected")
	pathPrefix := ".."

	stacksPath := filepath.Join(pathPrefix, basePath)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	// Create a temp dir and initialise a bare-minimum git repo inside it.
	// The repo has a single commit but NO Atmos stack files — this simulates the greenfield base.
	tempDir := t.TempDir()

	repo, err := gogit.PlainInit(tempDir, false)
	require.NoError(t, err)

	// Write a minimal README so the initial commit is non-empty.
	readmePath := filepath.Join(tempDir, "README.md")
	err = os.WriteFile(readmePath, []byte("# Greenfield base\n"), 0o600)
	require.NoError(t, err)

	w, err := repo.Worktree()
	require.NoError(t, err)

	_, err = w.Add("README.md")
	require.NoError(t, err)

	_, err = w.Commit("initial commit", &gogit.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@test.com",
			When:  time.Unix(1_000_000, 0),
		},
	})
	require.NoError(t, err)

	// Set BasePath so that executeDescribeAffected can compute the relative
	// offset from the local repo root to the stacks directory.
	atmosConfig.BasePath = basePath

	t.Run("all HEAD components are affected when BASE has no Atmos setup", func(t *testing.T) {
		affected, _, _, _, err := e.ExecuteDescribeAffectedWithTargetRepoPath(
			&atmosConfig,
			tempDir,
			false, // includeSpaceliftAdminStacks
			false, // includeSettings
			"",    // stack filter
			false, // processTemplates
			false, // processYamlFunctions
			nil,   // skip
			false, // excludeLocked
			nil,   // authManager
		)

		require.NoError(t, err,
			"describe affected should succeed even when the BASE branch has no Atmos configuration")

		// Every component that exists in HEAD must be in the affected list
		// because there is no corresponding state in the empty BASE.
		assert.Greater(t, len(affected), 0,
			"at least one component should be reported as affected on a greenfield base")

		// Confirm that no error was silently swallowed for a wrong reason.
		for _, a := range affected {
			assert.NotEmpty(t, a.Component, "affected entry should have a component name")
			assert.NotEmpty(t, a.Stack, "affected entry should have a stack name")
		}
	})
}
