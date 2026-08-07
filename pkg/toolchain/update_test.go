package toolchain

import (
	"path/filepath"
	"strings"
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

// TestUpdateOneTool_ExactPin_InstallFailureLeavesToolVersionsUnchanged reproduces a bug where
// the exact-pin update path wrote the new "newest" version into .tool-versions as the default
// BEFORE installing it. If install then failed, .tool-versions pointed at a version that was
// never actually installed. This asserts install is attempted first, and a failed install
// leaves the previously-configured (and actually-installed) default version untouched.
func TestUpdateOneTool_ExactPin_InstallFailureLeavesToolVersionsUnchanged(t *testing.T) {
	setupTestIO(t)

	filePath := createTempToolVersionsFile(t, "hashicorp/terraform 1.9.8\n")

	// InstallPath MUST be isolated to a per-test temp dir: a failed install still touches
	// the real, shared, XDG toolchain cache directory otherwise (see install_test.go for the
	// same isolation requirement and rationale).
	prevConfig := atmosConfig
	t.Cleanup(func() { SetAtmosConfig(prevConfig) })
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{
		VersionsFile: filePath,
		InstallPath:  filepath.Join(t.TempDir(), ".tools"),
	}})

	// "99.99.99" is not a real hashicorp/terraform release, so the real (unmocked) install
	// step must fail -- this is the "newest" version fetchAllGitHubVersions reports via the
	// mocked GitHub API, but the mock only fakes the release listing, not the download.
	mock := NewMockGitHubAPI()
	mock.SetReleases("hashicorp", "terraform", []string{"1.9.8", "99.99.99"})
	SetGitHubAPI(mock)
	t.Cleanup(ResetGitHubAPI)

	outcome := updateOneTool("hashicorp/terraform", UpdateOptions{MaxConcurrency: 1})
	assert.Equal(t, updateResultFailed, outcome.result)

	toolVersions, err := LoadToolVersions(filePath)
	require.NoError(t, err)
	assert.Equal(t, []string{"1.9.8"}, toolVersions.Tools["hashicorp/terraform"],
		"a failed install must not overwrite .tool-versions with the un-installed version")
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
	filePath := createTempToolVersionsFile(t, "owner/a pr:1\nowner/b sha:ceb7526\nowner/c ref:main\n")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: filePath}})

	var err error
	output := captureCleanTestOutput(t, func() {
		err = RunUpdate(nil, UpdateOptions{MaxConcurrency: 4})
	})
	require.NoError(t, err, "all-skipped tools should not count as failures")

	// Assert the reported ordering matches the target (file) order, not completion order --
	// this is the guarantee runUpdatesConcurrently/reportUpdateOutcomes must preserve.
	idxA := strings.Index(output, "owner/a")
	idxB := strings.Index(output, "owner/b")
	idxC := strings.Index(output, "owner/c")
	require.NotEqual(t, -1, idxA, "expected owner/a in output, got %q", output)
	require.NotEqual(t, -1, idxB, "expected owner/b in output, got %q", output)
	require.NotEqual(t, -1, idxC, "expected owner/c in output, got %q", output)
	assert.Less(t, idxA, idxB, "owner/a must be reported before owner/b")
	assert.Less(t, idxB, idxC, "owner/b must be reported before owner/c")
}

// TestUpdateOneTool_ExactPin_ReplacesDefaultWithoutAccumulatingStaleVersions reproduces a
// field-test finding: a real, successful update replaces the resolved "newest" version via
// AddToolToVersionsAsDefault, but that helper only ever prepended the new version -- it never
// removed the old default (or, for a multi-version line, any other previously-pinned version).
// Every real update on an exact pin therefore left the just-replaced version (and any
// secondary versions) behind as permanent, ever-growing stale entries. AddVersionToTool's
// asDefault path now matches asdf's own "set" convention (asdf's docs describe `asdf set
// <tool> <version>` as equivalent to `echo "<tool> <version>" > .tool-versions`): the whole
// line becomes exactly the new version, full stop -- including dropping any other
// already-pinned secondary version, not just the old default. This hits the real network
// (matching this file's existing convention for exercising a real install, e.g.
// TestUpdateOneTool_ExactPin_InstallFailureLeavesToolVersionsUnchanged) since the bug only
// manifests after install actually succeeds.
func TestUpdateOneTool_ExactPin_ReplacesDefaultWithoutAccumulatingStaleVersions(t *testing.T) {
	setupTestIO(t)

	// A pre-existing secondary version (1.9.7) proves the line is fully replaced, not just
	// the default entry -- update's "never leaves two versions pinned" guarantee applies to
	// the whole line, not just index 0.
	filePath := createTempToolVersionsFile(t, "hashicorp/terraform 1.9.8 1.9.7\n")

	// InstallPath MUST be isolated to a per-test temp dir -- see the sibling
	// InstallFailureLeavesToolVersionsUnchanged test for the same isolation rationale.
	prevConfig := atmosConfig
	t.Cleanup(func() { SetAtmosConfig(prevConfig) })
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{
		VersionsFile: filePath,
		InstallPath:  filepath.Join(t.TempDir(), ".tools"),
	}})

	// "1.11.4" is a real, installable hashicorp/terraform release (already used by
	// TestUpdateOneTool_ExactPin_DryRunDoesNotMutate), so this real (unmocked) install
	// actually succeeds.
	mock := NewMockGitHubAPI()
	mock.SetReleases("hashicorp", "terraform", []string{"1.9.8", "1.11.4"})
	SetGitHubAPI(mock)
	t.Cleanup(ResetGitHubAPI)

	outcome := updateOneTool("hashicorp/terraform", UpdateOptions{MaxConcurrency: 1})
	require.Equal(t, updateResultUpdated, outcome.result, "update message: %s", outcome.message)

	toolVersions, err := LoadToolVersions(filePath)
	require.NoError(t, err)
	assert.Equal(t, []string{"1.11.4"}, toolVersions.Tools["hashicorp/terraform"],
		"a real update fully replaces the line -- neither the old default (1.9.8) nor the secondary version (1.9.7) may survive as stale entries")
}
