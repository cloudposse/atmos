package exec

// Constants for vendor operations to avoid magic strings.
const (
	// File permissions.
	vendorDefaultFilePermissions = 0o600

	// Version defaults.
	defaultVersionLatest = "latest"

	// Template markers.
	templateStartMarker = "{{"

	// Git commit hash display length.
	gitShortHashLength = 8

	// Git command constants.
	gitCommand         = "git"
	gitTerminalPrompt0 = "GIT_TERMINAL_PROMPT=0"
)
