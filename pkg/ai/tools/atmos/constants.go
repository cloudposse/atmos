package atmos

import "os"

const (
	// File and directory permission constants.
	filePermissions os.FileMode = 0o600 // Read/write for owner only.
	dirPermissions  os.FileMode = 0o755 // Read/write/execute for owner, read/execute for others.
)
