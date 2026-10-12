//go:build mage

package bench

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBenchARMCompatibility(t *testing.T) {
	base, candidate := benchTestSnapshot(100), benchTestSnapshot(120)
	for _, snapshot := range []*benchSnapshot{base, candidate} {
		snapshot.Manifest.GOARCH = "arm64"
		snapshot.Manifest.Runner = "Linux/ARM64"
		var err error
		snapshot.Manifest.CPU, err = benchCPUIdentity(armCPUInfo)
		require.NoError(t, err)
	}
	require.Len(t, benchCompareSnapshots(base, candidate, 10).Metrics, 1)
	base.Manifest.GOARCH = "amd64"
	result := benchCompareSnapshots(base, candidate, 10)
	require.Empty(t, result.Metrics)
	require.Equal(t, []string{`incompatible architecture: baseline="amd64", candidate="arm64"`}, result.Unavailable)
}

func TestBenchReportRetainsMeasurementsWithoutBaseline(t *testing.T) {
	for _, baseline := range []string{"absent", "CPU mismatch", "changed fixtures"} {
		t.Run(baseline, func(t *testing.T) {
			dir := t.TempDir()
			candidate := benchTestSnapshot(120)
			// A skewed sample distinguishes the median from the mean.
			candidate.Values[benchTestMetric][9] = 10000
			benchSaveTestSnapshot(t, filepath.Join(dir, "candidate"), candidate)
			if baseline != "absent" {
				base := benchTestSnapshot(100)
				if baseline == "CPU mismatch" {
					base.Manifest.CPU = "AMD EPYC"
				} else {
					base.Manifest.Fingerprints["example/bench"] = "different"
				}
				benchSaveTestSnapshot(t, filepath.Join(dir, "history", "1"), base)
			}
			require.NoError(t, (Bench{}).Compare(filepath.Join(dir, "history"), filepath.Join(dir, "candidate"), filepath.Join(dir, "report")))
			data, err := os.ReadFile(filepath.Join(dir, "report", "report.json"))
			require.NoError(t, err)
			var report benchReport
			require.NoError(t, json.Unmarshal(data, &report))
			require.Equal(t, []benchMetricSummary{{Metric: benchTestMetric, Value: 120}}, report.CandidateMetrics)
			data, err = os.ReadFile(filepath.Join(dir, "report", "report.md"))
			require.NoError(t, err)
			body := string(data)
			require.Contains(t, body, "Comparison unavailable")
			require.Contains(t, body, "| 120 | sec/op |")
			require.Contains(t, body, "### Describe affected")
			for _, absent := range []string{"All other benchmarks", "Main snapshot (UTC)", "PR change", "Significance", "p < 0.05", "Cross-run"} {
				require.NotContains(t, body, absent)
			}
			if baseline == "CPU mismatch" {
				require.Contains(t, body, `incompatible CPU: baseline="AMD EPYC", candidate="test-cpu"`)
			}
		})
	}
}

func TestBenchReportShowsUncomparedMetricsAlongsideComparisons(t *testing.T) {
	base, candidate := benchTestSnapshot(100), benchTestSnapshot(120)
	other := "example/bench|Merge-2|B/op"
	candidate.Manifest.Metrics = append(candidate.Manifest.Metrics, other)
	candidate.Values[other] = candidate.Values[benchTestMetric]
	report := benchReport{
		Candidate: &candidate.Manifest, CandidateMetrics: benchSummaries(candidate), Threshold: 10,
		Comparisons: []benchComparison{benchCompareSnapshots(base, candidate, 10)},
	}
	body := benchMarkdown(&report)
	require.Contains(t, body, "+20.00%")
	require.Contains(t, body, "All other benchmarks")
	require.Contains(t, body, "| example/bench/Merge-2 | 120 | B/op |")
	details := strings.Index(body, "<details><summary>Measurement details</summary>")
	require.Positive(t, details)
	require.NotContains(t, body[:details], "Regression threshold")
	require.Contains(t, body[details:], "Regression threshold: 10.0%")
	require.Contains(t, body[details:], "Candidate revision:")
}

func TestBenchReportOmitsEmptyTablesAndWarnings(t *testing.T) {
	candidate := benchTestSnapshot(120)
	body := benchMarkdown(&benchReport{Candidate: &candidate.Manifest})
	for _, absent := range []string{"### Describe affected", "All other benchmarks", "| Benchmark", "Unavailable comparisons and statistical warnings"} {
		require.NotContains(t, body, absent)
	}
}
