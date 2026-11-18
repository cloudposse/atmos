package keystore

import (
	"errors"
	"fmt"
)

// CodeInteractionNotAllowed is the OSStatus errSecInteractionNotAllowed. The Secure Enclave
// returns it when a key that requires an unlocked screen is used while the screen is locked.
const CodeInteractionNotAllowed = -25308

// OSStatusError is a Core Foundation error returned by Apple's Security framework.
type OSStatusError struct {
	// Op names the Security framework call that failed.
	Op string
	// Code is the OSStatus (CFError code).
	Code int
	// Domain is the CFError domain.
	Domain string
	// Message is the CFError description.
	Message string
}

// Error implements error.
func (e *OSStatusError) Error() string {
	return fmt.Sprintf("%s failed: OSStatus %d (%s): %s", e.Op, e.Code, e.Domain, e.Message)
}

// OSStatus returns the OSStatus code carried by err, if any.
func OSStatus(err error) (int, bool) {
	var se *OSStatusError
	if errors.As(err, &se) {
		return se.Code, true
	}
	return 0, false
}

// IsInteractionNotAllowed reports whether err is the Secure Enclave refusing to sign because the
// screen is locked (errSecInteractionNotAllowed, -25308).
func IsInteractionNotAllowed(err error) bool {
	code, ok := OSStatus(err)
	return ok && code == CodeInteractionNotAllowed
}
