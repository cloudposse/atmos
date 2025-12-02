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

	// ErrDuplicateFunction is returned when trying to register a function that already exists.
	ErrDuplicateFunction = errors.New("function already registered")
)
