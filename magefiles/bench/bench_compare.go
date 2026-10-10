package bench

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/perf/benchfmt"
	"golang.org/x/perf/benchmath"
)

const benchMetricSeparator = "|"

type benchMetricComparison struct {
	Metric             string
	Base, Candidate, P float64
	Percent            *float64
	Regression         bool
	Warnings           []string
}

type benchComparison struct {
	Baseline    *benchManifest
	Metrics     []benchMetricComparison
	Unavailable []string
}

type benchReport struct {
	Candidate   *benchManifest
	Threshold   float64
	Comparisons []benchComparison
	Unavailable []string
}

func benchReadValues(path string) (map[string][]float64, error) {
	f, err := os.Open(path) // #nosec G703 -- Mage accepts a caller-selected local measurements path.
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := benchfmt.NewReader(f, path)
	values := map[string][]float64{}
	for r.Scan() {
		switch record := r.Result().(type) {
		case *benchfmt.SyntaxError:
			return nil, record
		case *benchfmt.Result:
			if err := benchRecordValues(record, values); err != nil {
				return nil, err
			}
		}
	}
	if err = r.Err(); err != nil {
		return nil, err
	}
	if len(values) == 0 {
		return nil, Error("no benchmark measurements in %s", path)
	}
	return values, nil
}

func benchRecordValues(record *benchfmt.Result, values map[string][]float64) error {
	pkg := record.GetConfig("pkg")
	if pkg == "" || record.Iters <= 0 {
		return Error("benchmark without package or iterations")
	}
	for _, value := range record.Values {
		if math.IsNaN(value.Value) || math.IsInf(value.Value, 0) || value.Value < 0 {
			return Error("invalid benchmark value")
		}
		key := pkg + benchMetricSeparator + string(record.Name) + benchMetricSeparator + value.Unit
		values[key] = append(values[key], value.Value)
	}
	return nil
}

func benchLoad(dir string) (*benchSnapshot, error) {
	s := &benchSnapshot{Dir: dir}
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json")) // #nosec G703 -- Mage accepts a caller-selected local snapshot directory.
	if err != nil {
		return s, err
	}
	if err = json.Unmarshal(data, &s.Manifest); err != nil {
		return s, err
	}
	m := &s.Manifest
	if !benchManifestComplete(m) {
		return s, Error("incomplete or unsupported benchmark manifest")
	}
	if s.Values, err = benchReadValues(filepath.Join(dir, "bench.txt")); err != nil {
		return s, err
	}
	return s, benchValidateInventory(s)
}

func benchManifestComplete(m *benchManifest) bool {
	return m.Schema == benchSchema && m.Complete && m.Samples >= benchMinSamples && m.Samples <= benchMaxSamples && len(m.Metrics) > 0 && !m.Started.IsZero() && m.SHA != "" && m.CPU != "" && m.GoVersion != ""
}

func benchValidateInventory(s *benchSnapshot) error {
	m := &s.Manifest
	if len(s.Values) != len(m.Metrics) {
		return Error("measurement inventory differs from manifest")
	}
	seen := map[string]bool{}
	for _, key := range m.Metrics {
		pkg, _, _ := strings.Cut(key, benchMetricSeparator)
		if seen[key] || len(s.Values[key]) != m.Samples || m.Fingerprints[pkg] == "" {
			return Error("missing, duplicate, or incomplete measurement: %s", key)
		}
		seen[key] = true
	}
	return nil
}

func benchCompatible(a, b *benchManifest) bool {
	if a.Mode != b.Mode {
		return false
	}
	return a.Schema == b.Schema && a.GoVersion == b.GoVersion && benchEnvironmentMatches(a, b) && a.BuildFlags == b.BuildFlags && a.Procs == b.Procs && a.Benchtime == b.Benchtime
}

func benchEnvironmentMatches(a, b *benchManifest) bool {
	return a.GOOS == b.GOOS && a.GOARCH == b.GOARCH && a.CPU == b.CPU && a.Runner == b.Runner && a.Image == b.Image
}

func benchCompareSnapshots(base, candidate *benchSnapshot, threshold float64) benchComparison {
	result := benchComparison{Baseline: &base.Manifest}
	if !benchCompatible(&base.Manifest, &candidate.Manifest) {
		result.Unavailable = append(result.Unavailable, "Incompatible toolchain, build settings, or runner environment")
		return result
	}
	keys := slices.Clone(candidate.Manifest.Metrics)
	for key := range base.Values {
		if _, ok := candidate.Values[key]; !ok {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		pkg, _, _ := strings.Cut(key, benchMetricSeparator)
		old, oldOK := base.Values[key]
		current, currentOK := candidate.Values[key]
		switch {
		case !oldOK:
			result.Unavailable = append(result.Unavailable, "Added: "+key)
			continue
		case !currentOK:
			result.Unavailable = append(result.Unavailable, "Removed: "+key)
			continue
		case base.Manifest.Fingerprints[pkg] != candidate.Manifest.Fingerprints[pkg]:
			result.Unavailable = append(result.Unavailable, "Changed benchmark definitions/fixtures: "+key)
			continue
		}
		metric := benchCompareMetric(key, old, current, threshold)
		result.Metrics = append(result.Metrics, metric)
	}
	return result
}

func benchCompareMetric(key string, old, current []float64, threshold float64) benchMetricComparison {
	a := benchmath.NewSample(slices.Clone(old), &benchmath.DefaultThresholds)
	b := benchmath.NewSample(slices.Clone(current), &benchmath.DefaultThresholds)
	sa, sb := benchmath.AssumeNothing.Summary(a, benchConfidence), benchmath.AssumeNothing.Summary(b, benchConfidence)
	comparison := benchmath.AssumeNothing.Compare(a, b)
	metric := benchMetricComparison{Metric: key, Base: sa.Center, Candidate: sb.Center, P: comparison.P}
	if sa.Center != 0 {
		delta := benchPercent * (sb.Center/sa.Center - 1)
		metric.Percent = &delta
	}
	metric.Regression = comparison.P < benchAlpha && sb.Center > sa.Center && (metric.Percent == nil || *metric.Percent >= threshold)
	for _, warnings := range [][]error{sa.Warnings, sb.Warnings, comparison.Warnings} {
		for _, warning := range warnings {
			metric.Warnings = append(metric.Warnings, warning.Error())
		}
	}
	return metric
}

func benchThreshold() (float64, error) {
	if value := os.Getenv("ATMOS_BENCH_THRESHOLD"); value != "" {
		n, err := strconv.ParseFloat(value, FloatBits)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 {
			return 0, Error("ATMOS_BENCH_THRESHOLD must be a positive percentage")
		}
		return n, nil
	}
	return benchDefaultThreshold, nil
}

// Compare compares the candidate independently with each main snapshot.
func (Bench) Compare(snapshotsDir, candidateDir, outputDir string) error {
	candidate, err := benchLoad(candidateDir)
	if err != nil {
		return err
	}
	threshold, err := benchThreshold()
	if err != nil {
		return err
	}
	snapshots, err := benchLoadHistory(snapshotsDir)
	if err != nil {
		return err
	}
	report := benchReport{Candidate: &candidate.Manifest, Threshold: threshold}
	if err = benchLoadUnavailable(snapshotsDir, &report); err != nil {
		return err
	}
	if err = os.MkdirAll(outputDir, benchPrivateDir); err != nil {
		return err
	}
	for index, snapshot := range snapshots {
		comparison := benchCompareSnapshots(snapshot, candidate, threshold)
		report.Comparisons = append(report.Comparisons, comparison)
		if len(comparison.Metrics) == 0 {
			continue
		}
		if err = benchWritePair(outputDir, index, snapshot, candidate, &comparison); err != nil {
			return err
		}
	}
	if err = benchWriteJSON(filepath.Join(outputDir, "report.json"), report); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outputDir, "report.md"), []byte(benchMarkdown(report)), PrivateFile)
}

func benchLoadUnavailable(snapshotsDir string, report *benchReport) error {
	if data, readErr := os.ReadFile(filepath.Join(snapshotsDir, "unavailable.json")); readErr == nil {
		if err := json.Unmarshal(data, &report.Unavailable); err != nil {
			return err
		}
	} else if !os.IsNotExist(readErr) {
		return readErr
	}
	return nil
}

func benchWriteFiltered(path string, snapshot *benchSnapshot, metrics []benchMetricComparison) error {
	var out strings.Builder
	for _, metric := range metrics {
		parts := strings.Split(metric.Metric, benchMetricSeparator)
		if len(parts) != 3 {
			return Error("invalid metric key")
		}
		fmt.Fprintf(&out, "pkg: %s\n", parts[0])
		for _, value := range snapshot.Values[metric.Metric] {
			fmt.Fprintf(&out, "Benchmark%s 1 %.12g %s\n", parts[1], value, parts[2])
		}
	}
	return os.WriteFile(path, []byte(out.String()), PrivateFile)
}

func Escape(value string) string {
	return strings.NewReplacer("&", "&amp;", "[", "&#91;", "]", "&#93;", "<", "&lt;", ">", "&gt;", benchMetricSeparator, "&#124;", "`", "&#96;", "\n", " ", "\r", " ", "@", "&#64;").Replace(value)
}

func benchMarkdown(report benchReport) string {
	var out bytes.Buffer
	fmt.Fprint(&out, Marker+"\n## Atmos performance\n\n")
	if report.Candidate.Mode == benchFieldMode {
		fmt.Fprintf(&out, "Candidate revision: `%s`. Every row compares this binary against the rebuilt reference revision.\n\n", Escape(report.Candidate.SHA))
	} else {
		fmt.Fprintf(&out, "Current PR: `%s`. Every row compares this same PR build against the named main snapshot.\n\n", Escape(report.Candidate.SHA))
	}
	headline := benchHeadline(&report)
	fmt.Fprintf(&out, "**%s** Threshold: %.1f%% and p < 0.05.\n\nCross-run runner variability can affect timing. Historical comparisons reuse the same PR samples and are not independent confirmations.\n\n", headline, report.Threshold)
	benchSnapshotWarnings(&out, &report)
	benchTables(&out, &report)
	benchWarnings(&out, &report)
	body := out.String()
	if report.Candidate.Mode == benchFieldMode {
		body = strings.NewReplacer("main baseline", "reference baseline", "same PR samples", "same candidate samples", "Main snapshot (UTC)", "Reference snapshot (UTC)", "| Main | Current PR | PR change |", "| Reference | Candidate | Change |").Replace(body)
	}
	return body
}

// Keep older-snapshot warnings visible even when a large detailed table must be
// truncated in the sticky comment. These also drive the agent's advisory prompt.
func benchSnapshotWarnings(out *bytes.Buffer, report *benchReport) {
	for _, comparison := range report.Comparisons {
		warnings := 0
		for _, metric := range comparison.Metrics {
			if metric.Regression {
				warnings++
			}
		}
		if warnings > 0 {
			fmt.Fprintf(out, "- **warning**: %d metrics against snapshot `%s` (%s UTC).\n", warnings, Escape(comparison.Baseline.SHA), comparison.Baseline.Started.UTC().Format("Jan 02 15:04"))
		}
	}
	fmt.Fprintln(out)
}

func benchHeadline(report *benchReport) string {
	headline := "Comparison unavailable: no compatible main baseline."
	for i := range report.Comparisons {
		comparison := &report.Comparisons[i]
		if len(comparison.Metrics) == 0 {
			continue
		}
		headline = "No significant regressions detected against the newest compatible snapshot."
		for _, metric := range comparison.Metrics {
			if metric.Regression {
				headline = "Advisory: potential regressions against the newest compatible snapshot."
				break
			}
		}
		if len(comparison.Unavailable) > 0 {
			headline += " Coverage is partial; see unavailable comparisons."
		}
		break
	}
	return headline
}

func benchTables(out *bytes.Buffer, report *benchReport) {
	for _, affected := range []bool{true, false} {
		if affected {
			fmt.Fprint(out, "### Describe affected\n\n")
		} else {
			fmt.Fprint(out, "<details><summary>All other benchmarks</summary>\n\n")
		}
		fmt.Fprintln(out, "| Benchmark / metric | Main snapshot (UTC) | Main | Current PR | PR change | Significance |\n|---|---|---:|---:|---:|---|")
		for i := range report.Comparisons {
			comparison := &report.Comparisons[i]
			for _, metric := range comparison.Metrics {
				if strings.Contains(metric.Metric, "DescribeAffected") != affected {
					continue
				}
				benchTableRow(out, comparison, &metric)
			}
		}
		fmt.Fprintln(out)
		if !affected {
			fmt.Fprint(out, "</details>\n\n")
		}
	}
}

func benchWarnings(out *bytes.Buffer, report *benchReport) {
	fmt.Fprint(out, "<details><summary>Unavailable comparisons and statistical warnings</summary>\n\n")
	for _, reason := range report.Unavailable {
		fmt.Fprintf(out, "- %s\n", Escape(reason))
	}
	for i := range report.Comparisons {
		comparison := &report.Comparisons[i]
		for _, reason := range comparison.Unavailable {
			fmt.Fprintf(out, "- `%s`: %s\n", Escape(comparison.Baseline.SHA), Escape(reason))
		}
		for _, metric := range comparison.Metrics {
			for _, warning := range metric.Warnings {
				fmt.Fprintf(out, "- %s: %s\n", Escape(metric.Metric), Escape(warning))
			}
		}
	}
	fmt.Fprintln(out, "\n</details>")
}

func benchTableRow(out *bytes.Buffer, comparison *benchComparison, metric *benchMetricComparison) {
	delta := "new nonzero cost"
	if metric.Percent != nil {
		delta = fmt.Sprintf("%+.2f%%", *metric.Percent)
	} else if metric.Candidate == 0 {
		delta = "0%"
	}
	status := fmt.Sprintf("p=%.4g", metric.P)
	if metric.Regression {
		status += "; **warning**"
	}
	fmt.Fprintf(out, "| %s | `%s` %s | %.5g | %.5g | %s | %s |\n", Escape(metric.Metric), Escape(comparison.Baseline.SHA[:min(12, len(comparison.Baseline.SHA))]), comparison.Baseline.Started.UTC().Format("Jan 02 15:04"), metric.Base, metric.Candidate, delta, status)
}

func benchLoadHistory(snapshotsDir string) ([]*benchSnapshot, error) {
	entries, err := os.ReadDir(snapshotsDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var snapshots []*benchSnapshot
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		snapshot, loadErr := benchLoad(filepath.Join(snapshotsDir, entry.Name()))
		if loadErr != nil {
			return nil, loadErr
		}
		snapshots = append(snapshots, snapshot)
	}
	sort.Slice(snapshots, func(i, j int) bool {
		return benchSnapshotTime(&snapshots[i].Manifest).After(benchSnapshotTime(&snapshots[j].Manifest))
	})
	return snapshots, nil
}

func benchWritePair(outputDir string, index int, snapshot, candidate *benchSnapshot, comparison *benchComparison) error {
	var err error
	// Feed benchstat only compatible metrics. Never let it silently compare changed fixtures.
	basePath := filepath.Join(outputDir, fmt.Sprintf("main-%02d.txt", index+1))
	headPath := filepath.Join(outputDir, fmt.Sprintf("candidate-%02d.txt", index+1))
	if err = benchWriteFiltered(basePath, snapshot, comparison.Metrics); err != nil {
		return err
	}
	if err = benchWriteFiltered(headPath, candidate, comparison.Metrics); err != nil {
		return err
	}
	text, runErr := benchOutput("", "go", "tool", "benchstat", basePath, headPath)
	if runErr != nil {
		return runErr
	}
	if err = os.WriteFile(filepath.Join(outputDir, fmt.Sprintf("benchstat-%02d.txt", index+1)), []byte(text+"\n"), PrivateFile); err != nil {
		return err
	}
	return nil
}

func benchSnapshotTime(m *benchManifest) time.Time {
	if !m.RunCreated.IsZero() {
		return m.RunCreated
	}
	return m.Started
}

func TruncateReport(body string, limit int) string {
	if len(body) <= limit {
		return body
	}
	end := strings.LastIndex(body[:limit], "\n")
	if end < 0 {
		end = limit
	}
	body = body[:end]
	unclosed := strings.Count(body, "<details>") - strings.Count(body, "</details>")
	if unclosed > 0 {
		body += "\n" + strings.Repeat("</details>\n", unclosed)
	}
	return body + "\n\nFull results are available in the comparison artifacts.\n"
}
