package yaml

import "errors"

var (
	// ErrNilAtmosConfig is returned when atmosConfig is nil.
	ErrNilAtmosConfig = errors.New("atmosConfig cannot be nil")

	// ErrIncludeInvalidArguments is returned when !include has invalid arguments.
	ErrIncludeInvalidArguments = errors.New("invalid number of arguments in the !include function")

	// ErrIncludeFileNotFound is returned when !include references a non-existent file.
	ErrIncludeFileNotFound = errors.New("the !include function references a file that does not exist")

	// ErrIncludeAbsPath is returned when converting to absolute path fails.
	ErrIncludeAbsPath = errors.New("failed to convert the file path to an absolute path in the !include function")

	// ErrIncludeProcessFailed is returned when processing stack manifest fails.
	ErrIncludeProcessFailed = errors.New("failed to process the stack manifest with the !include function")

	// ErrInvalidYAMLFunction is returned when a YAML function has invalid syntax.
	ErrInvalidYAMLFunction = errors.New("invalid Atmos YAML function")
)
