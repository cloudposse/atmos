// Package generator provides sentinel error aliases from the central errors package.
// These are re-exported for backward compatibility and convenience.
package generator

import (
	errUtils "github.com/cloudposse/atmos/errors"
)

// Re-exported errors from the central errors package.
// These provide the generator-specific errors for use within this package.
var (
	// ErrGeneratorNotFound is returned when a requested generator is not in the registry.
	ErrGeneratorNotFound = errUtils.ErrGeneratorNotFound

	// ErrInvalidContext is returned when the generator context is missing required data.
	ErrInvalidContext = errUtils.ErrInvalidGeneratorCtx

	// ErrValidationFailed is returned when generator validation fails.
	ErrValidationFailed = errUtils.ErrGeneratorValidation

	// ErrGenerationFailed is returned when generation fails.
	ErrGenerationFailed = errUtils.ErrGenerationFailed

	// ErrWriteFailed is returned when writing the generated file fails.
	ErrWriteFailed = errUtils.ErrGeneratorWriteFailed

	// ErrMissingWorkingDir is returned when WorkingDir is not set.
	ErrMissingWorkingDir = errUtils.ErrMissingWorkingDir

	// ErrMissingComponent is returned when Component is not set.
	ErrMissingComponent = errUtils.ErrComponentEmpty

	// ErrMissingStack is returned when Stack is not set.
	ErrMissingStack = errUtils.ErrStackEmpty

	// ErrMissingProviderSource is returned when a required_provider is missing the source field.
	ErrMissingProviderSource = errUtils.ErrMissingProviderSource
)
