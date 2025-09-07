package ci

import "errors"

// Common CI errors that apply across all providers.
var (
	// Integration errors.
	ErrNoIntegrationDetected   = errors.New("no CI integration detected")
	ErrIntegrationNotSupported = errors.New("CI provider not supported")
	ErrIntegrationNotAvailable = errors.New("CI integration not available in current environment")

	// Context errors.
	ErrContextNotDetected  = errors.New("CI context could not be detected")
	ErrContextNotSupported = errors.New("current CI context does not support this operation")

	// Comment errors.
	ErrCommentNotSupported = errors.New("comments not supported for this CI provider")
	ErrCommentTooLarge     = errors.New("comment content exceeds platform size limit")
	ErrCommentUpdateFailed = errors.New("failed to update existing comment")
	ErrCommentCreateFailed = errors.New("failed to create new comment")

	// Job summary errors.
	ErrJobSummaryNotSupported = errors.New("job summaries not supported for this CI provider")
	ErrJobSummaryWriteFailed  = errors.New("failed to write job summary")

	// Artifact errors.
	ErrArtifactNotSupported  = errors.New("artifacts not supported for this CI provider")
	ErrArtifactPublishFailed = errors.New("failed to publish artifact")
)
