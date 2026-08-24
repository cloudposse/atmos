package step

import (
	"context"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/say"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// Print policies for the say step.
const (
	printFallback = "fallback" // Print the message only when speech is unavailable (default).
	printAlways   = "always"   // Always print the message and also speak when possible.
	printNever    = "never"    // Speak when possible; otherwise stay silent.
)

// SayHandler speaks the step content using text-to-speech, degrading to a
// formatted info message when audio is unavailable.
type SayHandler struct {
	BaseHandler
}

func init() {
	Register(&SayHandler{
		BaseHandler: NewBaseHandler("say", CategoryUI, false),
	})
}

// Validate checks that the step has required fields.
func (h *SayHandler) Validate(step *schema.WorkflowStep) error {
	defer perf.Track(nil, "step.SayHandler.Validate")()

	return h.ValidateRequired(step, "content", step.Content)
}

// Execute speaks the content. The `print` field selects how the message is shown
// when (or whether) it is also printed.
func (h *SayHandler) Execute(ctx context.Context, step *schema.WorkflowStep, vars *Variables) (*StepResult, error) {
	defer perf.Track(nil, "step.SayHandler.Execute")()

	content, err := h.ResolveContent(ctx, step, vars)
	if err != nil {
		return nil, err
	}

	printQuote := func(text string) error {
		for _, line := range strings.Split(text, "\n") {
			ui.Info(line)
		}
		return nil
	}
	noop := func(string) error { return nil }

	mode := normalizePrint(step.Print)

	// `always` prints up front; `say` then speaks with a no-op fallback so the
	// message is not printed twice.
	fallback := printQuote
	switch mode {
	case printAlways:
		_ = printQuote(content)
		fallback = noop
	case printNever:
		fallback = noop
	}

	speaker := say.New(
		say.WithVoices(step.Voice),
		say.WithRate(step.Rate),
		say.WithFallback(fallback),
	)
	// A text-to-speech hiccup must never fail the workflow.
	_ = speaker.Speak(content)

	return NewStepResult(content), nil
}

// normalizePrint maps any input to a known print policy, defaulting to fallback.
func normalizePrint(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case printAlways:
		return printAlways
	case printNever:
		return printNever
	default:
		return printFallback
	}
}
