package secret

import "errors"

// ErrUnsupportedFormat indicates an unsupported file format was requested.
var ErrUnsupportedFormat = errors.New("unsupported format (use env or json)")

// ErrSecretNameRequired indicates a secret NAME argument is required (and --all was not given).
var ErrSecretNameRequired = errors.New("a secret NAME is required (or use --all to delete every declared secret)")

// ErrRawFormatConflict indicates --raw was combined with a non-text --format.
var ErrRawFormatConflict = errors.New("--raw is text-only and cannot be combined with --format=json or --format=env")

// ErrNoVault indicates no secrets vault is configured (or the named one is absent).
var ErrNoVault = errors.New("no secrets vault is configured")

// ErrAmbiguousVault indicates a vault must be named because several are configured.
var ErrAmbiguousVault = errors.New("multiple vaults configured; name one")
