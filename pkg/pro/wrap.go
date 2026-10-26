package pro

import (
	errUtils "github.com/cloudposse/atmos/errors"
)

// wrapErr returns an error that labels cause with the given sentinel.
//
// Both errors.Is(result, sentinel) and errors.Is(result, anyInnerSentinel)
// match. Cockroach hints attached to cause are preserved at the top level
// of the returned error. Note that stdlib errors.Join and fmt.Errorf with
// multiple %w verbs both produce multi-errors that hide hints from
// cockroachErrors.GetAllHints — using the project error builder's WithCause
// re-attaches hints on the outer layer, so the CLI renderer surfaces them.
func wrapErr(sentinel, cause error) error {
	switch {
	case cause == nil:
		return sentinel
	case sentinel == nil:
		return cause
	default:
		return errUtils.Build(sentinel).WithCause(cause).Err()
	}
}
