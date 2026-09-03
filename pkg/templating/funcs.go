package templating

import (
	"context"
	"sync"
	"text/template"

	"github.com/Masterminds/sprig/v3"
	"github.com/hairyhenderson/gomplate/v5"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Sprig function maps are expensive to create (173MB+ allocations per profile
// run) and immutable, so one copy is built and shared. Gomplate functions are
// not cached because they close over the render context.
var (
	sprigFuncMapCache     template.FuncMap
	sprigFuncMapCacheOnce sync.Once
)

// SprigFuncMap returns the shared Sprig function map. Callers must not mutate it.
func SprigFuncMap() template.FuncMap {
	defer perf.Track(nil, "templating.SprigFuncMap")()

	sprigFuncMapCacheOnce.Do(func() {
		sprigFuncMapCache = sprig.FuncMap()
	})
	return sprigFuncMapCache
}

// GomplateFuncs returns the Gomplate function map bound to ctx. It does not
// include the datasource functions, which gomplate only provides through its
// renderer; see Engine.Render for how those are made available.
func GomplateFuncs(ctx context.Context) template.FuncMap {
	defer perf.Track(nil, "templating.GomplateFuncs")()

	return gomplate.CreateFuncs(ctx)
}

// rendererOnlyFuncs are the functions gomplate registers only inside its
// renderer (see gomplate's internal/funcs/datasource.go and template.go). A
// template that calls any of them must be rendered through gomplate.
var rendererOnlyFuncs = map[string]struct{}{
	"datasource":          {},
	"ds":                  {},
	"include":             {},
	"defineDatasource":    {},
	"datasourceExists":    {},
	"datasourceReachable": {},
	"listDatasources":     {},
	"_datasource":         {},
	"tmpl":                {},
	"tpl":                 {},
}

// rendererOnlyMethods are `namespace.Method` chains that need a live gomplate
// render because they read datasources.
var rendererOnlyMethods = map[string]struct{}{
	"atmos.GomplateDatasource": {},
}

// stubFuncs lets templates that reference renderer-only functions parse on the
// fast path so their AST can be inspected. They are never executed: a template
// that uses them is routed to the gomplate renderer, which registers the real
// implementations under the same names.
func stubFuncs() template.FuncMap {
	stubs := make(template.FuncMap, len(rendererOnlyFuncs))
	for name := range rendererOnlyFuncs {
		stubs[name] = func(...any) (any, error) {
			return nil, errUtils.ErrGomplateDatasourceUnavailable
		}
	}
	return stubs
}

// baseFuncs assembles Gomplate, Sprig and caller functions in precedence order:
// later layers win, so Sprig overrides Gomplate and the caller overrides both.
func (e *engine) baseFuncs(ctx context.Context, extra template.FuncMap) template.FuncMap {
	funcs := template.FuncMap{}
	if e.gomplate {
		for k, v := range GomplateFuncs(ctx) {
			funcs[k] = v
		}
	}
	if e.sprig {
		for k, v := range SprigFuncMap() {
			funcs[k] = v
		}
	}
	for k, v := range extra {
		funcs[k] = v
	}
	return funcs
}
