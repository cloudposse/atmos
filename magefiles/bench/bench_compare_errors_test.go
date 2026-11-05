//go:build mage

package bench

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBenchThresholdValidation(t *testing.T) {
	for _, value := range []string{"0", "-1", "NaN", "+Inf", "invalid", "25"} {
		t.Setenv("ATMOS_BENCH_THRESHOLD", value)
		n, err := benchThreshold()
		if value == "25" {
			require.NoError(t, err)
			require.Equal(t, 25.0, n)
		} else {
			require.Error(t, err)
		}
	}
}

func TestBenchCompareRejectsIncompleteInputs(t *testing.T) {
	for _, scenario := range []string{"missing candidate", "bad threshold", "bad history", "bad unavailable", "blocked output", "blocked json", "blocked markdown"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			candidate := filepath.Join(dir, "candidate")
			history := filepath.Join(dir, "history")
			out := filepath.Join(dir, "report")
			benchSaveTestSnapshot(t, candidate, benchTestSnapshot(100))
			require.NoError(t, os.Mkdir(history, 0o700))
			switch scenario {
			case "missing candidate":
				candidate = filepath.Join(dir, "missing")
			case "bad threshold":
				t.Setenv("ATMOS_BENCH_THRESHOLD", "invalid")
			case "bad history":
				require.NoError(t, os.Mkdir(filepath.Join(history, "incomplete"), 0o700))
			case "bad unavailable":
				require.NoError(t, os.WriteFile(filepath.Join(history, "unavailable.json"), []byte("{"), 0o600))
			case "blocked output":
				require.NoError(t, os.WriteFile(out, nil, 0o600))
			case "blocked json":
				require.NoError(t, os.MkdirAll(filepath.Join(out, "report.json"), 0o700))
			case "blocked markdown":
				require.NoError(t, os.MkdirAll(filepath.Join(out, "report.md"), 0o700))
			}
			require.Error(t, (Bench{}).Compare(history, candidate, out))
		})
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "unavailable.json")
	require.NoError(t, os.Mkdir(path, 0o700))
	require.Error(t, benchLoadUnavailable(dir, &benchReport{}))
	require.Error(t, benchWriteFiltered(dir, benchTestSnapshot(100), nil))
	_, err := benchReadValues(filepath.Join(dir, "missing"))
	require.Error(t, err)
}

func TestBenchComparisonIncludesCompatibilityReasons(t *testing.T) {
	head := benchTestSnapshot(120)
	base := benchTestSnapshot(100)
	base.Manifest.CPU = "other CPU"
	report := benchReport{Candidate: &head.Manifest, Comparisons: []benchComparison{benchCompareSnapshots(base, head, 10)}}
	body := benchMarkdown(&report)
	require.Contains(t, body, "unavailable")
	require.Contains(t, body, `incompatible CPU: baseline="other CPU", candidate="test-cpu"`)
}
