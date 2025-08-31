package main

const (
	// Format constants.
	formatStdin    = "stdin"
	formatMarkdown = "markdown"
	formatGitHub   = "github"
	formatBoth     = "both"
	formatStream   = "stream"
	// File handling constants.
	stdinMarker        = "-"
	stdoutPath         = "stdout"
	defaultSummaryFile = "test-summary.md"
	filePermissions    = 0o644
	// Coverage threshold constants.
	coverageHighThreshold = 80.0
	coverageMedThreshold  = 40.0
	base10BitSize         = 64
	// Test display limits.
	maxTestsInChangedPackages = 200
	maxSlowestTests           = 20
	maxTotalTestsShown        = 250
	minTestsForSmartDisplay   = 100
	// Magic number constants.
	percentageMultiplier = 100
	regexMatchGroups     = 5
	floatBitSize         = 64
	// HTML template constants.
	detailsOpenTag  = "<details>\n"
	detailsCloseTag = "\n</details>\n\n"
)
