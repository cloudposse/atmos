package constants

const (
	// Format constants.
	FormatTerminal = "terminal" // Terminal/console output
	FormatMarkdown = "markdown" // Markdown file output
	FormatGitHub   = "github"   // GitHub Actions step summary
	FormatBoth     = "both"     // Terminal + Markdown output

	// Deprecated: Use FormatTerminal instead.
	FormatStdin = "terminal" // Alias for backward compatibility
	// File handling constants.
	StdinMarker        = "-"
	StdoutPath         = "stdout"
	DefaultSummaryFile = "test-summary.md"
	FilePermissions    = DefaultFilePerms
	// Coverage threshold constants.
	CoverageHighThreshold = 80.0
	CoverageMedThreshold  = 40.0
	Base10BitSize         = 64
	// Test display limits.
	MaxTestsInChangedPackages = 200
	MaxSlowestTests           = 20
	MaxTotalTestsShown        = 250
	MinTestsForSmartDisplay   = 100
	// Magic number constants.
	PercentageMultiplier = 100
	RegexMatchGroups     = 5
	FloatBitSize         = 64
	// HTML template constants.
	DetailsOpenTag  = "<details>\n"
	DetailsCloseTag = "\n</details>\n\n"
)
