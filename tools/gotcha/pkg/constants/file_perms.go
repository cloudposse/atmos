package constants

// File permission constants.
const (
	// DefaultFilePerms is the default file permission for created files (readable by all, writable by owner).
	DefaultFilePerms = 0o644

	// SecureFilePerms is used for sensitive files (only owner can read/write).
	SecureFilePerms = 0o600
)
