package render

import (
	"fmt"
	"os"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// rendererMu serializes every use of gomplate's renderer: it records timings
// in the package-global gomplate.Metrics map, which is not safe for
// concurrent writes, and Atmos renders stack files concurrently. Templates
// themselves execute without it; only datasource reads (see Data.render) use
// gomplate's renderer.
var rendererMu sync.Mutex

// renderWithContext reads the context datasources eagerly, building `.` the
// way gomplate's renderer does, and then renders on the plain path. Sharing
// that path means `tmpl` and `tpl` are bound, and passed through WrapFuncs,
// the same way for every render; gomplate's renderer would bind its own
// copies after WrapFuncs ran, out of reach of policy admission.
func (e *engine) renderWithContext(plan *renderPlan) (string, error) {
	data, err := contextData(plan)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrRenderTemplate, err)
	}

	req := *plan.req
	req.Data = data
	// Prepare does not apply to context renders.
	req.Prepare = nil
	contextPlan := *plan
	contextPlan.req = &req

	// Gomplate's renderer reports the template name from `tmpl.Path`.
	parsed, err := parsePlain(&contextPlan, e.gomplate, req.Name)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrRenderTemplate, err)
	}
	out, err := executePlain(parsed, data)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrRenderTemplate, err)
	}
	return out, nil
}

// contextData reads the request's context datasources as gomplate's renderer
// does: the "." source, when present, is the whole context; otherwise each
// source is a key of a context map that also provides `.Env`.
func contextData(plan *renderPlan) (any, error) {
	if _, ok := plan.req.ContextSources["."]; ok {
		return plan.sources.Datasource(".")
	}
	data := templateContext{}
	for alias := range plan.req.ContextSources {
		value, err := plan.sources.Datasource(alias)
		if err != nil {
			return nil, err
		}
		data[alias] = value
	}
	return &data, nil
}

// templateContext is gomplate's context type for templates whose context is
// a set of named datasources.
type templateContext map[string]any

// Env maps environment variables for use in a template, as gomplate's
// context does.
func (c *templateContext) Env() map[string]string {
	defer perf.Track(nil, "render.templateContext.Env")()

	env := map[string]string{}
	for _, entry := range os.Environ() {
		if name, value, ok := strings.Cut(entry, "="); ok {
			env[name] = value
		}
	}
	return env
}
