package toolchain

import (
	"os"
	"path/filepath"
	"runtime"
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

// TestUpdateOneTool_ExactPin_VPrefixMismatchIsUpToDate reproduces a live-usage bug: a tool
// pinned with a literal "v" prefix (e.g. "peteretelej/tree v1.3.0", written by an earlier `add`
// that captured the tool's real GitHub tag) was always reported as "updated" even when nothing
// changed, because pkg/github.GetReleaseVersions unconditionally strips the leading "v" from
// fetched release tags before returning them, but updateExactPinnedTool compared the (v-less)
// newest version against the (v-prefixed) current pin with a raw string equality check. This
// caused three compounding problems on every affected tool's first update run: a misleading
// "v1.3.0 -> 1.3.0" message implying a real version bump, a wrongly-incremented "updated" count
// in the summary instead of "up to date", and a wholly unnecessary reinstall + .tool-versions
// rewrite for a tool that was already current.
func TestUpdateOneTool_ExactPin_VPrefixMismatchIsUpToDate(t *testing.T) {
	setupTestIO(t)

	filePath := createTempToolVersionsFile(t, "peteretelej/tree v1.3.0\n")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: filePath}})

	// Mirrors pkg/github.GetReleaseVersions's real behavior: fetched releases always have the
	// "v" prefix stripped, regardless of what's pinned in .tool-versions.
	mock := NewMockGitHubAPI()
	mock.SetReleases("peteretelej", "tree", []string{"1.3.0"})
	SetGitHubAPI(mock)
	t.Cleanup(ResetGitHubAPI)

	outcome := updateOneTool("peteretelej/tree", UpdateOptions{MaxConcurrency: 1})
	assert.Equal(t, updateResultUpToDate, outcome.result, "message: %s", outcome.message)
	assert.Contains(t, outcome.message, "up to date")
	assert.NotContains(t, outcome.message, "->", "must not report a version bump when only the 'v' prefix differs")

	// No reinstall or rewrite should have happened -- the pin must survive exactly as written.
	toolVersions, err := LoadToolVersions(filePath)
	require.NoError(t, err)
	assert.Equal(t, []string{"v1.3.0"}, toolVersions.Tools["peteretelej/tree"])
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
// TestRunUpdate_ConcurrentAllSkippedReportsEveryTarget confirms every target is reported
// exactly once and none count as a failure. Lines now print live as each tool completes (via
// runConcurrentBatchWithLiveProgress, matching `atmos toolchain install`'s batch-mode
// convention) instead of being buffered and printed in original target order after the whole
// batch finishes -- so completion order, not target order, is what's observable here. See
// TestRunConcurrentBatchWithLiveProgress_ResultsPreserveItemOrder (batch_progress_test.go) for
// the guarantee that still holds regardless of completion order: per-item results are never
// misattributed to the wrong item.
//
// Uses captureUITestOutput, not captureCleanTestOutput: the latter forces TTY mode and redirects
// stderr through an os.Pipe that's only drained after the tested function returns. Combined with
// this test's real concurrent batch (which activates the live, ticker-driven renderer under
// force-tty), that redirect deadlocked on Windows CI: the renderer's repeated writes filled the
// pipe's bounded OS buffer, and since nothing reads it until RunUpdate returns, the write blocked
// forever, hanging the test for the full 40-minute Go test timeout. See
// docs/fixes/2026-08-08-toolchain-live-renderer-windows-ci-deadlock.md.
func TestRunUpdate_ConcurrentAllSkippedReportsEveryTarget(t *testing.T) {
	filePath := createTempToolVersionsFile(t, "owner/a pr:1\nowner/b sha:ceb7526\nowner/c ref:main\n")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: filePath}})

	var err error
	output := captureUITestOutput(t, func() {
		err = RunUpdate(nil, UpdateOptions{MaxConcurrency: 4})
	})
	require.NoError(t, err, "all-skipped tools should not count as failures")

	assert.Contains(t, output, "owner/a")
	assert.Contains(t, output, "owner/b")
	assert.Contains(t, output, "owner/c")
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

// TestRunUpdate_ToolVersionsFileLoadError verifies RunUpdate surfaces a wrapped
// ErrToolVersionsFileOperation when the configured .tool-versions file can't be read.
func TestRunUpdate_ToolVersionsFileLoadError(t *testing.T) {
	setupTestIO(t)
	missing := filepath.Join(t.TempDir(), "does-not-exist-dir", ".tool-versions")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: missing}})
	t.Cleanup(func() { SetAtmosConfig(nil) })

	err := RunUpdate(nil, UpdateOptions{MaxConcurrency: 4})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrToolVersionsFileOperation)
}

// TestRunUpdate_ReportsFailedCount verifies RunUpdate returns ErrToolInstall (not nil) when at
// least one tool update fails, using a mocked GitHub API error so the failure is deterministic
// and doesn't depend on network flakiness.
func TestRunUpdate_ReportsFailedCount(t *testing.T) {
	filePath := createTempToolVersionsFile(t, "hashicorp/terraform 1.9.8\n")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: filePath}})
	t.Cleanup(func() { SetAtmosConfig(nil) })

	mock := NewMockGitHubAPI()
	mock.SetError("hashicorp", "terraform", assert.AnError)
	SetGitHubAPI(mock)
	t.Cleanup(ResetGitHubAPI)

	var err error
	output := captureUITestOutput(t, func() {
		err = RunUpdate(nil, UpdateOptions{MaxConcurrency: 1})
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrToolInstall)
	assert.Contains(t, output, "failed to fetch available versions")
	assert.Contains(t, output, "1 failed")
}

// TestRenderUpdateOutcome covers every result -> (style) mapping renderUpdateOutcome makes for
// the live-progress display, including the updateResultUpdated/updateResultUpToDate fallthrough
// to the success style.
func TestRenderUpdateOutcome(t *testing.T) {
	tests := []struct {
		name      string
		result    updateResult
		wantStyle batchLineStyle
	}{
		{"updated renders as success", updateResultUpdated, batchLineSuccess},
		{"up to date renders as success", updateResultUpToDate, batchLineSuccess},
		{"skipped renders as info", updateResultSkipped, batchLineInfo},
		{"failed renders as error", updateResultFailed, batchLineError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcome := updateOutcome{result: tt.result, message: "some message"}
			line, style := renderUpdateOutcome(outcome)
			assert.Equal(t, outcome.message, line)
			assert.Equal(t, tt.wantStyle, style)
		})
	}
}

// TestTallyUpdateOutcomes_MixedResults covers every case branch of tallyUpdateOutcomes' tally
// switch and the resulting summary line printUpdateSummary builds from it.
func TestTallyUpdateOutcomes_MixedResults(t *testing.T) {
	outcomes := []updateOutcome{
		{result: updateResultUpdated},
		{result: updateResultUpToDate},
		{result: updateResultUpToDate},
		{result: updateResultSkipped},
		{result: updateResultFailed},
	}

	var failed int
	output := captureUITestOutput(t, func() {
		failed = tallyUpdateOutcomes(outcomes, false)
	})

	assert.Equal(t, 1, failed)
	assert.Contains(t, output, "Updated 1 tool(s)")
	assert.Contains(t, output, "2 up to date")
	assert.Contains(t, output, "1 skipped")
	assert.Contains(t, output, "1 failed")
}

// TestPrintUpdateSummary_NoOutcomes_NonDryRun covers printUpdateSummary's defensive
// zero-outcomes branch for a real (non-dry-run) update.
func TestPrintUpdateSummary_NoOutcomes_NonDryRun(t *testing.T) {
	output := captureUITestOutput(t, func() {
		printUpdateSummary(0, 0, 0, 0, false)
	})
	assert.Contains(t, output, "Updated 0 tool(s)")
}

// TestPrintUpdateSummary_NoOutcomes_DryRun covers both the dry-run verb switch ("Would update"
// instead of "Updated") and the same zero-outcomes defensive branch for a --dry-run run.
func TestPrintUpdateSummary_NoOutcomes_DryRun(t *testing.T) {
	output := captureUITestOutput(t, func() {
		printUpdateSummary(0, 0, 0, 0, true)
	})
	assert.Contains(t, output, "Would update 0 tool(s)")
}

// TestUpdateOneTool_ToolVersionsLoadError covers updateOneTool's own (independent) load
// failure, distinct from RunUpdate's up-front load: updateOneTool reloads .tool-versions itself
// on every call since it may run concurrently with writes from sibling workers.
func TestUpdateOneTool_ToolVersionsLoadError(t *testing.T) {
	setupTestIO(t)
	missing := filepath.Join(t.TempDir(), "does-not-exist-dir", ".tool-versions")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: missing}})
	t.Cleanup(func() { SetAtmosConfig(nil) })

	outcome := updateOneTool("hashicorp/terraform", UpdateOptions{MaxConcurrency: 1})
	assert.Equal(t, updateResultFailed, outcome.result)
	assert.Contains(t, outcome.message, "failed to load .tool-versions")
}

// TestUpdateOneTool_NotConfigured covers the "tool no longer present in .tool-versions" branch
// -- e.g. a race where another process removed it between target resolution and this worker
// running.
func TestUpdateOneTool_NotConfigured(t *testing.T) {
	setupTestIO(t)
	filePath := createTempToolVersionsFile(t, "hashicorp/terraform 1.9.8\n")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: filePath}})
	t.Cleanup(func() { SetAtmosConfig(nil) })

	outcome := updateOneTool("owner/other-tool", UpdateOptions{MaxConcurrency: 1})
	assert.Equal(t, updateResultFailed, outcome.result)
	assert.Contains(t, outcome.message, "not configured in .tool-versions")
}

// TestUpdateOneTool_InvalidToolSpec_PropagatesParseError covers updateOneTool's ParseToolSpec
// error branch using the same "more than one slash" malformed-key case as
// TestResolveLockTargets_InvalidResolvedKey_PropagatesParseError (lock_test.go): a raw
// .tool-versions key is a free-form token, not validated as owner/repo at parse time.
func TestUpdateOneTool_InvalidToolSpec_PropagatesParseError(t *testing.T) {
	setupTestIO(t)
	filePath := createTempToolVersionsFile(t, "a/b/c 1.0.0\n")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: filePath}})
	t.Cleanup(func() { SetAtmosConfig(nil) })

	outcome := updateOneTool("a/b/c", UpdateOptions{MaxConcurrency: 1})
	assert.Equal(t, updateResultFailed, outcome.result)
	assert.Contains(t, outcome.message, "failed to resolve tool")
}

// TestUpdateOneTool_LatestPin_DryRun covers updateOneTool's current=="latest" dispatch and
// updateLatestPinnedTool's --dry-run branch together: dry-run short-circuits before any network
// call, so it's the cheapest way to exercise the "latest" dispatch deterministically.
func TestUpdateOneTool_LatestPin_DryRun(t *testing.T) {
	setupTestIO(t)
	filePath := createTempToolVersionsFile(t, "hashicorp/terraform latest\n")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: filePath}})
	t.Cleanup(func() { SetAtmosConfig(nil) })

	outcome := updateOneTool("hashicorp/terraform", UpdateOptions{DryRun: true, MaxConcurrency: 1})
	assert.Equal(t, updateResultUpToDate, outcome.result)
	assert.Contains(t, outcome.message, "latest")
	assert.Contains(t, outcome.message, "dry-run")
}

// TestUpdateOneTool_LatestPin_InstallFailure covers updateLatestPinnedTool's non-dry-run
// install-failure branch: a tool that doesn't exist in any registry makes the underlying
// RunInstall fail fast (no large download), so this stays deterministic without mocking.
func TestUpdateOneTool_LatestPin_InstallFailure(t *testing.T) {
	setupTestIO(t)
	filePath := createTempToolVersionsFile(t, "nonexistent-owner-abcxyz/nonexistent-repo-abcxyz latest\n")

	prevConfig := atmosConfig
	t.Cleanup(func() { SetAtmosConfig(prevConfig) })
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{
		VersionsFile: filePath,
		InstallPath:  filepath.Join(t.TempDir(), ".tools"),
	}})

	outcome := updateOneTool("nonexistent-owner-abcxyz/nonexistent-repo-abcxyz", UpdateOptions{MaxConcurrency: 1})
	assert.Equal(t, updateResultFailed, outcome.result)
}

// TestUpdateOneTool_LatestPin_InstallSucceeds covers updateLatestPinnedTool's success branch
// with a real (unmocked, network-dependent) install, matching this file's existing convention
// for exercising a real install (e.g. TestUpdateOneTool_ExactPin_InstallFailureLeavesToolVersionsUnchanged).
func TestUpdateOneTool_LatestPin_InstallSucceeds(t *testing.T) {
	setupTestIO(t)
	filePath := createTempToolVersionsFile(t, "hashicorp/terraform latest\n")

	prevConfig := atmosConfig
	t.Cleanup(func() { SetAtmosConfig(prevConfig) })
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{
		VersionsFile: filePath,
		InstallPath:  filepath.Join(t.TempDir(), ".tools"),
	}})

	outcome := updateOneTool("hashicorp/terraform", UpdateOptions{MaxConcurrency: 1})
	require.Equal(t, updateResultUpdated, outcome.result, "message: %s", outcome.message)
	assert.Contains(t, outcome.message, "latest (re-resolved)")
}

// TestUpdateOneTool_ExactPin_NoVersionsInRegistry covers updateExactPinnedTool's
// "len(sorted) == 0" branch: the registry API call succeeds but returns no releases at all
// (distinct from TestRunUpdate_ReportsFailedCount's fetch-error branch).
func TestUpdateOneTool_ExactPin_NoVersionsInRegistry(t *testing.T) {
	setupTestIO(t)
	filePath := createTempToolVersionsFile(t, "owner/repo-with-no-releases 1.0.0\n")
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{VersionsFile: filePath}})
	t.Cleanup(func() { SetAtmosConfig(nil) })

	// A mock with no releases registered for this owner/repo returns an empty list with a nil
	// error, matching a real registry response for a repo with zero tagged releases.
	mock := NewMockGitHubAPI()
	SetGitHubAPI(mock)
	t.Cleanup(ResetGitHubAPI)

	outcome := updateOneTool("owner/repo-with-no-releases", UpdateOptions{MaxConcurrency: 1})
	assert.Equal(t, updateResultFailed, outcome.result)
	assert.Contains(t, outcome.message, "no versions found in registry")
}

// TestUpdateOneTool_ExactPin_ToolVersionsWriteFailureAfterInstall covers
// updateExactPinnedTool's final AddToolToVersionsAsDefault error branch: the real install must
// succeed first (matching this file's existing real-install convention) for this branch to be
// reachable at all, and only the post-install .tool-versions rewrite is made to fail.
func TestUpdateOneTool_ExactPin_ToolVersionsWriteFailureAfterInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits behave differently on Windows")
	}
	setupTestIO(t)

	filePath := createTempToolVersionsFile(t, "hashicorp/terraform 1.9.8\n")

	prevConfig := atmosConfig
	t.Cleanup(func() { SetAtmosConfig(prevConfig) })
	SetAtmosConfig(&schema.AtmosConfiguration{Toolchain: schema.Toolchain{
		VersionsFile: filePath,
		InstallPath:  filepath.Join(t.TempDir(), ".tools"),
	}})

	mock := NewMockGitHubAPI()
	mock.SetReleases("hashicorp", "terraform", []string{"1.9.8", "1.11.4"})
	SetGitHubAPI(mock)
	t.Cleanup(ResetGitHubAPI)

	// Made read-only AFTER creation: the install itself must succeed (it doesn't touch this
	// file), and updateOneTool's own up-front LoadToolVersions read is unaffected by a missing
	// write bit -- only the final rewrite via AddToolToVersionsAsDefault is blocked.
	require.NoError(t, os.Chmod(filePath, 0o400))
	t.Cleanup(func() { _ = os.Chmod(filePath, 0o600) })

	outcome := updateOneTool("hashicorp/terraform", UpdateOptions{MaxConcurrency: 1})
	assert.Equal(t, updateResultFailed, outcome.result, "message: %s", outcome.message)
	assert.Contains(t, outcome.message, "failed to update .tool-versions")
}
