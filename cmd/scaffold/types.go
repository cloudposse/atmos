package scaffold

// DryRunFile represents a file that would be generated in dry-run mode.
type DryRunFile struct {
	Path        string // Rendered file path
	Content     string // File content
	IsTemplate  bool   // Whether the file is a template
	Exists      bool   // Whether the file already exists
	WouldCreate bool   // Whether dry-run would create the file
	WouldUpdate bool   // Whether dry-run would update the file
}

// FileStatus represents the status of a file operation.
type FileStatus int

const (
	// FileStatusCreated indicates a new file was created.
	FileStatusCreated FileStatus = iota
	// FileStatusUpdated indicates an existing file was updated.
	FileStatusUpdated
	// FileStatusSkipped indicates a file was skipped.
	FileStatusSkipped
	// FileStatusConflict indicates a file conflict occurred.
	FileStatusConflict
)

// String returns the string representation of FileStatus.
func (s FileStatus) String() string {
	switch s {
	case FileStatusCreated:
		return "CREATE"
	case FileStatusUpdated:
		return "UPDATE"
	case FileStatusSkipped:
		return "SKIP"
	case FileStatusConflict:
		return "CONFLICT"
	default:
		return "UNKNOWN"
	}
}

// Icon returns the icon for the file status.
func (s FileStatus) Icon() string {
	switch s {
	case FileStatusCreated:
		return "+"
	case FileStatusUpdated:
		return "~"
	case FileStatusSkipped:
		return "-"
	case FileStatusConflict:
		return "!"
	default:
		return "?"
	}
}

// ValidationResult represents the result of validating a scaffold file.
type ValidationResult struct {
	Path    string   // Path to the scaffold file
	Valid   bool     // Whether validation passed
	Errors  []string // List of validation errors
	Message string   // Summary message
}
