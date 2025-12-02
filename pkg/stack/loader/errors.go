package loader

import "errors"

// Static errors for loader operations.
var (
	// ErrUnsupportedFormat is returned when the file format is not supported.
	ErrUnsupportedFormat = errors.New("unsupported file format")

	// ErrParseFailed is returned when parsing the file content fails.
	ErrParseFailed = errors.New("failed to parse file")

	// ErrEncodeFailed is returned when encoding data to the format fails.
	ErrEncodeFailed = errors.New("failed to encode data")

	// ErrLoaderNotFound is returned when no loader is found for the given extension.
	ErrLoaderNotFound = errors.New("no loader found for extension")

	// ErrDuplicateLoader is returned when trying to register a loader for an extension that already exists.
	ErrDuplicateLoader = errors.New("loader already registered for extension")
)
