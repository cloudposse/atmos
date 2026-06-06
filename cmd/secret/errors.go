package secret

import "errors"

// ErrUnsupportedFormat indicates an unsupported file format was requested.
var ErrUnsupportedFormat = errors.New("unsupported format (use env or json)")

// ErrSecretNameRequired indicates a secret NAME argument is required (and --all was not given).
var ErrSecretNameRequired = errors.New("a secret NAME is required (or use --all to delete every declared secret)")
