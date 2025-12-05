package generator

import "errors"

// Static errors for the generator package.
var (
	// ErrGeneratorNotFound is returned when a requested generator is not in the registry.
	ErrGeneratorNotFound = errors.New("generator not found")

	// ErrInvalidContext is returned when the generator context is missing required data.
	ErrInvalidContext = errors.New("invalid generator context")

	// ErrValidationFailed is returned when generator validation fails.
	ErrValidationFailed = errors.New("generator validation failed")

	// ErrGenerationFailed is returned when generation fails.
	ErrGenerationFailed = errors.New("generation failed")

	// ErrWriteFailed is returned when writing the generated file fails.
	ErrWriteFailed = errors.New("failed to write generated file")

	// ErrMissingWorkingDir is returned when WorkingDir is not set.
	ErrMissingWorkingDir = errors.New("working directory is required")

	// ErrMissingComponent is returned when Component is not set.
	ErrMissingComponent = errors.New("component is required")

	// ErrMissingStack is returned when Stack is not set.
	ErrMissingStack = errors.New("stack is required")

	// ErrMissingProviderSource is returned when a required_provider is missing the source field.
	ErrMissingProviderSource = errors.New("required_provider missing 'source' field")
)
