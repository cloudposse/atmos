package vcs

import "errors"

// Common VCS errors that apply across all platforms.
var (
	// Provider errors.
	ErrNoProviderDetected   = errors.New("no VCS provider detected")
	ErrProviderNotSupported = errors.New("VCS provider not supported")
	ErrProviderNotAvailable = errors.New("VCS provider not available in current environment")

	// Context errors.
	ErrContextNotDetected  = errors.New("VCS context could not be detected")
	ErrContextNotSupported = errors.New("current VCS context does not support this operation")

	// Comment errors.
	ErrCommentNotSupported = errors.New("comments not supported for this VCS platform")
	ErrCommentTooLarge     = errors.New("comment content exceeds platform size limit")
	ErrCommentUpdateFailed = errors.New("failed to update existing comment")
	ErrCommentCreateFailed = errors.New("failed to create new comment")

	// Job summary errors.
	ErrJobSummaryNotSupported = errors.New("job summaries not supported for this VCS platform")
	ErrJobSummaryWriteFailed  = errors.New("failed to write job summary")

	// Artifact errors.
	ErrArtifactNotSupported  = errors.New("artifacts not supported for this VCS platform")
	ErrArtifactPublishFailed = errors.New("failed to publish artifact")
)
