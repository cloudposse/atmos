package types

import "context"

// contextKey is a custom type for context keys to avoid collisions.
type contextKey string

const (
	// ContextKeyAllowPrompts is the context key for controlling whether credential prompts are allowed.
	// When set to false, authentication flows should not prompt for credentials.
	ContextKeyAllowPrompts contextKey = "atmos-auth-allow-prompts"
	// ContextKeySuppressAuthErrors is the context key for suppressing auth error printing.
	ContextKeySuppressAuthErrors contextKey = "atmos-auth-suppress-errors"
)

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

// WithSuppressAuthErrors returns a new context with the suppress-auth-errors flag set.
func WithSuppressAuthErrors(ctx context.Context, suppress bool) context.Context {
	return context.WithValue(ctx, ContextKeySuppressAuthErrors, suppress)
}

// SuppressAuthErrors returns whether auth error printing should be suppressed.
// Returns false if the flag is not set (default behavior prints errors).
func SuppressAuthErrors(ctx context.Context) bool {
	val := ctx.Value(ContextKeySuppressAuthErrors)
	if val == nil {
		return false
	}
	suppress, ok := val.(bool)
	if !ok {
		return false
	}
	return suppress
}
