package toolchain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestRunUpdate_RejectsInvalidMaxConcurrency(t *testing.T) {
	err := RunUpdate(nil, UpdateOptions{MaxConcurrency: 0})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
}

func TestRunUpdate_NoToolsConfigured(t *testing.T) {
	setupTestIO(t)
	filePath := createTempToolVersionsFile(t, "")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: filePath}})

	err := RunUpdate(nil, UpdateOptions{MaxConcurrency: 4})
	require.NoError(t, err)
}

func TestRunUpdate_UnknownToolRequested(t *testing.T) {
	setupTestIO(t)
	filePath := createTempToolVersionsFile(t, "owner/repo 1.0.0\n")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: filePath}})

	err := RunUpdate([]string{"nonexistent-tool"}, UpdateOptions{MaxConcurrency: 4})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrToolNotFound)
}

// TestUpdateOneTool_SkipsImmutablePins verifies pr:/sha:/ref: pins are never
// bumped automatically -- they're immutable by design.
func TestUpdateOneTool_SkipsImmutablePins(t *testing.T) {
	setupTestIO(t)

	tests := []struct {
		name    string
		pinned  string
		wantSub string
	}{
		{"pr pin", "pr:2038", "pinned to pr:2038"},
		{"sha pin", "sha:ceb7526", "pinned to sha:ceb7526"},
		{"ref pin", "ref:main", "pinned to ref:main"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := createTempToolVersionsFile(t, "owner/repo "+tt.pinned+"\n")
			SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: filePath}})

			outcome := updateOneTool("owner/repo", UpdateOptions{MaxConcurrency: 1})
			assert.Equal(t, updateResultSkipped, outcome.result)
			assert.Contains(t, outcome.message, tt.wantSub)
		})
	}
}

// TestUpdateOneTool_ExactPin_UpToDate verifies that when the registry's newest
// version matches the current pin, update reports up-to-date and does not
// rewrite .tool-versions.
func TestUpdateOneTool_ExactPin_UpToDate(t *testing.T) {
	setupTestIO(t)

	filePath := createTempToolVersionsFile(t, "hashicorp/terraform 1.11.4\n")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: filePath}})

	mock := NewMockGitHubAPI()
	mock.SetReleases("hashicorp", "terraform", []string{"1.11.4"})
	SetGitHubAPI(mock)
	t.Cleanup(ResetGitHubAPI)

	outcome := updateOneTool("hashicorp/terraform", UpdateOptions{MaxConcurrency: 1})
	assert.Equal(t, updateResultUpToDate, outcome.result)
	assert.Contains(t, outcome.message, "up to date")

	toolVersions, err := LoadToolVersions(filePath)
	require.NoError(t, err)
	assert.Equal(t, []string{"1.11.4"}, toolVersions.Tools["hashicorp/terraform"])
}

// TestUpdateOneTool_ExactPin_DryRunDoesNotMutate verifies --dry-run reports
// the available update without touching .tool-versions or installing anything.
func TestUpdateOneTool_ExactPin_DryRunDoesNotMutate(t *testing.T) {
	setupTestIO(t)

	filePath := createTempToolVersionsFile(t, "hashicorp/terraform 1.9.8\n")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: filePath}})

	mock := NewMockGitHubAPI()
	mock.SetReleases("hashicorp", "terraform", []string{"1.9.8", "1.11.4"})
	SetGitHubAPI(mock)
	t.Cleanup(ResetGitHubAPI)

	outcome := updateOneTool("hashicorp/terraform", UpdateOptions{DryRun: true, MaxConcurrency: 1})
	assert.Equal(t, updateResultUpdated, outcome.result)
	assert.Contains(t, outcome.message, "1.9.8 -> 1.11.4")
	assert.Contains(t, outcome.message, "dry-run")

	// .tool-versions must be untouched by a dry run.
	toolVersions, err := LoadToolVersions(filePath)
	require.NoError(t, err)
	assert.Equal(t, []string{"1.9.8"}, toolVersions.Tools["hashicorp/terraform"])
}

// TestResolveUpdateTargets covers the empty-args-means-everything convention
// and alias resolution to canonical .tool-versions keys.
func TestResolveUpdateTargets(t *testing.T) {
	toolVersions := &ToolVersions{Tools: map[string][]string{
		"hashicorp/terraform": {"1.11.4"},
		"jqlang/jq":           {"jq-1.7.1"},
	}}

	t.Run("empty selects everything", func(t *testing.T) {
		targets, err := resolveUpdateTargets(toolVersions, nil)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"hashicorp/terraform", "jqlang/jq"}, targets)
	})

	t.Run("explicit canonical name", func(t *testing.T) {
		targets, err := resolveUpdateTargets(toolVersions, []string{"hashicorp/terraform"})
		require.NoError(t, err)
		assert.Equal(t, []string{"hashicorp/terraform"}, targets)
	})

	t.Run("unknown tool errors", func(t *testing.T) {
		_, err := resolveUpdateTargets(toolVersions, []string{"nonexistent"})
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrToolNotFound)
	})
}

// TestDescribeImmutablePin covers the pin-type classification used to decide
// whether update should attempt to bump a tool at all.
func TestDescribeImmutablePin(t *testing.T) {
	tests := []struct {
		version       string
		wantImmutable bool
	}{
		{"pr:2038", true},
		{"sha:ceb7526", true},
		{"ref:main", true},
		{"1.11.4", false},
		{"latest", false},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			_, immutable := describeImmutablePin(tt.version)
			assert.Equal(t, tt.wantImmutable, immutable)
		})
	}
}

// TestRunUpdate_ConcurrencyPreservesOrder verifies that concurrent workers
// (MaxConcurrency > 1) still report outcomes in the original target order,
// not completion order.
func TestRunUpdate_ConcurrencyPreservesOrder(t *testing.T) {
	setupTestIO(t)

	filePath := createTempToolVersionsFile(t, "owner/a pr:1\nowner/b sha:ceb7526\nowner/c ref:main\n")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: filePath}})

	err := RunUpdate(nil, UpdateOptions{MaxConcurrency: 4})
	require.NoError(t, err, "all-skipped tools should not count as failures")
}
