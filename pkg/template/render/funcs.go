package render

import (
	"context"
	"sync"
	"text/template"

	"github.com/Masterminds/sprig/v3"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/template/compatibility"
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
	defer perf.Track(nil, "render.SprigFuncMap")()

	sprigFuncMapCacheOnce.Do(func() {
		sprigFuncMapCache = sprig.FuncMap()
	})
	return sprigFuncMapCache
}

// baseFuncs assembles Gomplate, Sprig and caller functions in precedence order:
// later layers win, so Sprig overrides Gomplate and the caller overrides both.
// The name parameter is the template name (Request.Name), threaded through so
// the compatibility shims in compat.go and compat_aliases.go can attribute a
// deprecation warning to the template that triggered it. Gomplate's datasource
// functions are served by sources.
func (e *engine) baseFuncs(ctx context.Context, name string, extra template.FuncMap, sources *Data) template.FuncMap {
	funcs := template.FuncMap{}
	if e.gomplate {
		gomplateFuncs := GomplateFuncs(ctx, sources)
		for k, v := range gomplateFuncs {
			funcs[k] = v
		}
		compatibility.ApplyNamespaceShims(funcs, gomplateFuncs, name, e.reporter)
	}
	if e.sprig {
		for k, v := range SprigFuncMap() {
			funcs[k] = v
		}
	}
	// The bare aliases were Gomplate v3 names, so only restore them when
	// Gomplate is enabled. The call stays after the Sprig layer so a name
	// Sprig provides keeps precedence.
	if e.gomplate {
		compatibility.ApplyBareAliasShims(funcs, name, e.reporter)
	}
	for k, v := range extra {
		funcs[k] = v
	}
	return funcs
}
