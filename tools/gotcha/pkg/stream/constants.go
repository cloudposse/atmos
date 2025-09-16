package stream

// Stream processing constants.
const (
	// File permissions.
	DefaultFilePerms = 0o644

	// Buffer and display sizes.
	DefaultBufferSize    = 100
	MaxDisplayLines      = 80
	PackageHeaderPadding = 10
	TestOutputIndent     = 4
	ProgressBarWidth     = 30

	// Test status strings.
	TestStatusPass    = "pass"
	TestStatusFail    = "fail"
	TestStatusSkip    = "skip"
	TestStatusRunning = "running"

	// Formatting.
	SpaceString   = " "
	NewlineString = "\n"
	DashString    = "-"
	TabString     = "\t"

	// Progress tracking.
	DefaultEstimatedTests = 100
	ProgressUpdateMs      = 100

	// Terminal dimensions.
	DefaultTerminalWidth = 80
	MinTerminalWidth     = 30
	MaxTerminalWidth     = 200
	
	// Exit codes.
	ExitCodeInterrupted = 130 // Standard exit code for SIGINT
)
