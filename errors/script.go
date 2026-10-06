package errors

import "errors"

var (
	// ErrScript identifies language-independent script execution or configuration failures.
	ErrScript = errors.New("script execution failed")
	// ErrScriptInvalidArgument identifies invalid arguments to shared script services.
	ErrScriptInvalidArgument = errors.New("invalid script argument")
	// ErrScriptProcessFailed identifies a script subprocess failure.
	ErrScriptProcessFailed = errors.New("script process failed")
	// ErrScriptRegistration identifies an invalid interpreter registration.
	ErrScriptRegistration = errors.New("invalid script interpreter registration")
)
