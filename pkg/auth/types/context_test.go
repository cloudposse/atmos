package types

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestAllowPrompts_DefaultTrue verifies that AllowPrompts returns true when no flag is set.
func TestAllowPrompts_DefaultTrue(t *testing.T) {
	ctx := context.Background()
	assert.True(t, AllowPrompts(ctx), "AllowPrompts should return true by default")
}

// TestAllowPrompts_ExplicitTrue verifies that AllowPrompts returns true when explicitly set to true.
func TestAllowPrompts_ExplicitTrue(t *testing.T) {
	ctx := WithAllowPrompts(context.Background(), true)
	assert.True(t, AllowPrompts(ctx), "AllowPrompts should return true when explicitly set to true")
}

// TestAllowPrompts_ExplicitFalse verifies that AllowPrompts returns false when explicitly set to false.
func TestAllowPrompts_ExplicitFalse(t *testing.T) {
	ctx := WithAllowPrompts(context.Background(), false)
	assert.False(t, AllowPrompts(ctx), "AllowPrompts should return false when explicitly set to false")
}

// TestAllowPrompts_WrongType verifies that AllowPrompts returns true when value is wrong type.
func TestAllowPrompts_WrongType(t *testing.T) {
	// Set a string value instead of bool to test robustness.
	ctx := context.WithValue(context.Background(), ContextKeyAllowPrompts, "not a bool")
	assert.True(t, AllowPrompts(ctx), "AllowPrompts should return true when value is wrong type")
}

// TestWithAllowPrompts_Propagation verifies that the flag propagates through context chain.
func TestWithAllowPrompts_Propagation(t *testing.T) {
	// Create a context with prompts disabled.
	ctx := WithAllowPrompts(context.Background(), false)

	// Derive a new context (simulating passing through function calls).
	derivedCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// The flag should still be accessible in the derived context.
	assert.False(t, AllowPrompts(derivedCtx), "AllowPrompts flag should propagate through derived contexts")
}

// TestWithAllowPrompts_Override verifies that the flag can be overridden in child context.
func TestWithAllowPrompts_Override(t *testing.T) {
	// Create a context with prompts disabled.
	ctx := WithAllowPrompts(context.Background(), false)
	assert.False(t, AllowPrompts(ctx), "First context should have prompts disabled")

	// Override in child context.
	childCtx := WithAllowPrompts(ctx, true)
	assert.True(t, AllowPrompts(childCtx), "Child context should have prompts enabled")

	// Original context should be unchanged.
	assert.False(t, AllowPrompts(ctx), "Original context should still have prompts disabled")
}

// TestAllowPrompts_UseCaseNonInteractiveWhoami demonstrates the intended use case.
// Whoami should use a non-interactive context to prevent credential prompts.
func TestAllowPrompts_UseCaseNonInteractiveWhoami(t *testing.T) {
	// Simulate Whoami creating a non-interactive context.
	baseCtx := context.Background()
	nonInteractiveCtx := WithAllowPrompts(baseCtx, false)

	// Code in authentication flow should check this before prompting.
	if AllowPrompts(nonInteractiveCtx) {
		t.Fatal("Authentication code should NOT prompt when AllowPrompts is false")
	}

	// This is the expected behavior - no prompting in Whoami.
	assert.False(t, AllowPrompts(nonInteractiveCtx),
		"Whoami should use non-interactive context to prevent credential prompts")
}
