package marketplace

import (
	"errors"
	"fmt"
)

var (
	// Installation errors.
	ErrSkillAlreadyInstalled = errors.New("skill already installed")
	ErrInvalidSource         = errors.New("invalid skill source")
	ErrDownloadFailed        = errors.New("skill download failed")

	// Validation errors.
	ErrInvalidMetadata     = errors.New("invalid skill metadata")
	ErrIncompatibleVersion = errors.New("incompatible Atmos version")
	ErrMissingPromptFile   = errors.New("SKILL.md file not found")
	ErrInvalidToolConfig   = errors.New("invalid tool configuration")

	// User action errors.
	ErrInstallationCancelled   = errors.New("installation cancelled by user")
	ErrUninstallationCancelled = errors.New("uninstallation cancelled")
	ErrUpdateCancelled         = errors.New("update cancelled by user")
	ErrNoValidSkillsFound      = errors.New("no valid skills found in package")
	ErrNoYAMLFrontmatter       = errors.New("no YAML frontmatter found")

	// Update errors.
	ErrSkillUpdateNotSupported   = errors.New("update not supported for this skill")
	ErrSkillBatchOperationFailed = errors.New("one or more skills failed during batch operation")

	// Registry errors.
	ErrSkillNotFound     = errors.New("skill not found")
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
