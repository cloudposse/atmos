package errors

import (
	goerrors "errors"
	"fmt"
	"strings"
)

// authFailureSeparator splits an error message into the segments that wrapping layers joined
// with ": ".
const authFailureSeparator = ": "

// authenticationFailedError reports a failed authentication attempt. It matches
// ErrAuthenticationFailed under errors.Is and keeps its cause on a single-cause Unwrap chain, so
// both the cause (errors.Is / errors.As) and any hints or details attached to it stay reachable.
//
// Layers that wrap an authentication failure with more context would otherwise each repeat the
// sentinel text and produce a stuttering chain of "authentication failed" clauses. This type puts
// the sentinel first and once, followed by the accumulated scopes (identity or provider) and
// the real cause.
type authenticationFailedError struct {
	scopes []string
	detail string
	cause  error
}

// WrapAuthenticationFailed wraps cause as an authentication failure scoped to what was being
// authenticated, for example:
//
//	WrapAuthenticationFailed(err, "identity %q", name)
//
// renders as `authentication failed for identity "name": <cause>`.
//
// The result matches ErrAuthenticationFailed and cause under errors.Is. Redundant occurrences of
// the sentinel text in cause are dropped from the message, and wrapping an error that is already
// an authentication failure extends its scope list rather than nesting another sentinel:
//
//	authentication failed for identity "dev" via provider "sso": <cause>
//
// It returns nil when cause is nil.
func WrapAuthenticationFailed(cause error, scopeFormat string, args ...any) error {
	if cause == nil {
		return nil
	}

	var scopes []string
	if scope := fmt.Sprintf(scopeFormat, args...); scope != "" {
		scopes = []string{scope}
	}
	wrapped := &authenticationFailedError{cause: cause}

	//nolint:errorlint // Only a direct authenticationFailedError is merged; a wrapped one keeps its own message.
	if inner, ok := cause.(*authenticationFailedError); ok {
		wrapped.scopes = appendUniqueScope(scopes, inner.scopes...)
		wrapped.detail = inner.detail
		return wrapped
	}

	wrapped.scopes = scopes
	wrapped.detail = dropSentinelSegments(cause.Error())
	return wrapped
}

// EnsureAuthenticationFailed returns err unchanged when it already matches
// ErrAuthenticationFailed, and otherwise wraps it so it does. Use it at command boundaries that
// need the sentinel for errors.Is or exit-code mapping but must not repeat its text on top of an
// error the authentication manager already worded. It returns nil when err is nil.
func EnsureAuthenticationFailed(err error) error {
	if err == nil || goerrors.Is(err, ErrAuthenticationFailed) {
		return err
	}
	return WrapAuthenticationFailed(err, "")
}

// Error renders the sentinel text once, then the scopes, then the cause detail.
func (e *authenticationFailedError) Error() string {
	var msg strings.Builder
	msg.WriteString(ErrAuthenticationFailed.Error())
	if len(e.scopes) > 0 {
		msg.WriteString(" for " + strings.Join(e.scopes, " via "))
	}
	if e.detail != "" {
		msg.WriteString(authFailureSeparator + e.detail)
	}
	return msg.String()
}

// Unwrap exposes the cause on a single-cause chain so hints and details remain reachable.
func (e *authenticationFailedError) Unwrap() error {
	return e.cause
}

// Is reports a match with the ErrAuthenticationFailed sentinel.
func (e *authenticationFailedError) Is(target error) bool {
	return target == ErrAuthenticationFailed
}

// appendUniqueScope appends the scopes that are not already present, preserving order.
func appendUniqueScope(scopes []string, more ...string) []string {
	for _, s := range more {
		duplicate := false
		for _, existing := range scopes {
			if existing == s {
				duplicate = true
				break
			}
		}
		if !duplicate {
			scopes = append(scopes, s)
		}
	}
	return scopes
}

// dropSentinelSegments removes the segments of msg that consist solely of the
// ErrAuthenticationFailed text, so wrapped sentinels are not repeated in the message. Segments are
// split on the ": " that fmt.Errorf wrapping produces and on the newline that errors.Join
// produces, so a joined sentinel does not leave a dangling "authentication failed" line or glue
// the next message onto it without a separator.
func dropSentinelSegments(msg string) string {
	sentinel := ErrAuthenticationFailed.Error()
	segments := strings.Split(msg, authFailureSeparator)
	kept := segments[:0]
	for _, segment := range segments {
		if segment = dropSentinelLines(segment, sentinel); segment != "" {
			kept = append(kept, segment)
		}
	}
	return strings.Join(kept, authFailureSeparator)
}

// dropSentinelLines removes the lines of segment that equal the sentinel text.
func dropSentinelLines(segment, sentinel string) string {
	lines := strings.Split(segment, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if line != sentinel {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// sentinelMarkedError carries extra sentinels for errors.Is without changing the message.
type sentinelMarkedError struct {
	err       error
	sentinels []error
}

// MarkAs makes err additionally match the given sentinels under errors.Is while leaving its
// message untouched. Use it when a layer wants callers to be able to match its sentinel but the
// message already says everything the sentinel text would. It returns nil when err is nil.
func MarkAs(err error, sentinels ...error) error {
	if err == nil {
		return nil
	}
	return &sentinelMarkedError{err: err, sentinels: sentinels}
}

// Error returns the wrapped error's message unchanged.
func (e *sentinelMarkedError) Error() string {
	return e.err.Error()
}

// Unwrap exposes the wrapped error on a single-cause chain.
func (e *sentinelMarkedError) Unwrap() error {
	return e.err
}

// Is reports whether target is one of the marked sentinels.
func (e *sentinelMarkedError) Is(target error) bool {
	for _, s := range e.sentinels {
		if goerrors.Is(s, target) {
			return true
		}
	}
	return false
}

// WrapIdentityAuthFailed wraps an error from authenticating the named identity so it matches
// ErrIdentityAuthFailed. When err already is an authentication failure, its message (which names
// the identity) is kept as is rather than prefixed with a second "failed to authenticate
// identity" clause. It returns nil when err is nil.
func WrapIdentityAuthFailed(identityName string, err error) error {
	if err == nil {
		return nil
	}
	if goerrors.Is(err, ErrAuthenticationFailed) {
		return MarkAs(err, ErrIdentityAuthFailed)
	}
	return fmt.Errorf(ErrWrapWithNameAndCauseFormat, ErrIdentityAuthFailed, identityName, err)
}
