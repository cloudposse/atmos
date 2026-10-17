package cloudformation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

const (
	fmtCleanBody = "AWSTemplateFormatVersion: \"2010-09-09\"\n"
	fmtDirtyBody = "AWSTemplateFormatVersion:   '2010-09-09'\nResources: {}\n"
)

// writeFmtFixture writes body to a template file under dir and returns its spec.
func writeFmtFixture(t *testing.T, dir, name, body string) *stackSpec {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return &stackSpec{StackName: name, TemplateBody: body, TemplateAbsPath: path}
}

// fmt --check on a clean file must say "already formatted": plain "formatted" reads like the file
// was just written. A write run must keep saying "formatted".
func TestRunFmt_CheckWording(t *testing.T) {
	require.Equal(t, fmtCleanBody, mustFormat(t, fmtCleanBody), "fixture must already be canonical")

	tests := []struct {
		name     string
		body     string
		check    bool
		contains string
		excludes string
		wantErr  bool
	}{
		{name: "check clean", body: fmtCleanBody, check: true, contains: ": already formatted"},
		{name: "check dirty", body: fmtDirtyBody, check: true, contains: ": not formatted", wantErr: true},
		{name: "write dirty", body: fmtDirtyBody, contains: ": formatted", excludes: "already formatted"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := writeFmtFixture(t, t.TempDir(), "template.yaml", tt.body)
			var err error
			out := captureStdout(t, func() {
				_, err = runFmt(spec, map[string]any{"check": tt.check}, map[string]any{})
			})
			if tt.wantErr {
				require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationFmtNotClean)
			} else {
				require.NoError(t, err)
			}
			assert.Contains(t, out, tt.contains)
			if tt.excludes != "" {
				assert.NotContains(t, out, tt.excludes)
			}
		})
	}
}

func mustFormat(t *testing.T, body string) string {
	t.Helper()
	formatted, err := formatTemplate(body)
	require.NoError(t, err)
	return formatted
}

// In a bulk run, --check must record every unformatted template without failing the node, so the
// remaining templates are still checked; finishFmtBulk then fails once, listing all of them.
func TestFmtBulk_CheckReportsEveryUnformattedFile(t *testing.T) {
	dir := t.TempDir()
	dirtyA := writeFmtFixture(t, dir, "a.yaml", fmtDirtyBody)
	clean := writeFmtFixture(t, dir, "b.yaml", fmtCleanBody)
	dirtyC := writeFmtFixture(t, dir, "c.yaml", fmtDirtyBody)

	flags := map[string]any{"check": true}
	attachFmtBulkState(flags)

	out := captureStdout(t, func() {
		for _, spec := range []*stackSpec{dirtyA, clean, dirtyC} {
			_, err := runFmt(spec, flags, map[string]any{})
			require.NoError(t, err, "a bulk --check node must not stop the run at the first unformatted file")
		}
	})
	assert.Contains(t, out, dirtyA.TemplateAbsPath+": not formatted")
	assert.Contains(t, out, clean.TemplateAbsPath+": already formatted")
	assert.Contains(t, out, dirtyC.TemplateAbsPath+": not formatted")

	err := finishFmtBulk(flags, nil)
	require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationFmtNotClean)
	assert.Contains(t, err.Error(), dirtyA.TemplateAbsPath)
	assert.Contains(t, err.Error(), dirtyC.TemplateAbsPath)
	assert.NotContains(t, err.Error(), clean.TemplateAbsPath)
	assert.Contains(t, err.Error(), "2 template(s)")

	for _, spec := range []*stackSpec{dirtyA, dirtyC} {
		onDisk, readErr := os.ReadFile(spec.TemplateAbsPath)
		require.NoError(t, readErr)
		assert.Equal(t, fmtDirtyBody, string(onDisk), "--check must never write")
	}
}

// Negative path: a bulk --check run where everything is formatted must finish cleanly, and a
// graph error must pass through untouched instead of being replaced by the not-clean summary.
func TestFinishFmtBulk_CleanAndGraphError(t *testing.T) {
	flags := map[string]any{"check": true}
	attachFmtBulkState(flags)
	spec := writeFmtFixture(t, t.TempDir(), "ok.yaml", fmtCleanBody)
	captureStdout(t, func() {
		_, err := runFmt(spec, flags, map[string]any{})
		require.NoError(t, err)
	})
	require.NoError(t, finishFmtBulk(flags, nil))

	graphErr := errUtils.ErrAwsCloudFormationAPICallFailed
	dirtyFlags := map[string]any{"check": true}
	state := attachFmtBulkState(dirtyFlags)
	state.recordUnformatted("x.yaml")
	require.ErrorIs(t, finishFmtBulk(dirtyFlags, graphErr), graphErr)
	require.NotErrorIs(t, finishFmtBulk(dirtyFlags, graphErr), errUtils.ErrAwsCloudFormationFmtNotClean)
}

// Outside a bulk run (no state in flags) finishFmtBulk is a pass-through, and runFmt keeps its
// fail-fast single-file behavior.
func TestFinishFmtBulk_NoStatePassThrough(t *testing.T) {
	require.NoError(t, finishFmtBulk(map[string]any{}, nil))
	require.NoError(t, finishFmtBulk(nil, nil))
	sentinel := errUtils.ErrAwsCloudFormationAPICallFailed
	require.ErrorIs(t, finishFmtBulk(nil, sentinel), sentinel)
}

// Components that share one template file must check it once per run, identified by resolved
// path (two spellings of the same file count as one). Without bulk state every call checks.
func TestFmtBulk_DedupesSharedTemplateByResolvedPath(t *testing.T) {
	dir := t.TempDir()
	shared := writeFmtFixture(t, dir, "shared.yaml", fmtDirtyBody)
	// Same file reached through a redundant path element.
	alias := *shared
	alias.TemplateAbsPath = filepath.Join(dir, ".", "sub", "..", "shared.yaml")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o750))
	other := writeFmtFixture(t, dir, "other.yaml", fmtDirtyBody)

	flags := map[string]any{"check": true}
	state := attachFmtBulkState(flags)

	var summaries []map[string]any
	out := captureStdout(t, func() {
		for _, spec := range []*stackSpec{shared, &alias, other} {
			summary, err := runFmt(spec, flags, map[string]any{})
			require.NoError(t, err)
			summaries = append(summaries, summary)
		}
	})
	require.Len(t, summaries, 3)
	assert.Equal(t, true, summaries[1]["skipped"], "the second component sharing a template must be skipped")
	assert.NotContains(t, summaries[0], "skipped")
	assert.NotContains(t, summaries[2], "skipped")
	assert.Equal(t, 1, countOccurrences(out, shared.TemplateAbsPath+": not formatted"), "a shared template must be reported once")
	assert.Equal(t, []string{shared.TemplateAbsPath, other.TemplateAbsPath}, state.unformatted)

	// Negative path: without bulk state, nothing is deduplicated.
	for range 2 {
		var err error
		captureStdout(t, func() { _, err = runFmt(shared, map[string]any{"check": true}, map[string]any{}) })
		require.ErrorIs(t, err, errUtils.ErrAwsCloudFormationFmtNotClean)
	}
}

// Non-check bulk runs format a shared template once, and a second claim of it never rewrites it.
func TestFmtBulk_WriteRunFormatsSharedTemplateOnce(t *testing.T) {
	spec := writeFmtFixture(t, t.TempDir(), "shared.yaml", fmtDirtyBody)
	flags := map[string]any{}
	attachFmtBulkState(flags)

	out := captureStdout(t, func() {
		for range 2 {
			_, err := runFmt(spec, flags, map[string]any{})
			require.NoError(t, err)
		}
	})
	assert.Equal(t, 1, countOccurrences(out, ": formatted"))
	onDisk, err := os.ReadFile(spec.TemplateAbsPath)
	require.NoError(t, err)
	assert.Equal(t, mustFormat(t, fmtDirtyBody), string(onDisk))
}

func countOccurrences(haystack, needle string) int {
	count := 0
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			count++
		}
	}
	return count
}

// claim must be safe for concurrent use and report the first claimant only.
func TestFmtBulkState_ClaimIsFirstWins(t *testing.T) {
	state := newFmtBulkState()
	path := filepath.Join(t.TempDir(), "t.yaml")
	assert.True(t, state.claim(path))
	assert.False(t, state.claim(path))
	assert.True(t, state.claim(path+".other"))
}
