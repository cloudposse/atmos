package filemanager

import "errors"

// Sentinel errors for file manager operations.
var (
	// ErrUpdateFailed indicates that updating file managers failed.
	ErrUpdateFailed = errors.New("update failed")

	// ErrVerificationFailed indicates that verification of file managers failed.
	ErrVerificationFailed = errors.New("verification failed")
)
