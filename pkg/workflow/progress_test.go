package workflow

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terminal"
	"github.com/cloudposse/atmos/pkg/ui/theme"
)

// fakeTerminal is a minimal terminal.Terminal implementation for tests.
// Only Width returns a configurable value; the remaining methods are no-ops
// so the renderer can be exercised without a real TTY.
type fakeTerminal struct {
	width int
}

func (f fakeTerminal) Write(string) error                  { return nil }
func (f fakeTerminal) IsTTY(terminal.Stream) bool          { return true }
func (f fakeTerminal) IsPiped(terminal.Stream) bool        { return false }
func (f fakeTerminal) ColorProfile() terminal.ColorProfile { return terminal.ColorNone }
func (f fakeTerminal) Width(terminal.Stream) int           { return f.width }
func (f fakeTerminal) Height(terminal.Stream) int          { return 24 }
func (f fakeTerminal) SetTitle(string)                     {}
func (f fakeTerminal) RestoreTitle()                       {}
func (f fakeTerminal) Alert()                              {}

// newTestProgressRenderer builds an enabled ProgressRenderer with an injected
// fake terminal so the rendering logic can be tested without a real TTY.
func newTestProgressRenderer(total, current, width int) *ProgressRenderer {
	return &ProgressRenderer{
		progress: progress.New(
			progress.WithWidth(progressBarWidth),
			progress.WithoutPercentage(),
		),
		total:    total,
		current:  current,
		enabled:  true,
		styles:   theme.GetCurrentStyles(),
		term:     fakeTerminal{width: width},
		stepName: "deploy",
	}
}

func TestProgressRenderer_FormatProgressLine(t *testing.T) {
	t.Run("with explicit label includes label and count", func(t *testing.T) {
		r := newTestProgressRenderer(5, 2, 80)

		out := r.formatProgressLine("[2/5] deploy")

		assert.Contains(t, out, "deploy")
		assert.Contains(t, out, "2/5")
	})

	t.Run("empty label falls back to styled step name", func(t *testing.T) {
		r := newTestProgressRenderer(3, 1, 80)

		out := r.formatProgressLine("")

		assert.Contains(t, out, "deploy")
		assert.Contains(t, out, "1/3")
	})

	t.Run("zero width falls back to default width", func(t *testing.T) {
		r := newTestProgressRenderer(4, 2, 0)

		out := r.formatProgressLine("step")

		assert.Contains(t, out, "2/4")
		// width should have been set to the default for layout calculations.
		assert.Equal(t, defaultWidth, r.width)
	})

	t.Run("zero total avoids divide-by-zero", func(t *testing.T) {
		r := newTestProgressRenderer(0, 0, 80)

		out := r.formatProgressLine("step")

		assert.Contains(t, out, "0/0")
	})

	t.Run("narrow width clamps gap without panic", func(t *testing.T) {
		r := newTestProgressRenderer(10, 5, 5)

		out := r.formatProgressLine("a-very-long-step-label-that-exceeds-width")

		assert.Contains(t, out, "5/10")
	})

	t.Run("nil styles falls back to plain step name", func(t *testing.T) {
		r := newTestProgressRenderer(2, 1, 80)
		r.styles = nil

		out := r.formatProgressLine("")

		assert.Contains(t, out, "deploy")
	})
}

func TestProgressRenderer_Update(t *testing.T) {
	r := newTestProgressRenderer(5, 0, 80)

	r.Update(3, "plan")

	assert.Equal(t, 3, r.current)
	assert.Equal(t, "plan", r.stepName)
}

func TestProgressRenderer_RenderMethods(t *testing.T) {
	// These render to stderr via the ui package; we assert they do not panic
	// and that the enabled guards are respected.
	r := newTestProgressRenderer(5, 2, 80)

	assert.NotPanics(t, func() {
		r.Render()
		r.RenderWithLabel("[2/5] deploy")
		r.RenderPermanent("[2/5] deploy")
	})
}

func TestProgressRenderer_DoneAndIsEnabled(t *testing.T) {
	r := newTestProgressRenderer(5, 2, 80)
	assert.True(t, r.IsEnabled())

	r.Done()

	assert.False(t, r.IsEnabled())
	assert.False(t, r.enabled)
}

func TestProgressRenderer_NilAndDisabledAreNoOps(t *testing.T) {
	var nilRenderer *ProgressRenderer

	assert.NotPanics(t, func() {
		nilRenderer.Update(1, "x")
		nilRenderer.Render()
		nilRenderer.RenderWithLabel("x")
		nilRenderer.RenderPermanent("x")
		nilRenderer.Done()
	})
	assert.False(t, nilRenderer.IsEnabled())

	disabled := newTestProgressRenderer(5, 2, 80)
	disabled.enabled = false

	assert.NotPanics(t, func() {
		disabled.Update(2, "y")
		disabled.Render()
		disabled.RenderWithLabel("y")
		disabled.RenderPermanent("y")
		disabled.Done()
	})
	// Update should be a no-op when disabled.
	assert.Equal(t, 2, disabled.current)
	assert.False(t, disabled.IsEnabled())
}

func TestNewProgressRenderer_DisabledWhenNotConfigured(t *testing.T) {
	t.Run("nil when progress not enabled", func(t *testing.T) {
		wf := &schema.WorkflowDefinition{}

		r := NewProgressRenderer(wf, 3)

		assert.Nil(t, r)
	})

	t.Run("nil when enabled but no TTY", func(t *testing.T) {
		// Tests run without a TTY, so even with progress requested the renderer
		// is disabled and returns nil.
		enabled := true
		wf := &schema.WorkflowDefinition{
			Show: &schema.ShowConfig{Progress: &enabled},
		}

		r := NewProgressRenderer(wf, 3)

		assert.Nil(t, r)
	})
}

func TestProgressRenderer_FormatProgressLineSpacing(t *testing.T) {
	// Sanity check that a wide layout produces a gap of spaces between the
	// left label and the right-aligned progress bar.
	r := newTestProgressRenderer(2, 1, 120)

	out := r.formatProgressLine("step")

	assert.True(t, strings.Contains(out, "  "), "expected padding gap in %q", out)
}
