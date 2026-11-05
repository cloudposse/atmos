package errors

import "errors"

var (
	// ErrScript identifies language-independent script execution or configuration failures.
	ErrScript = errors.New("script execution failed")
	// ErrScriptInvalidArgument identifies invalid arguments to shared script services.
	ErrScriptInvalidArgument = errors.New("invalid script argument")
	// ErrScriptProcessFailed identifies a script subprocess failure.
	ErrScriptProcessFailed = errors.New("script process failed")
	// ErrNestedAtmosFailed marks a script subprocess failure of a nested Atmos command. The
	// nested command already reported its own failure, including any CI annotations.
	ErrNestedAtmosFailed = errors.New("nested atmos command failed")
	// ErrScriptRegistration identifies an invalid interpreter registration.
	ErrScriptRegistration = errors.New("invalid script interpreter registration")
	// ErrScriptSourceConflict identifies a script step that sets both script and source.
	ErrScriptSourceConflict = errors.New("script step cannot set both script and source")
	// ErrScriptBodyRequired identifies a script step that sets neither script nor source.
	ErrScriptBodyRequired = errors.New("script step requires either script or source")
	// ErrScriptSourceInvalid identifies a script step source that is not a non-empty path string.
	ErrScriptSourceInvalid = errors.New("script step source must be a non-empty path string")
	// ErrScriptSourceRemote identifies a script step source that names a remote location.
	ErrScriptSourceRemote = errors.New("script step source must be a local file; remote sources are not supported")
	// ErrScriptSourceRead identifies a script step source file that could not be read.
	ErrScriptSourceRead = errors.New("failed to read script step source file")
	// ErrScriptInterpreterRequired identifies a script step whose interpreter is missing and cannot be inferred.
	ErrScriptInterpreterRequired = errors.New("script step interpreter is required and cannot be inferred")
)
