package types

import "errors"

// Static errors for gotcha tool to comply with linting requirements.
var (
	// Coverage parsing errors.
	ErrInvalidCoverageLineFormat     = errors.New("invalid coverage line format")
	ErrInvalidFileFormat             = errors.New("invalid file format")
	ErrInvalidFunctionCoverageFormat = errors.New("invalid function coverage line format")

	// Format errors.
	ErrUnsupportedFormat = errors.New("unsupported format")

	// Pattern errors.
	ErrInvalidIncludePattern = errors.New("invalid include pattern")
	ErrInvalidExcludePattern = errors.New("invalid exclude pattern")

	// Filter errors.
	ErrInvalidShowFilter = errors.New("invalid show filter")

	// Test execution errors.
	ErrTestsFailed = errors.New("tests failed")
)
