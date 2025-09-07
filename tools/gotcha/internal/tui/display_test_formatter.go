package tui

import (
	"fmt"
	"strings"
)

// TestFormatter handles formatting of test output based on status.
type TestFormatter struct {
	model *TestModel
}

// NewTestFormatter creates a new test formatter.
func NewTestFormatter(model *TestModel) *TestFormatter {
	return &TestFormatter{model: model}
}

// FormatTest formats a single test result.
func (f *TestFormatter) FormatTest(output *strings.Builder, test *TestResult) {
	// Skip running tests
	if test.Status != "pass" && test.Status != "fail" && test.Status != "skip" {
		return
	}

	// Format test header
	f.formatTestHeader(output, test)

	// Format test output if needed
	if f.shouldShowOutput(test) {
		f.formatTestOutput(output, test)
	}

	// Format subtests if needed
	if f.shouldShowSubtests(test) {
		f.formatSubtests(output, test)
	}
}

// formatTestHeader formats the test name, icon, and basic info.
func (f *TestFormatter) formatTestHeader(output *strings.Builder, test *TestResult) {
	icon := f.getStatusIcon(test.Status)
	output.WriteString(fmt.Sprintf("  %s %s", icon, TestNameStyle.Render(test.Name)))

	// Add duration
	if test.Elapsed > 0 {
		duration := fmt.Sprintf("(%.2fs)", test.Elapsed)
		output.WriteString(fmt.Sprintf(" %s", DurationStyle.Render(duration)))
	}

	// Add skip reason
	if test.Status == "skip" && test.SkipReason != "" {
		reason := fmt.Sprintf("- %s", test.SkipReason)
		output.WriteString(fmt.Sprintf(" %s", DurationStyle.Render(reason)))
	}

	// Add subtest progress
	f.addSubtestProgress(output, test)

	output.WriteString("\n")
}

// getStatusIcon returns the styled icon for a test status.
func (f *TestFormatter) getStatusIcon(status string) string {
	switch status {
	case "pass":
		return PassStyle.Render(CheckPass)
	case "fail":
		return FailStyle.Render(CheckFail)
	case "skip":
		return SkipStyle.Render(CheckSkip)
	default:
		return ""
	}
}

// addSubtestProgress adds a progress indicator for subtests.
func (f *TestFormatter) addSubtestProgress(output *strings.Builder, test *TestResult) {
	if len(test.Subtests) == 0 {
		return
	}

	stats := f.model.subtestStats[test.Name]
	if stats == nil {
		return
	}

	total := len(stats.passed) + len(stats.failed) + len(stats.skipped)
	if total == 0 {
		return
	}

	progress := f.model.generateSubtestProgress(len(stats.passed), total)
	percentage := (len(stats.passed) * 100) / total
	output.WriteString(fmt.Sprintf(" %s %d%% passed", progress, percentage))
}

// shouldShowOutput determines if test output should be displayed.
func (f *TestFormatter) shouldShowOutput(test *TestResult) bool {
	if test.Status != "fail" {
		return false
	}
	if f.model.showFilter == "collapsed" {
		return false
	}
	return len(test.Output) > 0
}

// formatTestOutput formats the output of a failed test.
func (f *TestFormatter) formatTestOutput(output *strings.Builder, test *TestResult) {
	output.WriteString("\n")

	formatter := f.getOutputFormatter()
	for _, line := range test.Output {
		output.WriteString("    ")
		output.WriteString(formatter(line))
	}

	output.WriteString("\n")
}

// getOutputFormatter returns the appropriate output formatter based on verbosity.
func (f *TestFormatter) getOutputFormatter() func(string) string {
	if f.model.verbosityLevel == "with-output" || f.model.verbosityLevel == "verbose" {
		return func(line string) string {
			formatted := strings.ReplaceAll(line, `\t`, "\t")
			return strings.ReplaceAll(formatted, `\n`, "\n")
		}
	}
	return func(line string) string {
		return line
	}
}

// shouldShowSubtests determines if subtest details should be displayed.
func (f *TestFormatter) shouldShowSubtests(test *TestResult) bool {
	if test.Status != "fail" {
		return false
	}
	if len(test.Subtests) == 0 {
		return false
	}
	return f.model.showFilter != "collapsed"
}

// formatSubtests formats the subtests of a failed test.
func (f *TestFormatter) formatSubtests(output *strings.Builder, test *TestResult) {
	stats := f.model.subtestStats[test.Name]
	if stats == nil {
		return
	}

	summary := NewSubtestSummaryFormatter(f.model, stats)
	summary.Format(output, test)
}

// SubtestSummaryFormatter handles formatting of subtest summaries.
type SubtestSummaryFormatter struct {
	model *TestModel
	stats *SubtestStats
}

// NewSubtestSummaryFormatter creates a new subtest summary formatter.
func NewSubtestSummaryFormatter(model *TestModel, stats *SubtestStats) *SubtestSummaryFormatter {
	return &SubtestSummaryFormatter{
		model: model,
		stats: stats,
	}
}

// Format formats the subtest summary.
func (s *SubtestSummaryFormatter) Format(output *strings.Builder, test *TestResult) {
	total := len(s.stats.passed) + len(s.stats.failed) + len(s.stats.skipped)
	if total == 0 {
		return
	}

	// Write summary header
	output.WriteString(fmt.Sprintf("\n    Subtest Summary: %d passed, %d failed of %d total\n",
		len(s.stats.passed), len(s.stats.failed), total))

	// Format each category
	s.formatPassedSubtests(output)
	s.formatFailedSubtests(output, test)
	s.formatSkippedSubtests(output)
}

// formatPassedSubtests formats the list of passed subtests.
func (s *SubtestSummaryFormatter) formatPassedSubtests(output *strings.Builder) {
	if len(s.stats.passed) == 0 {
		return
	}

	output.WriteString(fmt.Sprintf("\n    %s Passed (%d):\n", 
		PassStyle.Render("✔"), len(s.stats.passed)))
	
	for _, name := range s.stats.passed {
		subtestName := s.extractSubtestName(name)
		output.WriteString(fmt.Sprintf("      • %s\n", subtestName))
	}
}

// formatFailedSubtests formats the list of failed subtests with their output.
func (s *SubtestSummaryFormatter) formatFailedSubtests(output *strings.Builder, test *TestResult) {
	if len(s.stats.failed) == 0 {
		return
	}

	output.WriteString(fmt.Sprintf("\n    %s Failed (%d):\n", 
		FailStyle.Render("✘"), len(s.stats.failed)))
	
	formatter := s.getOutputFormatter()
	
	for _, name := range s.stats.failed {
		subtestName := s.extractSubtestName(name)
		output.WriteString(fmt.Sprintf("      • %s\n", subtestName))

		// Show subtest output if available
		s.formatSubtestOutput(output, test, name, formatter)
	}
}

// formatSubtestOutput formats the output of a single subtest.
func (s *SubtestSummaryFormatter) formatSubtestOutput(output *strings.Builder, test *TestResult, name string, formatter func(string) string) {
	subtest := test.Subtests[name]
	if subtest == nil || len(subtest.Output) == 0 {
		return
	}

	for _, line := range subtest.Output {
		output.WriteString("        ")
		output.WriteString(formatter(line))
	}
}

// formatSkippedSubtests formats the list of skipped subtests.
func (s *SubtestSummaryFormatter) formatSkippedSubtests(output *strings.Builder) {
	if len(s.stats.skipped) == 0 {
		return
	}

	output.WriteString(fmt.Sprintf("\n    %s Skipped (%d):\n", 
		SkipStyle.Render("⊘"), len(s.stats.skipped)))
	
	for _, name := range s.stats.skipped {
		subtestName := s.extractSubtestName(name)
		output.WriteString(fmt.Sprintf("      • %s\n", subtestName))
	}
}

// extractSubtestName extracts the subtest name from the full path.
func (s *SubtestSummaryFormatter) extractSubtestName(fullName string) string {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) > 1 {
		return parts[1]
	}
	return fullName
}

// getOutputFormatter returns the appropriate output formatter.
func (s *SubtestSummaryFormatter) getOutputFormatter() func(string) string {
	if s.model.verbosityLevel == "with-output" || s.model.verbosityLevel == "verbose" {
		return func(line string) string {
			formatted := strings.ReplaceAll(line, `\t`, "\t")
			return strings.ReplaceAll(formatted, `\n`, "\n")
		}
	}
	return func(line string) string {
		return line
	}
}