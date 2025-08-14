package exec

import "errors"

// Common constants for YAML functions.
const (
	invalidYamlFuncMsg = "invalid YAML function"
	function           = "function"
)

// Common errors for store YAML functions.
var (
	ErrStoreNotFound         = errors.New("store not found")
	ErrGetKeyFailed          = errors.New("failed to get key from store")
	ErrInvalidPipeParams     = errors.New("invalid parameters after pipe")
	ErrInvalidPipeIdentifier = errors.New("invalid identifier after pipe")
)
