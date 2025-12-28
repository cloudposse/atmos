package function

import "errors"

var (
	// ErrFunctionNotFound is returned when a function is not registered.
	ErrFunctionNotFound = errors.New("function not found")

	// ErrFunctionAlreadyRegistered is returned when attempting to register
	// a function with a name or alias that already exists.
	ErrFunctionAlreadyRegistered = errors.New("function already registered")

	// ErrInvalidArguments is returned when a function receives invalid arguments.
	ErrInvalidArguments = errors.New("invalid function arguments")

	// ErrExecutionFailed is returned when a function fails to execute.
	ErrExecutionFailed = errors.New("function execution failed")

	// ErrCircularDependency is returned when a circular dependency is detected.
	ErrCircularDependency = errors.New("circular dependency detected")

	// ErrSpecialYAMLHandling is returned when a function requires special YAML node handling.
	ErrSpecialYAMLHandling = errors.New("function requires special YAML node handling")
)
