package bench

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"golang.org/x/perf/benchmath"
)

type benchMetricSummary struct {
	Metric string
	Value  float64
}

func benchSummaries(candidate *benchSnapshot) []benchMetricSummary {
	keys := slices.Clone(candidate.Manifest.Metrics)
	slices.Sort(keys)
	metrics := make([]benchMetricSummary, 0, len(keys))
	for _, key := range keys {
		sample := benchmath.NewSample(slices.Clone(candidate.Values[key]), &benchmath.DefaultThresholds)
		summary := benchmath.AssumeNothing.Summary(sample, benchConfidence)
		metrics = append(metrics, benchMetricSummary{Metric: key, Value: summary.Center})
	}
	return metrics
}

func benchMetricLabel(key string) (string, string) {
	parts := strings.Split(key, benchMetricSeparator)
	if len(parts) != 3 {
		return key, ""
	}
	return strings.TrimPrefix(parts[0], "github.com/cloudposse/atmos/") + "/" + parts[1], parts[2]
}

// Use the same fields for eligibility and diagnostics so a rejected baseline
// always explains exactly which compatibility requirement it failed.
func benchCompatibilityReasons(base, candidate *benchManifest) []string {
	fields := []struct {
		name            string
		base, candidate any
	}{
		{"mode", base.Mode, candidate.Mode},
		{"schema", base.Schema, candidate.Schema},
		{"Go version", base.GoVersion, candidate.GoVersion},
		{"OS", base.GOOS, candidate.GOOS},
		{"architecture", base.GOARCH, candidate.GOARCH},
		{"CPU", base.CPU, candidate.CPU},
		{"runner", base.Runner, candidate.Runner},
		{"image", base.Image, candidate.Image},
		{"build flags", base.BuildFlags, candidate.BuildFlags},
		{"processes", base.Procs, candidate.Procs},
		{"benchmark duration", base.Benchtime, candidate.Benchtime},
	}
	var reasons []string
	for _, field := range fields {
		if field.base != field.candidate {
			reasons = append(reasons, fmt.Sprintf("incompatible %s: baseline=%q, candidate=%q", field.name, fmt.Sprint(field.base), fmt.Sprint(field.candidate)))
		}
	}
	return reasons
}

func benchReportDetails(out *bytes.Buffer, report *benchReport) {
	fmt.Fprint(out, "<details><summary>Measurement details</summary>\n\n")
	m := report.Candidate
	if m.PRHeadSHA != "" {
		fmt.Fprintf(out, "Current PR head: `%s`. Measured merge revision: `%s`, based on main `%s`.\n\n", Escape(m.PRHeadSHA), Escape(m.SHA), Escape(m.BaseSHA))
	} else {
		fmt.Fprintf(out, "Candidate revision: `%s`.\n\n", Escape(m.SHA))
	}
	fmt.Fprintf(out, "Environment: %s/%s · %s · %s. Values are medians of %d samples.\n\n", Escape(m.GOOS), Escape(m.GOARCH), Escape(m.CPU), Escape(m.GoVersion), m.Samples)
	for _, comparison := range report.Comparisons {
		if len(comparison.Metrics) == 0 {
			continue
		}
		fmt.Fprintf(out, "Regression threshold: %.1f%% and p < 0.05.\n\n", report.Threshold)
		if m.Mode == benchFieldMode {
			fmt.Fprint(out, "Each row compares this binary against the rebuilt reference revision.\n\n")
		} else {
			fmt.Fprint(out, "Each row compares this merged build against a main snapshot. Older snapshots include intervening main changes. Cross-run runner variability can affect timing; historical comparisons reuse the same PR samples and are not independent confirmations.\n\n")
		}
		break
	}
	fmt.Fprint(out, "</details>\n")
}
