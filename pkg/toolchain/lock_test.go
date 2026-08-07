package toolchain

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRunLock_RejectsInvalidMaxConcurrency(t *testing.T) {
	err := RunLock(nil, LockOptions{MaxConcurrency: 0})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
}

func TestRunLock_NoToolsConfigured(t *testing.T) {
	setupTestIO(t)
	filePath := createTempToolVersionsFile(t, "")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: filePath}})

	err := RunLock(nil, LockOptions{MaxConcurrency: 4})
	require.NoError(t, err)
}

func TestRunLock_UnknownToolRequested(t *testing.T) {
	setupTestIO(t)
	filePath := createTempToolVersionsFile(t, "owner/repo 1.0.0\n")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: filePath}})

	err := RunLock([]string{"nonexistent-tool"}, LockOptions{MaxConcurrency: 4})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrToolNotFound)
}

// TestResolveLockTargets covers the empty-args-means-everything convention (sorted for
// determinism, since it's built from a map), per-version expansion of multi-version
// .tool-versions lines, and alias resolution to a tool's default version for explicit
// names.
func TestResolveLockTargets(t *testing.T) {
	toolVersions := &ToolVersions{Tools: map[string][]string{
		"hashicorp/terraform": {"1.11.4", "1.9.8"},
		"jqlang/jq":           {"jq-1.7.1"},
	}}

	t.Run("empty selects everything, sorted", func(t *testing.T) {
		targets, err := resolveLockTargets(toolVersions, nil)
		require.NoError(t, err)
		require.Len(t, targets, 3)
		// Sorted by owner, then repo, then version.
		assert.Equal(t, toolInfo{"1.11.4", "hashicorp", "terraform"}, targets[0])
		assert.Equal(t, toolInfo{"1.9.8", "hashicorp", "terraform"}, targets[1])
		assert.Equal(t, toolInfo{"jq-1.7.1", "jqlang", "jq"}, targets[2])
	})

	t.Run("explicit name resolves to default (first) version", func(t *testing.T) {
		targets, err := resolveLockTargets(toolVersions, []string{"hashicorp/terraform"})
		require.NoError(t, err)
		require.Len(t, targets, 1)
		assert.Equal(t, toolInfo{"1.11.4", "hashicorp", "terraform"}, targets[0])
	})

	t.Run("unknown tool errors", func(t *testing.T) {
		_, err := resolveLockTargets(toolVersions, []string{"nonexistent"})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrToolNotFound)
	})
}

// TestRunLock_ReportsInTargetOrder verifies that concurrent workers (MaxConcurrency > 1)
// report outcomes in the original target order, not completion order -- mirroring the
// same guarantee (and the same regression class) fixed for RunUpdate. Uses tools that
// don't exist in any registry, so every worker fails at the same FindTool step -- the
// property under test is reporting order, not success.
func TestRunLock_ReportsInTargetOrder(t *testing.T) {
	filePath := createTempToolVersionsFile(t, "nonexistent-owner/aaa 1.0.0\nnonexistent-owner/bbb 1.0.0\nnonexistent-owner/ccc 1.0.0\n")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: filePath}})

	var err error
	output := captureCleanTestOutput(t, func() {
		err = RunLock(nil, LockOptions{MaxConcurrency: 4})
	})
	require.Error(t, err, "tools that don't exist in any registry should be reported as failed")

	idxA := strings.Index(output, "aaa")
	idxB := strings.Index(output, "bbb")
	idxC := strings.Index(output, "ccc")
	require.NotEqual(t, -1, idxA, "expected aaa in output, got %q", output)
	require.NotEqual(t, -1, idxB, "expected bbb in output, got %q", output)
	require.NotEqual(t, -1, idxC, "expected ccc in output, got %q", output)
	assert.Less(t, idxA, idxB, "aaa must be reported before bbb")
	assert.Less(t, idxB, idxC, "bbb must be reported before ccc")
}
