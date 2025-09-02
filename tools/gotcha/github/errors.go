package github

import "errors"

// GitHub-related static errors.
var (
	// Context errors.
	ErrNotGitHubActions       = errors.New("not running in GitHub Actions environment")
	ErrRepositoryNotSet       = errors.New("GITHUB_REPOSITORY environment variable not set")
	ErrInvalidRepositoryFormat = errors.New("invalid GITHUB_REPOSITORY format")
	ErrEventNameNotSet        = errors.New("GITHUB_EVENT_NAME environment variable not set")
	ErrCommentUUIDNotSet      = errors.New("GOTCHA_COMMENT_UUID environment variable not set")
	ErrGitHubTokenNotAvailable = errors.New("GitHub token not available")
	ErrUnsupportedEventType   = errors.New("event type is not supported for PR comments")
	ErrEventPathNotSet        = errors.New("GITHUB_EVENT_PATH not set")
	ErrNoPRNumberInEvent      = errors.New("no PR number found in event payload")
	ErrNoPRNumberInEnv        = errors.New("no PR number found in environment variables")
	
	// Comment manager errors.
	ErrUUIDCannotBeEmpty      = errors.New("UUID cannot be empty")
	ErrGitHubContextIsNil     = errors.New("GitHub context cannot be nil")
	
	// Mock client errors.
	ErrCommentNotFound        = errors.New("comment not found")
)