package errors

import "errors"

// Cache errors.
var (
	ErrCacheDisabled       = errors.New("cache is disabled")
	ErrNoCacheToSave       = errors.New("no cache to save")
	ErrCacheNotInitialized = errors.New("cache not initialized")
)

// Format errors.
var (
	ErrUnsupportedFormat = errors.New("unsupported format")
)

// Mock errors.
var (
	ErrMockSummaryFailed  = errors.New("mock summary write failed")
	ErrMockArtifactFailed = errors.New("mock artifact publish failed")
)

// CI errors.
var (
	ErrCommentUUIDRequired = errors.New("comment UUID is required for posting comments")
	ErrNoGitHubToken       = errors.New("no GitHub token available")
	ErrCIContextNotFound   = errors.New("CI context not found")
)

// Test errors.
var (
	ErrNoTestsRun        = errors.New("no tests were run")
	ErrTestFailed        = errors.New("test execution failed")
	ErrInvalidTestData   = errors.New("invalid test data")
	ErrNoPackagesMatched = errors.New("no packages matched the filters")
)

// Coverage errors.
var (
	ErrCoverageBelowThreshold = errors.New("coverage is below threshold")
	ErrUnsupportedPlatform    = errors.New("unsupported platform")
)
