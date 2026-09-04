package templating

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"text/template"

	"github.com/hairyhenderson/gomplate/v5"
	"github.com/hairyhenderson/gomplate/v5/tmpl"

	errUtils "github.com/cloudposse/atmos/errors"
)

const (
	// Name of the private function the wrapper template calls to render the
	// user's template inside gomplate's renderer.
	renderFuncName = "__atmos_render"
	// Name of the private function used to read a datasource value out of a
	// live render (see engine.Datasource).
	captureFuncName = "__atmos_capture"
	// Suffix that distinguishes the wrapper template's name from the user's
	// template name in gomplate's metrics and error messages.
	wrapperSuffix = "#atmos-wrapper"
)

// rendererMu serializes every use of gomplate's renderer: it records timings
// in the package-global gomplate.Metrics map, which is not safe for
// concurrent writes, and Atmos renders stack files concurrently.
var rendererMu sync.Mutex

// liveRender is the state of the gomplate render currently executing on an
// engine. It exists only between the wrapper calling __atmos_render and that
// call returning, and is only ever touched under rendererMu.
type liveRender struct {
	tmpl        *tmpl.Template
	left, right string
	err         error
	captured    any
}

// liveRenderHolder guards the engine's live render pointer.
type liveRenderHolder struct {
	mu   sync.Mutex
	live *liveRender
}

func (h *liveRenderHolder) set(live *liveRender) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.live = live
}

func (h *liveRenderHolder) get() *liveRender {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.live
}

// renderWithGomplate renders the plan through gomplate's renderer while
// keeping req.Data as the in-memory `.` value. Gomplate's renderer only
// exposes datasource-backed contexts, so a one-action wrapper template is
// rendered instead; it hands gomplate's `tmpl` namespace to __atmos_render,
// which uses the public tmpl.Inline to parse the user's text as an associated
// template (sharing functions, delimiters and the missingkey option) and
// execute it against req.Data.
func (e *engine) renderWithGomplate(ctx context.Context, plan *renderPlan) (string, error) {
	rendererMu.Lock()
	defer rendererMu.Unlock()

	datasources, err := toDataSources(plan.req.Datasources)
	if err != nil {
		return "", err
	}

	state := &liveRender{left: plan.left, right: plan.right}
	e.live.set(state)
	defer e.live.set(nil)

	renderer := gomplate.NewRenderer(gomplate.RenderOptions{
		Datasources: datasources,
		Funcs:       wrapperFuncs(plan, state),
		LDelim:      plan.left,
		RDelim:      plan.right,
		MissingKey:  plan.missingKey,
	})

	var buf bytes.Buffer
	wrapper := gomplate.Template{
		Name:   plan.req.Name + wrapperSuffix,
		Text:   plan.left + " " + renderFuncName + " tmpl " + plan.right,
		Writer: &buf,
	}
	if err := renderer.RenderTemplates(ctx, []gomplate.Template{wrapper}); err != nil {
		if state.err != nil {
			// Return the user's template error verbatim (`template: name:L:C: ...`)
			// rather than gomplate's wrapper-level decoration of it.
			return "", state.err
		}
		return "", fmt.Errorf("%w: %w", errUtils.ErrRenderTemplate, err)
	}

	return buf.String(), nil
}

// wrapperFuncs returns the plan's functions plus the two private functions the
// wrapper template and engine.Datasource rely on.
func wrapperFuncs(plan *renderPlan, state *liveRender) template.FuncMap {
	funcs := make(template.FuncMap, len(plan.funcs)+2)
	for k, v := range plan.funcs {
		funcs[k] = v
	}
	funcs[renderFuncName] = func(t *tmpl.Template) (string, error) {
		state.tmpl = t
		out, inlineErr := t.Inline(plan.req.Name, plan.req.Text, plan.req.Data)
		if inlineErr != nil {
			state.err = inlineErr
		}
		return out, inlineErr
	}
	funcs[captureFuncName] = func(v any) string {
		state.captured = v
		return ""
	}
	return funcs
}

// renderWithContext renders the template text directly through gomplate's
// renderer with eagerly-read context datasources bound to `.`.
func (e *engine) renderWithContext(ctx context.Context, plan *renderPlan) (string, error) {
	rendererMu.Lock()
	defer rendererMu.Unlock()

	contextSources, err := toDataSources(plan.req.ContextSources)
	if err != nil {
		return "", err
	}
	datasources, err := toDataSources(plan.req.Datasources)
	if err != nil {
		return "", err
	}

	renderer := gomplate.NewRenderer(gomplate.RenderOptions{
		Context:     contextSources,
		Datasources: datasources,
		Funcs:       plan.funcs,
		LDelim:      plan.left,
		RDelim:      plan.right,
		MissingKey:  plan.missingKey,
	})

	var buf bytes.Buffer
	err = renderer.RenderTemplates(ctx, []gomplate.Template{{Name: plan.req.Name, Text: plan.req.Text, Writer: &buf}})
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrRenderTemplate, err)
	}

	return buf.String(), nil
}
