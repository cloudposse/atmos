package errors

import "errors"

var (
	// ErrPublishSource indicates an invalid publishing source or relative destination.
	ErrPublishSource = errors.New("invalid publish source or destination")
	// ErrPublishTarget indicates an invalid or unsupported publishing target.
	ErrPublishTarget = errors.New("invalid publish target")
	// ErrPublishFailed indicates that publishing could not complete.
	ErrPublishFailed = errors.New("publishing failed")
)
