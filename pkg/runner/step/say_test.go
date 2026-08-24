package step

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestSayHandlerValidation(t *testing.T) {
	handler, ok := Get("say")
	require.True(t, ok)

	t.Run("valid with content", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "test", Type: "say", Content: "Hello"}
		assert.NoError(t, handler.Validate(step))
	})

	t.Run("missing content", func(t *testing.T) {
		step := &schema.WorkflowStep{Name: "test", Type: "say"}
		assert.Error(t, handler.Validate(step))
	})
}

func TestSayHandlerMetadata(t *testing.T) {
	handler, ok := Get("say")
	require.True(t, ok)

	assert.Equal(t, "say", handler.GetName())
	assert.Equal(t, CategoryUI, handler.GetCategory())
	assert.False(t, handler.RequiresTTY(), "say must run in non-interactive workflows")
}

func TestSayHandlerExecute(t *testing.T) {
	// Force the fallback path so tests never emit audio or shell out.
	t.Setenv("GO_TEST", "1")

	handler, ok := Get("say")
	require.True(t, ok)

	// Cover every print policy: each must succeed and return the resolved content.
	for _, mode := range []string{"", "fallback", "always", "never", "unknown"} {
		t.Run("print="+mode, func(t *testing.T) {
			step := &schema.WorkflowStep{
				Name:    "notify",
				Type:    "say",
				Content: "Deploying {{ .steps.env.value }}",
				Voice:   []string{"Samantha", "Zira", "en-us"},
				Rate:    "fast",
				Print:   mode,
			}
			vars := NewVariables()
			vars.Set("env", NewStepResult("prod"))

			result, err := handler.Execute(context.Background(), step, vars)
			require.NoError(t, err)
			assert.Equal(t, "Deploying prod", result.Value)
		})
	}
}

func TestNormalizePrint(t *testing.T) {
	assert.Equal(t, printFallback, normalizePrint(""))
	assert.Equal(t, printFallback, normalizePrint("bogus"))
	assert.Equal(t, printAlways, normalizePrint("always"))
	assert.Equal(t, printAlways, normalizePrint("  ALWAYS "))
	assert.Equal(t, printNever, normalizePrint("Never"))
}
