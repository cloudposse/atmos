package constants

// File and system constants.
const (

	// Magic numbers for terminal and display.
	DefaultTerminalWidth  = 80
	MaxProgressBarWidth   = 100
	ProgressBarPadding    = 10
	MaxTestNameLength     = 64
	MinTerminalWidth      = 30
	ScrollIncrement       = 10
	DefaultEstimatedTests = 100
	CoverageTablePadding  = 20

	// Time and duration constants.
	SecondsPerDay = 86400

	// Formatting constants.
	SpaceString   = " "
	NewlineString = "\n"
	DashString    = "-"

	// Test status constants.
	TestStatusPass    = "pass"
	TestStatusFail    = "fail"
	TestStatusSkip    = "skip"
	TestStatusRunning = "running"

	// Format type constants.
	FormatTerminal = "terminal"
	FormatJSON     = "json"
	FormatGitHub   = "github"
	FormatPlain    = "plain"
	FormatMarkdown = "markdown"

	// Exit codes.
	ExitCodeInterrupted = 130 // Standard exit code for SIGINT
)
