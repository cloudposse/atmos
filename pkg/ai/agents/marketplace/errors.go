package marketplace

import (
	"errors"
	"fmt"
)

var (
	// Installation errors.
	ErrAgentAlreadyInstalled = errors.New("agent already installed")
	ErrInvalidSource         = errors.New("invalid agent source")
	ErrDownloadFailed        = errors.New("agent download failed")

	// Validation errors.
	ErrInvalidMetadata     = errors.New("invalid agent metadata")
	ErrIncompatibleVersion = errors.New("incompatible Atmos version")
	ErrMissingPromptFile   = errors.New("prompt file not found")
	ErrInvalidToolConfig   = errors.New("invalid tool configuration")

	// Registry errors.
	ErrAgentNotFound     = errors.New("agent not found")
	ErrRegistryCorrupted = errors.New("registry file corrupted")
)

// ValidationError provides detailed validation failure information.
type ValidationError struct {
	Field   string
	Message string
	Err     error
}

func (e *ValidationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("validation failed for %s: %s (%v)", e.Field, e.Message, e.Err)
	}
	return fmt.Sprintf("validation failed for %s: %s", e.Field, e.Message)
}

func (e *ValidationError) Unwrap() error {
	return e.Err
}
