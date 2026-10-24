package store

import "errors"

// ErrStoreNameRequired indicates a store NAME argument is required.
var ErrStoreNameRequired = errors.New("a store NAME is required")

// ErrStoreKeyRequired indicates a KEY argument is required.
var ErrStoreKeyRequired = errors.New("a KEY is required")

// ErrRawFormatConflict indicates --raw was combined with a non-text --format.
var ErrRawFormatConflict = errors.New("--raw is text-only and cannot be combined with --format=json or --format=env")
