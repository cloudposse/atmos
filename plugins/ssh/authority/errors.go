package authority

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// Sentinel errors. An *Error wraps one of the first group so callers can match with errors.Is.
var (
	// ErrUnauthorized means the authority rejected the credential, for example a bad token.
	ErrUnauthorized = errors.New("credential rejected")
	// ErrMissingPermission means the credential lacks a permission the operation needs.
	ErrMissingPermission = errors.New("credential lacks a required permission")
	// ErrKeyInUse means the key is already registered on another account.
	ErrKeyInUse = errors.New("key is already in use on another account")
	// ErrRateLimited means the authority is throttling requests.
	ErrRateLimited = errors.New("rate limited")
	// ErrUnexpectedStatus means the authority answered in a way the client does not understand.
	ErrUnexpectedStatus = errors.New("unexpected response")
	// ErrUnavailable means the authority could not be reached.
	ErrUnavailable = errors.New("authority unreachable")
	// ErrUnsupportedUse means the authority cannot vouch for the requested use.
	ErrUnsupportedUse = errors.New("use not supported by this authority")
	// ErrInvalidRequest means the request or a receipt is malformed.
	ErrInvalidRequest = errors.New("invalid request")

	// ErrUnknownAuthority means no authority is registered under the name.
	ErrUnknownAuthority = errors.New("unknown authority")
	// ErrDuplicateAuthority means an authority is already registered under the name.
	ErrDuplicateAuthority = errors.New("authority already registered")

	// ErrReceiptNotFound means no receipt exists for the key and authority.
	ErrReceiptNotFound = errors.New("receipt not found")
	// ErrReceiptCorrupt means a receipt file cannot be read.
	ErrReceiptCorrupt = errors.New("receipt is corrupt")
	// ErrReceiptCollision means two different keys map to the same receipt file.
	ErrReceiptCollision = errors.New("receipt file belongs to another key")
)

// Error is a typed failure from an authority. It never carries a credential, so it is safe to
// record and to render in a later status command.
type Error struct {
	// Authority is the name of the authority that failed.
	Authority string
	// Operation is what was being attempted, such as "list" or "create".
	Operation string
	// Use is the use being enrolled or revoked, when it applies.
	Use Use
	// Permission names the permission or scope that is missing, when it applies.
	Permission string
	// Status is the HTTP status, when the authority speaks HTTP.
	Status int
	// RetryAfter is how long the authority asked the caller to wait, when known.
	RetryAfter time.Duration
	// Detail is extra context. It never contains the credential.
	Detail string
	// Err is the sentinel that classifies the failure.
	Err error
}

// Error implements the error interface.
func (e *Error) Error() string {
	var b strings.Builder
	if e.Authority != "" {
		b.WriteString(e.Authority)
		b.WriteString(": ")
	}
	if e.Operation != "" {
		b.WriteString(e.Operation)
		if e.Use != "" {
			b.WriteString(" ")
			b.WriteString(string(e.Use))
		}
		b.WriteString(": ")
	}
	if e.Err != nil {
		b.WriteString(e.Err.Error())
	}
	if e.Permission != "" {
		fmt.Fprintf(&b, " (needs %s)", e.Permission)
	}
	if e.Status != 0 {
		fmt.Fprintf(&b, " [HTTP %d]", e.Status)
	}
	if e.RetryAfter > 0 {
		fmt.Fprintf(&b, " [retry after %s]", e.RetryAfter)
	}
	if e.Detail != "" {
		b.WriteString(": ")
		b.WriteString(e.Detail)
	}
	return b.String()
}

// Unwrap returns the classifying sentinel.
func (e *Error) Unwrap() error { return e.Err }
