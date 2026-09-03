//go:build mage

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/internal/ci/rerun"
)

const zombieRunJobs = `{"jobs":[
  {"name":"Build (linux)","status":"completed","conclusion":"success","completed_at":"2026-09-03T16:09:36Z","steps":[
    {"name":"Complete job","status":"completed","conclusion":"success","completed_at":"2026-09-03T16:09:36Z"}]},
  {"name":"Build (windows)","status":"completed","conclusion":"cancelled","completed_at":"2026-09-03T16:43:17Z","steps":[
    {"name":"Set up job","status":"completed","conclusion":"success","completed_at":"2026-09-03T16:00:00Z"},
    {"name":"Complete job","status":"completed","conclusion":"success","completed_at":"2026-09-03T16:09:36Z"}]}
]}
{"jobs":[
  {"name":"Acceptance Tests (windows)","status":"completed","conclusion":"failure","completed_at":"2026-09-03T16:43:31Z","steps":[
    {"name":"Check per-OS test matrix result","status":"completed","conclusion":"failure","completed_at":"2026-09-03T16:43:28Z"}]}
]}
`

func writeJobsFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "jobs.json")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestClassifyInfraFailures(t *testing.T) {
	t.Run("zombie run reruns and records GITHUB_OUTPUT", func(t *testing.T) {
		jobsFile := writeJobsFile(t, zombieRunJobs)
		outputPath := filepath.Join(t.TempDir(), "output")
		var stdout, stderr bytes.Buffer

		require.NoError(t, classifyInfraFailures(jobsFile, &stdout, &stderr, outputPath, ""))

		wantTable := "Build (windows)\tcancelled\trunner-stuck-after-complete\n" +
			"Acceptance Tests (windows)\tfailure\tcheck-cascade\n"
		assert.Equal(t, wantTable, stdout.String())
		assert.Equal(t, "verdict: rerun (all 2 non-success job(s) are infrastructure zombies or cascades of one)\n", stderr.String())

		output, err := os.ReadFile(outputPath)
		require.NoError(t, err)
		assert.Equal(t, "verdict=rerun\n"+
			"reason=all 2 non-success job(s) are infrastructure zombies or cascades of one\n"+
			"table<<"+githubOutputDelimiter+"\n"+wantTable+githubOutputDelimiter+"\n", string(output))
	})

	t.Run("appends to an existing GITHUB_OUTPUT", func(t *testing.T) {
		jobsFile := writeJobsFile(t, `{"jobs":[]}`)
		outputPath := filepath.Join(t.TempDir(), "output")
		require.NoError(t, os.WriteFile(outputPath, []byte("earlier=1\n"), 0o644))

		require.NoError(t, classifyInfraFailures(jobsFile, &bytes.Buffer{}, &bytes.Buffer{}, outputPath, ""))

		output, err := os.ReadFile(outputPath)
		require.NoError(t, err)
		assert.Equal(t, "earlier=1\nverdict=no-jobs\nreason=no non-success jobs\ntable<<"+githubOutputDelimiter+"\n"+githubOutputDelimiter+"\n", string(output))
	})

	t.Run("without GITHUB_OUTPUT only stdout and stderr are written", func(t *testing.T) {
		jobsFile := writeJobsFile(t, zombieRunJobs)
		var stdout, stderr bytes.Buffer

		require.NoError(t, classifyInfraFailures(jobsFile, &stdout, &stderr, "", ""))

		assert.Contains(t, stdout.String(), "runner-stuck-after-complete")
		assert.Contains(t, stderr.String(), "verdict: rerun")
	})

	t.Run("stuck gap override changes the verdict", func(t *testing.T) {
		jobsFile := writeJobsFile(t, zombieRunJobs)
		var stdout, stderr bytes.Buffer

		// 34 minutes elapsed between the last step and the job completing.
		require.NoError(t, classifyInfraFailures(jobsFile, &stdout, &stderr, "", "1h"))

		assert.Contains(t, stdout.String(), "Build (windows)\tcancelled\tsuperseded\n")
		assert.Equal(t, "verdict: no-rerun (no runner-stuck-after-complete or runner-lost jobs)\n", stderr.String())
	})

	t.Run("invalid stuck gap is rejected", func(t *testing.T) {
		jobsFile := writeJobsFile(t, zombieRunJobs)
		for _, value := range []string{"soon", "-5s", "0"} {
			err := classifyInfraFailures(jobsFile, &bytes.Buffer{}, &bytes.Buffer{}, "", value)
			require.ErrorIs(t, err, errInvalidStuckGap, value)
		}
	})

	t.Run("missing jobs file is an error", func(t *testing.T) {
		err := classifyInfraFailures(filepath.Join(t.TempDir(), "missing.json"), &bytes.Buffer{}, &bytes.Buffer{}, "", "")
		require.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("malformed jobs JSON is an error", func(t *testing.T) {
		jobsFile := writeJobsFile(t, `{"jobs":[`)
		err := classifyInfraFailures(jobsFile, &bytes.Buffer{}, &bytes.Buffer{}, "", "")
		require.Error(t, err)
	})

	t.Run("unwritable GITHUB_OUTPUT is an error", func(t *testing.T) {
		jobsFile := writeJobsFile(t, zombieRunJobs)
		err := classifyInfraFailures(jobsFile, &bytes.Buffer{}, &bytes.Buffer{}, filepath.Join(t.TempDir(), "no-such-dir", "output"), "")
		require.Error(t, err)
	})
}

func TestStuckGapOptions(t *testing.T) {
	opts, err := stuckGapOptions("")
	require.NoError(t, err)
	assert.Equal(t, rerun.Options{}, opts)

	opts, err = stuckGapOptions("300s")
	require.NoError(t, err)
	assert.Equal(t, rerun.DefaultStuckGap, opts.StuckGap)
}
