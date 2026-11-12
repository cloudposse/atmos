package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/cloudposse/atmos/plugins/ssh/agent"
	"github.com/cloudposse/atmos/plugins/ssh/authority"
	"github.com/cloudposse/atmos/plugins/ssh/keystore"
	"github.com/cloudposse/atmos/plugins/ssh/service"
)

// Code is a short, stable name for a kind of failure. It is recorded in the status file.
type Code string

// Failure codes.
const (
	CodeInvalidSpec            Code = "invalid-spec"
	CodeRequirementMismatch    Code = "requirement-mismatch"
	CodeUnsupportedRequirement Code = "unsupported-requirement"
	CodeUnknownAuthority       Code = "unknown-authority"
	CodeUnsupportedUse         Code = "unsupported-use"
	CodeTokenRejected          Code = "token-rejected"
	CodeMissingScope           Code = "missing-scope"
	CodeAuthorityUnreachable   Code = "authority-unreachable"
	CodeKeyLocked              Code = "key-locked"
	CodeAgentUnavailable       Code = "agent-unavailable"
	CodeUnsupportedPlatform    Code = "unsupported-platform"
	CodeCanceled               Code = "canceled"
	CodeInternal               Code = "internal"
)

// Sentinel errors.
var (
	// ErrAgentUnavailable means the identity's agent socket did not come up.
	ErrAgentUnavailable = errors.New("the agent did not start serving the identity")
	// ErrNoSSHKeygen means no ssh-keygen was found, and Git signing needs one.
	ErrNoSSHKeygen = errors.New("ssh-keygen not found")
)

// Error is a failure with a Code and a message that says what to do about it. The message is
// safe to record: credentials are removed from it.
type Error struct {
	Code    Code
	Message string
	Err     error
}

// Error implements the error interface.
func (e *Error) Error() string { return e.Message }

// Unwrap returns the cause.
func (e *Error) Unwrap() error { return e.Err }

// classify turns any failure into an *Error. secrets are removed from the message.
func classify(identityName, keyName string, err error, secrets ...string) *Error {
	var known *Error
	if errors.As(err, &known) {
		return known
	}
	code, msg := describe(identityName, keyName, err)
	return &Error{Code: code, Message: scrub(msg, secrets), Err: err}
}

// rule maps a kind of failure to its Code and the message to record.
type rule struct {
	matches func(err error) bool
	code    Code
	message func(keyName string, err error) string
}

// is matches when the error wraps any of the targets.
func is(targets ...error) func(error) bool {
	return func(err error) bool {
		for _, t := range targets {
			if errors.Is(err, t) {
				return true
			}
		}
		return false
	}
}

func plain(keyName string, err error) string { return err.Error() }

func suffixed(suffix string) func(string, error) string {
	return func(_ string, err error) string { return err.Error() + suffix }
}

// rules are tried in order; the first match wins.
var rules = []rule{
	{is(context.Canceled, context.DeadlineExceeded), CodeCanceled, func(_ string, err error) string {
		return "the operation was canceled or timed out: " + err.Error()
	}},
	{keystore.IsInteractionNotAllowed, CodeKeyLocked, func(_ string, err error) string {
		return "the key is locked: the screen is locked and the key requires an unlocked screen (" + err.Error() + ")"
	}},
	{is(keystore.ErrRequirementMismatch), CodeRequirementMismatch, func(keyName string, err error) string {
		return fmt.Sprintf("%v. The requirement is fixed when a key is created, so rotate the key: run "+
			"\"atmos ssh key delete %s --yes\", then log in again", err, keyName)
	}},
	{
		is(keystore.ErrUnsupportedRequirement), CodeUnsupportedRequirement,
		suffixed(". Remove the term from require or choose a key store that can enforce it"),
	},
	{is(authority.ErrUnknownAuthority), CodeUnknownAuthority, func(_ string, err error) string {
		return fmt.Sprintf("%v (known: %s)", err, strings.Join(authority.Names(), ", "))
	}},
	{is(authority.ErrUnsupportedUse), CodeUnsupportedUse, plain},
	{is(authority.ErrMissingPermission), CodeMissingScope, func(_ string, err error) string { return missingScope(err) }},
	{is(authority.ErrUnauthorized), CodeTokenRejected, suffixed(". Check the token stored for the provider this identity uses")},
	{
		is(authority.ErrUnavailable, authority.ErrRateLimited), CodeAuthorityUnreachable,
		suffixed(". The key could not be enrolled; try again later"),
	},
	{is(keystore.ErrUnsupportedPlatform, agent.ErrUnsupportedPlatform, service.ErrUnsupportedPlatform), CodeUnsupportedPlatform, plain},
	{is(ErrInvalidSpec), CodeInvalidSpec, plain},
	{is(ErrAgentUnavailable, agent.ErrSocketInUse), CodeAgentUnavailable, func(_ string, err error) string {
		return fmt.Sprintf("%v. Run \"atmos ssh agent status\" and read \"atmos ssh agent log\" for details", err)
	}},
}

func describe(_, keyName string, err error) (Code, string) {
	for _, r := range rules {
		if r.matches(err) {
			return r.code, r.message(keyName, err)
		}
	}
	return CodeInternal, err.Error()
}

// missingScope names the permission the credential lacked.
func missingScope(err error) string {
	var ae *authority.Error
	if errors.As(err, &ae) && ae.Permission != "" {
		return fmt.Sprintf("the token lacks the %s scope: %v. Add the scope to the token and log in again", ae.Permission, err)
	}
	return err.Error() + ". Add the missing scope to the token and log in again"
}

// scrub removes every secret from s.
func scrub(s string, secrets []string) string {
	for _, secret := range secrets {
		if len(secret) >= 4 {
			s = strings.ReplaceAll(s, secret, "[redacted]")
		}
	}
	return s
}
