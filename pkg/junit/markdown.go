package junit

import (
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Options controls how Markdown renders a report.
type Options struct {
	// Title is the heading shown above the report (e.g. "Terraform tests").
	// Defaults to "Test results" when empty.
	Title string
}

// Markdown renders a GitHub-flavored markdown summary of the report: a heading,
// total/passed/failed/errored/skipped badges, a per-case results table, and the
// failure messages. It is pure and reusable as a CI step summary for any
// JUnit-producing test runner.
func Markdown(r *Report, opts Options) string {
	defer perf.Track(nil, "junit.Markdown")()

	r.Aggregate()

	title := opts.Title
	if title == "" {
		title = "Test results"
	}

	var b strings.Builder
	icon := "✅"
	if !r.Passed() {
		icon = "❌"
	}
	fmt.Fprintf(&b, "## %s %s\n\n", icon, title)

	writeBadges(&b, r)
	writeTable(&b, r)
	writeFailureDetails(&b, r)

	return b.String()
}

func writeBadges(b *strings.Builder, r *Report) {
	passed := r.Tests - r.Failures - r.Errors - r.Skipped
	fmt.Fprintf(b, "[![total](https://shields.io/badge/TESTS-%d-blue?style=for-the-badge)](#)", r.Tests)
	if passed > 0 {
		fmt.Fprintf(b, " [![passed](https://shields.io/badge/PASSED-%d-success?style=for-the-badge)](#)", passed)
	}
	if r.Failures > 0 {
		fmt.Fprintf(b, " [![failed](https://shields.io/badge/FAILED-%d-critical?style=for-the-badge)](#)", r.Failures)
	}
	if r.Errors > 0 {
		fmt.Fprintf(b, " [![errored](https://shields.io/badge/ERRORED-%d-ff0000?style=for-the-badge)](#)", r.Errors)
	}
	if r.Skipped > 0 {
		fmt.Fprintf(b, " [![skipped](https://shields.io/badge/SKIPPED-%d-inactive?style=for-the-badge)](#)", r.Skipped)
	}
	b.WriteString("\n\n")
}

func writeTable(b *strings.Builder, r *Report) {
	b.WriteString("| Result | Suite | Test |\n|--------|-------|------|\n")
	for _, s := range r.Suites {
		for _, c := range s.Cases {
			fmt.Fprintf(b, "| %s | `%s` | `%s` |\n", statusMark(c.Status()), s.Name, c.Name)
		}
	}
}

func writeFailureDetails(b *strings.Builder, r *Report) {
	failed := r.FailedCases()
	if len(failed) == 0 {
		return
	}
	b.WriteString("\n<details><summary>Failures</summary>\n\n")
	for _, f := range failed {
		location := f.Suite
		if f.File != "" {
			location = f.File
			if f.Line > 0 {
				location = fmt.Sprintf("%s:%d", f.File, f.Line)
			}
		}
		fmt.Fprintf(b, "**`%s`** (%s)\n\n```\n%s\n```\n\n", f.Name, location, f.Message)
	}
	b.WriteString("</details>\n")
}

func statusMark(status string) string {
	switch status {
	case "pass":
		return ":white_check_mark: pass"
	case "fail":
		return ":x: fail"
	case "error":
		return ":boom: error"
	default:
		return ":fast_forward: skip"
	}
}
