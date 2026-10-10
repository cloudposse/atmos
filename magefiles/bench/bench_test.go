//go:build mage

package bench

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudposse/atmos/magefiles/internal/githubapi"
	"github.com/cloudposse/atmos/magefiles/internal/githubtest"

	"github.com/stretchr/testify/require"
)

const (
	benchTestMetric = "example/bench|CLI/DescribeAffected/instances=100-2|sec/op"
)

func benchTestSnapshot(value float64) *benchSnapshot {
	m := benchManifest{
		Schema: benchSchema, SHA: githubtest.SHA, Started: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC), GoVersion: "go1.26.9", GOOS: "linux", GOARCH: "amd64", CPU: "test-cpu", Runner: "Linux/X64", Image: "ubuntu24", BuildFlags: "test", Procs: 2, Samples: 10, Benchtime: "1s", Complete: true,
		Metrics: []string{benchTestMetric}, Fingerprints: map[string]string{"example/bench": "fixture-v1"},
	}
	values := make([]float64, 10)
	for i := range values {
		values[i] = value
	}
	return &benchSnapshot{Manifest: m, Values: map[string][]float64{benchTestMetric: values}}
}

func benchSaveTestSnapshot(t *testing.T, dir string, s *benchSnapshot) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, benchWriteJSON(filepath.Join(dir, "manifest.json"), s.Manifest))
	var metrics []benchMetricComparison
	for key := range s.Values {
		metrics = append(metrics, benchMetricComparison{Metric: key})
	}
	require.NoError(t, benchWriteFiltered(filepath.Join(dir, "bench.txt"), s, metrics))
}

func TestBenchCompareEverySnapshotToCandidate(t *testing.T) {
	head := benchTestSnapshot(120)
	newer, older := benchTestSnapshot(130), benchTestSnapshot(100)
	older.Manifest.SHA = githubtest.Base
	newResult := benchCompareSnapshots(newer, head, 10)
	oldResult := benchCompareSnapshots(older, head, 10)
	require.False(t, newResult.Metrics[0].Regression)
	require.True(t, oldResult.Metrics[0].Regression)
	require.InDelta(t, -7.6923, *newResult.Metrics[0].Percent, .001)
	require.InDelta(t, 20, *oldResult.Metrics[0].Percent, .001)
	require.Equal(t, newResult.Metrics[0].Candidate, oldResult.Metrics[0].Candidate)
	body := benchMarkdown(benchReport{Candidate: &head.Manifest, Threshold: 10, Comparisons: []benchComparison{newResult, oldResult}})
	require.Contains(t, body, "No significant regressions detected against the newest")
	require.Contains(t, body, "+20.00%")
	require.Contains(t, body, "**warning**")
	warning := "**warning**: 1 metrics against snapshot `" + githubtest.Base + "`"
	require.Contains(t, body, warning)
	require.Less(t, strings.Index(body, warning), strings.Index(body, "### Describe affected"), "older warnings must survive truncation of detailed tables")
}

func TestBenchStatistics(t *testing.T) {
	for _, tc := range []struct {
		name                  string
		base, head, threshold float64
		regression            bool
	}{
		{"regression", 100, 120, 10, true}, {"improvement", 100, 80, 10, false}, {"equal", 100, 100, 10, false}, {"below threshold", 100, 105, 10, false}, {"custom threshold", 100, 120, 25, false}, {"both zero", 0, 0, 10, false}, {"new allocation", 0, 1, 10, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := benchCompareSnapshots(benchTestSnapshot(tc.base), benchTestSnapshot(tc.head), tc.threshold)
			require.Equal(t, tc.regression, result.Metrics[0].Regression)
		})
	}
	noisy := benchTestSnapshot(0)
	noisy.Values[benchTestMetric] = []float64{1, 10, 30, 50, 100, 150, 200, 300, 400, 1000}
	other := benchTestSnapshot(0)
	other.Values[benchTestMetric] = []float64{1, 10, 30, 50, 120, 170, 200, 300, 400, 1000}
	require.False(t, benchCompareSnapshots(noisy, other, 10).Metrics[0].Regression)
}

func TestBenchCompatibilityAndInventory(t *testing.T) {
	for _, field := range []string{"go", "cpu", "image", "flags", "fixture"} {
		t.Run(field, func(t *testing.T) {
			base, head := benchTestSnapshot(100), benchTestSnapshot(120)
			switch field {
			case "go":
				head.Manifest.GoVersion = "other"
			case "cpu":
				head.Manifest.CPU = "other"
			case "image":
				head.Manifest.Image = "other"
			case "flags":
				head.Manifest.BuildFlags = "other"
			case "fixture":
				head.Manifest.Fingerprints["example/bench"] = "other"
			}
			result := benchCompareSnapshots(base, head, 10)
			require.Empty(t, result.Metrics)
			require.NotEmpty(t, result.Unavailable)
		})
	}
	base, head := benchTestSnapshot(100), benchTestSnapshot(120)
	head.Manifest.Metrics = []string{"example/bench|New-2|sec/op"}
	head.Values = map[string][]float64{head.Manifest.Metrics[0]: head.Values[benchTestMetric]}
	result := benchCompareSnapshots(base, head, 10)
	require.Len(t, result.Unavailable, 2)
	require.Contains(t, strings.Join(result.Unavailable, "\n"), "Added:")
	require.Contains(t, strings.Join(result.Unavailable, "\n"), "Removed:")
}

func TestBenchLoadRejectsIncompleteData(t *testing.T) {
	for _, change := range []string{"valid", "incomplete", "samples", "inventory", "duplicate", "nan", "empty"} {
		t.Run(change, func(t *testing.T) {
			dir := t.TempDir()
			s := benchTestSnapshot(100)
			switch change {
			case "incomplete":
				s.Manifest.Complete = false
			case "samples":
				s.Manifest.Samples = 11
			case "inventory":
				s.Manifest.Metrics = append(s.Manifest.Metrics, "missing")
			case "duplicate":
				s.Manifest.Metrics = append(s.Manifest.Metrics, s.Manifest.Metrics[0])
			}
			benchSaveTestSnapshot(t, dir, s)
			if change == "nan" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "bench.txt"), []byte("pkg: example/bench\nBenchmarkCLI 1 NaN ns/op\n"), 0o600))
			}
			if change == "empty" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "bench.txt"), nil, 0o600))
			}
			_, err := benchLoad(dir)
			if change == "valid" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

func TestBenchCompareWritesBenchstatAndReports(t *testing.T) {
	dir := t.TempDir()
	base, head := benchTestSnapshot(100), benchTestSnapshot(120)
	benchSaveTestSnapshot(t, filepath.Join(dir, "snapshots", "1"), base)
	benchSaveTestSnapshot(t, filepath.Join(dir, "candidate"), head)
	require.NoError(t, (Bench{}).Compare(filepath.Join(dir, "snapshots"), filepath.Join(dir, "candidate"), filepath.Join(dir, "report")))
	data, err := os.ReadFile(filepath.Join(dir, "report", "benchstat-01.txt"))
	require.NoError(t, err)
	require.Contains(t, string(data), "+20.00%")
	data, err = os.ReadFile(filepath.Join(dir, "report", "report.json"))
	require.NoError(t, err)
	var report benchReport
	require.NoError(t, json.Unmarshal(data, &report))
	require.True(t, report.Comparisons[0].Metrics[0].Regression)
}

func benchZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var data bytes.Buffer
	w := zip.NewWriter(&data)
	for name, contents := range files {
		file, err := w.Create(name)
		require.NoError(t, err)
		_, err = file.Write(contents)
		require.NoError(t, err)
	}
	require.NoError(t, w.Close())
	return data.Bytes()
}

//nolint:gocritic // Test fixtures are copied so callers retain their original run metadata.
func benchSnapshotArchive(t *testing.T, run githubapi.Run) []byte {
	s := benchTestSnapshot(100)
	s.Manifest.RunID = fmt.Sprint(run.ID)
	s.Manifest.Attempt = fmt.Sprint(run.Attempt)
	s.Manifest.Repository = githubtest.Repo
	s.Manifest.Event = run.Event
	s.Manifest.SHA = run.HeadSHA
	s.Manifest.BaseSHA = githubtest.Base
	dir := t.TempDir()
	benchSaveTestSnapshot(t, dir, s)
	files := map[string][]byte{}
	for _, name := range []string{"manifest.json", "bench.txt"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		files[name] = data
	}
	return benchZip(t, files)
}

func TestBenchArchiveDataOnly(t *testing.T) {
	for _, name := range []string{"../manifest.json", "/manifest.json", "atmos", "subdir/bench.txt"} {
		t.Run(name, func(t *testing.T) {
			require.Error(t, benchExtract(benchZip(t, map[string][]byte{name: []byte("bad")}), t.TempDir()))
		})
	}
	require.Error(t, benchExtract(benchZip(t, map[string][]byte{"manifest.json": []byte("{}")}), t.TempDir()))
	require.NoError(t, benchExtract(benchSnapshotArchive(t, githubtest.Run()), t.TempDir()))
}

func TestBenchFetchFiltersPaginatesAndFreezes(t *testing.T) {
	run := githubtest.Run()
	candidate := benchTestSnapshot(120)
	candidate.Manifest.Started = run.Updated.Add(time.Hour)
	late := run
	late.ID = 20
	late.Updated = candidate.Manifest.Started.Add(time.Minute)
	wrong := run
	wrong.ID = 21
	wrong.Event = "pull_request"
	expired := run
	expired.ID = 22
	diverged := run
	diverged.ID = 23
	diverged.HeadSHA = strings.Repeat("c", 40)
	api := &githubtest.FakeAPI{T: t, Archive: benchSnapshotArchive(t, run)}
	pages := 0
	api.Get = func(path string, response any) error {
		switch {
		case strings.Contains(path, "/workflows/"):
			pages++
			require.Contains(t, path, "created=")
			if pages == 1 {
				rows := make([]githubapi.Run, 100)
				rows[0] = late
				rows[1] = wrong
				rows[2] = expired
				rows[3] = diverged
				return githubtest.Respond(map[string]any{"workflow_runs": rows}, response)
			}
			return githubtest.Respond(map[string]any{"workflow_runs": []githubapi.Run{run, run}}, response)
		case strings.Contains(path, "/compare/"):
			status := "behind"
			if strings.HasSuffix(path, diverged.HeadSHA) {
				status = "diverged"
			}
			return githubtest.Respond(map[string]string{"status": status}, response)
		case strings.Contains(path, "/runs/22/artifacts"):
			return githubtest.Respond(map[string]any{"artifacts": []benchArtifact{{ID: 1, Name: benchArtifactName(&expired), Expired: true}}}, response)
		case strings.Contains(path, "/runs/10/artifacts"):
			if strings.HasSuffix(path, "page=1") {
				return githubtest.Respond(map[string]any{"artifacts": make([]benchArtifact, 100)}, response)
			}
			return githubtest.Respond(map[string]any{"artifacts": []benchArtifact{{ID: 2, Name: benchArtifactName(&run)}}}, response)
		default:
			t.Fatalf("unexpected endpoint %s", path)
			return nil
		}
	}
	dir := t.TempDir()
	require.NoError(t, benchFetch(context.Background(), api, githubtest.Repo, githubtest.Base, dir, candidate))
	require.Equal(t, 2, pages)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	_, err = benchLoad(filepath.Join(dir, "10"))
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(dir, "unavailable.json"))
	require.NoError(t, err)
	require.Contains(t, string(data), "missing or expired")
}

func TestBenchFetchBootstrap(t *testing.T) {
	api := &githubtest.FakeAPI{T: t, Get: func(_ string, response any) error {
		return githubtest.Respond(map[string]any{"workflow_runs": []githubapi.Run{}}, response)
	}}
	dir := t.TempDir()
	require.NoError(t, benchFetch(context.Background(), api, githubtest.Repo, githubtest.Base, dir, benchTestSnapshot(100)))
	body := benchMarkdown(benchReport{Candidate: &benchTestSnapshot(100).Manifest, Threshold: 10})
	require.Contains(t, body, "Comparison unavailable")
}

func TestBenchProvenanceAndMarkdown(t *testing.T) {
	run := githubtest.Run()
	dir := t.TempDir()
	require.NoError(t, benchExtract(benchSnapshotArchive(t, run), dir))
	s, err := benchLoad(dir)
	require.NoError(t, err)
	require.NoError(t, benchValidateRun(&s.Manifest, githubtest.Repo, &run))
	run.Attempt++
	require.Error(t, benchValidateRun(&s.Manifest, githubtest.Repo, &run))
	require.Equal(t, "&#64;team &lt;script&gt; &#124; &#96;", Escape("@team <script> | `"))
}

func TestBenchTrustedCutoffAndOrdering(t *testing.T) {
	candidate := benchTestSnapshot(1)
	run := githubtest.Run()
	candidate.Manifest.Started = run.Updated.Add(2 * time.Hour)
	candidate.Cutoff = run.Updated.Add(-time.Minute)
	f := benchFetcher{candidate: candidate, seen: map[string]bool{}}
	require.False(t, f.eligible(&run), "a main run finishing after the PR started must not qualify")
	candidate.Cutoff = run.Updated.Add(time.Minute)
	require.True(t, f.eligible(&run))
	run.Event = "pull_request"
	require.False(t, f.eligible(&run))

	dir := t.TempDir()
	older, newer := benchTestSnapshot(1), benchTestSnapshot(1)
	older.Manifest.SHA, newer.Manifest.SHA = githubtest.Base, githubtest.SHA
	older.Manifest.RunCreated = run.Created
	newer.Manifest.RunCreated = run.Created.Add(time.Minute)
	// The older run waited longer for a runner and started collecting later.
	older.Manifest.Started = newer.Manifest.Started.Add(time.Hour)
	benchSaveTestSnapshot(t, filepath.Join(dir, "older"), older)
	benchSaveTestSnapshot(t, filepath.Join(dir, "newer"), newer)
	history, err := benchLoadHistory(dir)
	require.NoError(t, err)
	require.Equal(t, githubtest.SHA, history[0].Manifest.SHA)
}

func TestBenchLongCommentPreservesLinksAndClosesDetails(t *testing.T) {
	body := "<details><summary>Measurements</summary>\n" + strings.Repeat("benchmark row\n", 10000) + "</details>\n"
	run := githubtest.Run()
	body = benchReportLinks(body, githubtest.Repo, &run)
	require.Equal(t, strings.Count(body, "<details>"), strings.Count(body, "</details>"))
	require.Contains(t, body, "https://github.com/cloudposse/atmos/actions/runs/10")
	require.Less(t, len(body), CommentLimit)
}

func TestBenchFieldReportReproducesRegressionAndRecovery(t *testing.T) {
	dir := t.TempDir()
	versions := []benchFieldVersion{
		{Ref: "known-good", SHA: githubtest.Base},
		{Ref: "known-bad", SHA: strings.Repeat("c", 40)},
		{Ref: "fixed", SHA: githubtest.SHA},
	}
	for i, version := range versions {
		value := 100.0
		if i == 1 {
			value = 1000
		}
		snapshot := benchTestSnapshot(value)
		snapshot.Manifest.Mode = benchFieldMode
		snapshot.Manifest.SHA = version.SHA
		snapshot.Manifest.SourceRef = version.Ref
		benchSaveTestSnapshot(t, filepath.Join(dir, benchFieldMeasurements, version.SHA), snapshot)
	}
	require.NoError(t, benchFieldReports(dir, versions))
	for i, version := range versions[1:] {
		data, err := os.ReadFile(filepath.Join(dir, "comparisons", version.SHA, "report.json"))
		require.NoError(t, err)
		var report benchReport
		require.NoError(t, json.Unmarshal(data, &report))
		require.Equal(t, i == 0, report.Comparisons[0].Metrics[0].Regression)
		require.Equal(t, githubtest.Base, report.Comparisons[0].Baseline.SHA)
	}
	body, err := os.ReadFile(filepath.Join(dir, "report.md"))
	require.NoError(t, err)
	require.Contains(t, string(body), "Candidate revision")
	require.NotContains(t, string(body), "Current PR")
	ci, field := benchTestSnapshot(1), benchTestSnapshot(1)
	field.Manifest.Mode = benchFieldMode
	require.False(t, benchCompatible(&ci.Manifest, &field.Manifest))
}

func TestBenchFieldRefsValidateBeforeBuilding(t *testing.T) {
	root, err := benchRepoRoot()
	require.NoError(t, err)
	for _, refs := range []string{"", "--help", "HEAD", "HEAD,HEAD", "HEAD,does-not-exist-quality-benchmark"} {
		_, err = benchFieldVersions(root, refs)
		require.Error(t, err, refs)
	}
}

func TestBenchCompareMissingHistory(t *testing.T) {
	dir := t.TempDir()
	candidate := filepath.Join(dir, "candidate")
	benchSaveTestSnapshot(t, candidate, benchTestSnapshot(100))
	output := filepath.Join(dir, "report")
	require.NoError(t, (Bench{}).Compare(filepath.Join(dir, "missing-history"), candidate, output))
	body, err := os.ReadFile(filepath.Join(output, "report.md"))
	require.NoError(t, err)
	require.Contains(t, string(body), "Comparison unavailable")
}
