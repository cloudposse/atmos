package blob

import (
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Operation names carried by [Error.Op] and used by every backend.
const (
	OpPut      = "put"
	OpGet      = "get"
	OpStat     = "stat"
	OpDelete   = "delete"
	OpList     = "list"
	OpValidate = "validate"
	OpNew      = "new"
)

// Kind classifies a blob error independently of the backend that produced it.
type Kind int

// Error kinds.
const (
	// KindOther is any failure without a more specific class. It matches ErrBlobOperationFailed
	// and keeps the cause visible.
	KindOther Kind = iota

	// KindNotFound means the addressed object does not exist.
	KindNotFound

	// KindPrecondition means a conditional write or delete lost its race.
	KindPrecondition

	// KindUnsupported means the endpoint cannot enforce a required condition.
	KindUnsupported

	// KindAmbiguous means a mutation was sent but its result was not observed.
	KindAmbiguous

	// KindInvalid means the request itself was malformed.
	KindInvalid

	// KindIdentity means a configured identity could not be resolved to credentials.
	KindIdentity
)

// sentinel returns the static error for the kind. KindOther maps to ErrBlobOperationFailed so every
// classified error carries a repository sentinel.
func (k Kind) sentinel() error {
	switch k {
	case KindNotFound:
		return errUtils.ErrBlobNotFound
	case KindPrecondition:
		return errUtils.ErrBlobPreconditionFailed
	case KindUnsupported:
		return errUtils.ErrBlobConditionalUnsupported
	case KindAmbiguous:
		return errUtils.ErrBlobOutcomeUnknown
	case KindInvalid:
		return errUtils.ErrBlobInvalid
	case KindIdentity:
		return errUtils.ErrBlobIdentityUnresolved
	default:
		return errUtils.ErrBlobOperationFailed
	}
}

// Error is a classified blob failure. It matches both the kind sentinel and the cause under
// errors.Is, so context cancellation and SDK errors stay visible.
type Error struct {
	// Kind is the failure class.
	Kind Kind

	// Op is the transport operation, for example "put" or "stat".
	Op string

	// Key is the object key, when the operation addressed one.
	Key string

	// Err is the underlying cause.
	Err error
}

// NewError returns a classified error. A nil err is allowed because every kind has a sentinel.
func NewError(kind Kind, op, key string, err error) *Error {
	defer perf.Track(nil, "blob.NewError")()

	return &Error{Kind: kind, Op: op, Key: key, Err: err}
}

// Error implements error.
func (e *Error) Error() string {
	defer perf.Track(nil, "blob.Error.Error")()

	subject := "blob " + e.Op
	if e.Key != "" {
		subject = fmt.Sprintf("%s %q", subject, e.Key)
	}
	sentinel := e.Kind.sentinel()
	switch {
	case e.Err == nil:
		return fmt.Sprintf("%s: %v", subject, sentinel)
	case errors.Is(e.Err, sentinel):
		return fmt.Sprintf("%s: %v", subject, e.Err)
	default:
		return fmt.Sprintf("%s: %v: %v", subject, sentinel, e.Err)
	}
}

// Unwrap exposes the kind sentinel and the cause to errors.Is and errors.As.
func (e *Error) Unwrap() []error {
	defer perf.Track(nil, "blob.Error.Unwrap")()

	chain := []error{e.Kind.sentinel()}
	if e.Err != nil {
		chain = append(chain, e.Err)
	}
	return chain
}

// Temporary reports an ambiguous outcome: the request may or may not have been applied, so the
// caller must reconcile by reading the object instead of assuming failure.
func (e *Error) Temporary() bool {
	defer perf.Track(nil, "blob.Error.Temporary")()

	return e.Kind == KindAmbiguous
}

// IsNotFound reports whether err means the object does not exist.
func IsNotFound(err error) bool {
	defer perf.Track(nil, "blob.IsNotFound")()

	return errors.Is(err, errUtils.ErrBlobNotFound)
}

// IsPreconditionFailed reports whether err means a conditional write or delete lost its race.
func IsPreconditionFailed(err error) bool {
	defer perf.Track(nil, "blob.IsPreconditionFailed")()

	return errors.Is(err, errUtils.ErrBlobPreconditionFailed)
}

// IsConditionalUnsupported reports whether err means the endpoint cannot enforce a condition.
func IsConditionalUnsupported(err error) bool {
	defer perf.Track(nil, "blob.IsConditionalUnsupported")()

	return errors.Is(err, errUtils.ErrBlobConditionalUnsupported)
}

// IsAmbiguous reports whether err means a mutation's outcome was not observed.
func IsAmbiguous(err error) bool {
	defer perf.Track(nil, "blob.IsAmbiguous")()

	return errors.Is(err, errUtils.ErrBlobOutcomeUnknown)
}
