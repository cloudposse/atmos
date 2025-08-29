package main

import "errors"

// Static errors for test-summary tool to comply with linting requirements.
var (
	// Coverage parsing errors.
	ErrInvalidCoverageLineFormat     = errors.New("invalid coverage line format")
	ErrInvalidFileFormat             = errors.New("invalid file format")
	ErrInvalidFunctionCoverageFormat = errors.New("invalid function coverage line format")
	
	// Format errors.
	ErrUnsupportedFormat = errors.New("unsupported format")
)