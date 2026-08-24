package workflow

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestNewShowRenderer(t *testing.T) {
	r := NewShowRenderer()

	require.NotNil(t, r)
	assert.False(t, r.headerDone)
}

func TestShowRenderer_FormatHeader(t *testing.T) {
	r := NewShowRenderer()

	out := r.formatHeader("deploy", "Deploys the stack")

	assert.Equal(t, "# deploy\nDeploys the stack", out)
}

func TestShowRenderer_FormatFlags(t *testing.T) {
	r := NewShowRenderer()

	t.Run("empty map returns empty string", func(t *testing.T) {
		assert.Empty(t, r.formatFlags(map[string]string{}))
	})

	t.Run("includes each key and value", func(t *testing.T) {
		out := r.formatFlags(map[string]string{
			"stack": "prod",
			"env":   "use1",
		})

		assert.Contains(t, out, "stack")
		assert.Contains(t, out, "prod")
		assert.Contains(t, out, "env")
		assert.Contains(t, out, "use1")
	})

	t.Run("keys are sorted alphabetically", func(t *testing.T) {
		out := r.formatFlags(map[string]string{
			"zeta":  "z",
			"alpha": "a",
		})

		assert.Less(t, indexOf(out, "alpha"), indexOf(out, "zeta"),
			"alpha should appear before zeta in %q", out)
	})

	t.Run("nil styles still formats", func(t *testing.T) {
		plain := &ShowRenderer{styles: nil}

		out := plain.formatFlags(map[string]string{"k": "v"})

		assert.Equal(t, "k: v", out)
	})
}

func TestShowRenderer_RenderHeaderIfNeeded(t *testing.T) {
	enabled := true

	t.Run("header only", func(t *testing.T) {
		r := NewShowRenderer()
		wf := &schema.WorkflowDefinition{
			Description: "Deploys the stack",
			Show:        &schema.ShowConfig{Header: &enabled},
		}

		assert.NotPanics(t, func() {
			r.RenderHeaderIfNeeded(wf, "deploy", nil)
		})
		assert.True(t, r.headerDone)
	})

	t.Run("flags only", func(t *testing.T) {
		r := NewShowRenderer()
		wf := &schema.WorkflowDefinition{
			Show: &schema.ShowConfig{Flags: &enabled},
		}

		assert.NotPanics(t, func() {
			r.RenderHeaderIfNeeded(wf, "deploy", map[string]string{"stack": "prod"})
		})
		assert.True(t, r.headerDone)
	})

	t.Run("header and flags together", func(t *testing.T) {
		r := NewShowRenderer()
		wf := &schema.WorkflowDefinition{
			Description: "Deploys the stack",
			Show:        &schema.ShowConfig{Header: &enabled, Flags: &enabled},
		}

		assert.NotPanics(t, func() {
			r.RenderHeaderIfNeeded(wf, "deploy", map[string]string{"stack": "prod"})
		})
		assert.True(t, r.headerDone)
	})

	t.Run("headerDone short-circuits second call", func(t *testing.T) {
		r := NewShowRenderer()
		r.headerDone = true
		wf := &schema.WorkflowDefinition{
			Description: "Deploys the stack",
			Show:        &schema.ShowConfig{Header: &enabled},
		}

		// Should return immediately without re-rendering.
		assert.NotPanics(t, func() {
			r.RenderHeaderIfNeeded(wf, "deploy", nil)
		})
		assert.True(t, r.headerDone)
	})

	t.Run("nothing enabled still marks done", func(t *testing.T) {
		r := NewShowRenderer()
		wf := &schema.WorkflowDefinition{Description: "Deploys"}

		r.RenderHeaderIfNeeded(wf, "deploy", nil)

		assert.True(t, r.headerDone)
	})
}

// indexOf returns the index of substr in s, or -1 if not present.
func indexOf(s, substr string) int {
	return strings.Index(s, substr)
}
