package types

import (
	"context"
	"time"
)

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const (
	// ContextKeyAllowPrompts is the context key for controlling whether credential prompts are allowed.
	// When set to false, authentication flows should not prompt for credentials.
	ContextKeyAllowPrompts contextKey = "atmos-auth-allow-prompts"
	// ContextKeyForceAWSWebflow is the context key for bypassing cached and long-lived
	// AWS user credentials in favor of a new browser authentication flow.
	ContextKeyForceAWSWebflow contextKey = "atmos-auth-force-aws-webflow"
	// ContextKeyMinCredentialValidity is the context key for the minimum remaining lifetime
	// credentials must have to be reused instead of refreshed.
	ContextKeyMinCredentialValidity contextKey = "atmos-auth-min-credential-validity"
)

// DefaultMinCredentialValidity is the remaining lifetime credentials must have to be reused
// instead of refreshed. AWS can invalidate credentials before their stated expiration time, so
// handing out credentials that are about to expire risks failures in long-running operations.
// It is the single source of truth for the auth manager's chain cache, the credential-process
// identity's file cache, and the default of `atmos aws credential-process --min-validity`.
const DefaultMinCredentialValidity = 15 * time.Minute

// WithAllowPrompts returns a new context with the allow-prompts flag set.
// When allowPrompts is false, authentication flows should not prompt for credentials.
func WithAllowPrompts(ctx context.Context, allowPrompts bool) context.Context {
	return context.WithValue(ctx, ContextKeyAllowPrompts, allowPrompts)
}

// AllowPrompts returns whether credential prompts are allowed in this context.
// Returns true if the flag is not set (default behavior allows prompts).
func AllowPrompts(ctx context.Context) bool {
	val := ctx.Value(ContextKeyAllowPrompts)
	if val == nil {
		return true // Default: allow prompts.
	}
	allow, ok := val.(bool)
	if !ok {
		return true // Default: allow prompts if value is wrong type.
	}
	return allow
}

// WithForceAWSWebflow returns a new context that controls forced browser authentication
// for aws/user identities. This is intentionally invocation-scoped rather than configuration.
func WithForceAWSWebflow(ctx context.Context, force bool) context.Context {
	return context.WithValue(ctx, ContextKeyForceAWSWebflow, force)
}

// ForceAWSWebflow reports whether aws/user authentication must start a new browser flow.
// Returns false when the flag is not set or has an unexpected type.
func ForceAWSWebflow(ctx context.Context) bool {
	force, ok := ctx.Value(ContextKeyForceAWSWebflow).(bool)
	return ok && force
}

// WithMinCredentialValidity returns a new context carrying the minimum remaining lifetime that
// cached credentials must have to be reused. Identities that cache their own credentials
// (aws/user sessions, aws/credential-process helper output) refresh when their cache does not
// satisfy it. A zero value reuses any credentials that have not yet expired.
// This is intentionally invocation-scoped rather than configuration.
func WithMinCredentialValidity(ctx context.Context, d time.Duration) context.Context {
	return context.WithValue(ctx, ContextKeyMinCredentialValidity, d)
}

// MinCredentialValidity returns the caller-requested minimum remaining credential lifetime and
// whether one was set. A negative value is treated as zero.
func MinCredentialValidity(ctx context.Context) (time.Duration, bool) {
	d, ok := ctx.Value(ContextKeyMinCredentialValidity).(time.Duration)
	if !ok {
		return 0, false
	}
	if d < 0 {
		d = 0
	}
	return d, true
}

// MinCredentialValidityOr returns the caller-requested minimum remaining credential lifetime,
// or def when the context does not carry one.
func MinCredentialValidityOr(ctx context.Context, def time.Duration) time.Duration {
	if d, ok := MinCredentialValidity(ctx); ok {
		return d
	}
	return def
}
