package signature

import (
	"errors"
)

// Error carries a shared sentinel (Kind) separately from its cause, so adapters can translate the
// sentinel to a legacy one without changing the rest of the message.
type Error struct {
	// Kind is one of the ErrSignature* sentinels.
	Kind error
	// Cause is the underlying failure, if any.
	Cause error
}

// Error joins the sentinel and cause exactly like errors.Join.
//
//nolint:lintroller // Error formatting only.
func (e *Error) Error() string {
	return errors.Join(e.Kind, e.Cause).Error()
}

// Unwrap exposes both the sentinel and the cause to errors.Is and errors.As.
//
//nolint:lintroller // Trivial accessor.
func (e *Error) Unwrap() []error {
	if e.Cause == nil {
		return []error{e.Kind}
	}
	return []error{e.Kind, e.Cause}
}

func fail(kind, cause error) error {
	return &Error{Kind: kind, Cause: cause}
}
