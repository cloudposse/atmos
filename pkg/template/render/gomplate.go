package render

import (
	"bytes"
	"context"
	"fmt"
	"sync"

	"github.com/hairyhenderson/gomplate/v5"

	errUtils "github.com/cloudposse/atmos/errors"
)

// rendererMu serializes every use of gomplate's renderer: it records timings
// in the package-global gomplate.Metrics map, which is not safe for
// concurrent writes, and Atmos renders stack files concurrently. Templates
// themselves execute without it; only datasource reads (see Data.render) and
// context-source renders use gomplate's renderer.
var rendererMu sync.Mutex

// holdRenderer locks rendererMu for a render that binds sources, and marks
// sources so datasource reads made from inside the render don't lock it again.
// The returned function releases both.
func holdRenderer(sources *Data) func() {
	rendererMu.Lock()
	sources.rendererHeld = true
	return func() {
		sources.rendererHeld = false
		rendererMu.Unlock()
	}
}

// renderWithContext renders the template text directly through gomplate's
// renderer with eagerly-read context datasources bound to `.`.
func (e *engine) renderWithContext(ctx context.Context, plan *renderPlan) (string, error) {
	contextSources, err := toDataSources(plan.req.ContextSources)
	if err != nil {
		return "", err
	}

	defer holdRenderer(plan.sources)()

	renderer := gomplate.NewRenderer(gomplate.RenderOptions{
		Context:    contextSources,
		Funcs:      plan.funcs,
		LDelim:     plan.left,
		RDelim:     plan.right,
		MissingKey: plan.missingKey,
	})

	var buf bytes.Buffer
	err = renderer.RenderTemplates(ctx, []gomplate.Template{{Name: plan.req.Name, Text: plan.req.Text, Writer: &buf}})
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrRenderTemplate, err)
	}

	return buf.String(), nil
}
