package marketplace

import "os"

const (
	// Standard name for skill metadata files.
	skillFileName = "SKILL.md"

	// File and directory permission constants.
	dirPermissions  os.FileMode = 0o755 // Read/write/execute for owner, read/execute for others.
	filePermissions os.FileMode = 0o600 // Read/write for owner only.

	// Format string for wrapping an error with a quoted name.
	// Use with fmt.Errorf to produce: "<sentinel>: "<name>"".
	errFmtWithName = "%w: %q"
)
