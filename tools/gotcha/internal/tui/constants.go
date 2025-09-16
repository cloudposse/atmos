package tui

// Display and UI constants.
const (
	// File permissions.
	DefaultFilePerms = 0o644

	// Terminal dimensions.
	DefaultTerminalWidth  = 80
	MaxProgressBarWidth   = 100
	MinTerminalWidth      = 30
	ScrollIncrement       = 10
	HeaderHeight          = 10
	FooterHeight          = 5
	PackageDisplayPadding = 20
	TestNameMaxLength     = 64

	// Progress and estimation.
	DefaultEstimatedTests = 100
	ProgressUpdateDelay   = 100 // milliseconds

	// Test status strings.
	TestStatusPass    = "pass"
	TestStatusFail    = "fail"
	TestStatusSkip    = "skip"
	TestStatusRunning = "running"

	// Display formatting.
	SpaceString   = " "
	NewlineString = "\n"
	DashString    = "-"
	TabString     = "\t"

	// Coverage display.
	CoverageBarWidth     = 30
	CoveragePercentWidth = 10

	// Exit codes.
	ExitCodeInterrupted = 130 // Standard exit code for SIGINT
)
