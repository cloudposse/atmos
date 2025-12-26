package function

import "errors"

// Static errors for function operations.
var (
	// ErrFunctionNotFound is returned when a function is not found in the registry.
	ErrFunctionNotFound = errors.New("function not found")

	// ErrInvalidArguments is returned when function arguments are invalid.
	ErrInvalidArguments = errors.New("invalid function arguments")

	// ErrFunctionExecution is returned when a function fails to execute.
	ErrFunctionExecution = errors.New("function execution failed")

	// ErrExecutionFailed is returned when function execution fails.
	ErrExecutionFailed = errors.New("execution failed")

	// ErrDuplicateFunction is returned when trying to register a function that already exists.
	ErrDuplicateFunction = errors.New("function already registered")

	// ErrFunctionAlreadyRegistered is returned when attempting to register
	// a function with a name or alias that already exists.
	ErrFunctionAlreadyRegistered = errors.New("function already registered")

	// ErrUnclosedQuote is returned when a string has an unclosed quote.
	ErrUnclosedQuote = errors.New("unclosed quote")

	// ErrCircularDependency is returned when a circular dependency is detected.
	ErrCircularDependency = errors.New("circular dependency detected")

	// ErrSpecialYAMLHandling indicates that a function requires special YAML node processing.
	// Functions like !include need to modify the YAML tree directly.
	ErrSpecialYAMLHandling = errors.New("special YAML handling required")
)
