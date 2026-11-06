// Package render renders Go templates with the Gomplate, Sprig and Atmos
// function sets. It is the only package that imports gomplate, so the rest of
// the codebase is insulated from gomplate's library API.
//
// Templates are parsed and executed with text/template directly, lock-free,
// which matters because stack files are rendered concurrently. Gomplate's
// renderer-only functions are provided without its renderer: Data reads
// datasources lazily and the `tmpl` namespace is bound to the parsed template.
// Gomplate's renderer is used only to read datasources, including the context
// datasources of Request.ContextSources; it updates package-global metrics,
// so those reads are serialized.
package render

import (
	"context"
	"fmt"
	"net/url"
	"text/template"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/template/compatibility"
)

const (
	// MissingKeyError makes execution fail when a map is indexed with a missing key.
	MissingKeyError = "error"
	// MissingKeyDefault substitutes the zero value when a map is indexed with a missing key.
	MissingKeyDefault = "default"
	// DefaultLeftDelim is the default left action delimiter.
	DefaultLeftDelim = "{{"
	// DefaultRightDelim is the default right action delimiter.
	DefaultRightDelim = "}}"
)

// Datasource is a named gomplate datasource definition.
type Datasource struct {
	// URL is the datasource URL (any scheme gomplate supports). A bare absolute
	// path is treated as a file:// URL.
	URL string
	// Headers are extra HTTP headers sent to http(s) datasources.
	Headers map[string][]string
}

// Request describes one template render.
type Request struct {
	// Name is the template name used in error messages.
	Name string
	// Text is the template source.
	Text string
	// Data is the value bound to `.` when the template executes. It may be a
	// map or any Go value; it is never serialized.
	Data any
	// Funcs are functions merged last, so they override Gomplate and Sprig
	// functions with the same name (e.g. the `atmos` namespace).
	Funcs template.FuncMap
	// LeftDelim and RightDelim override the default action delimiters.
	LeftDelim  string
	RightDelim string
	// MissingKey is the text/template "missingkey" option; empty means MissingKeyError.
	MissingKey string
	// Datasources are read lazily when the template calls `datasource`/`ds`/`include`.
	// They are added to Sources (or to a fresh Data when Sources is nil).
	Datasources map[string]Datasource
	// Sources serves the template's datasource functions. Pass the same Data to
	// functions that read datasources themselves (such as the `atmos`
	// namespace) so they share its definitions. Nil means a fresh Data.
	Sources *Data
	// ContextSources are read eagerly before execution and exposed on `.`; the
	// alias "." replaces the whole context, as in gomplate's renderer. When set,
	// Data is ignored. They are also readable as datasources (for example
	// `ds "config"`).
	ContextSources map[string]Datasource
	// WrapFuncs, when set, receives the fully assembled function map (Gomplate,
	// Sprig, compatibility shims and Funcs) and the render's datasources, and
	// returns the map the template is parsed with. Policy admission uses it to
	// guard every function call. Gomplate's `tmpl` namespace and `tpl` alias
	// need the parsed root, so they are passed through it in a second call of
	// their own once they are bound.
	WrapFuncs FuncsWrapper
	// Prepare, when set, runs on the parsed template before it executes, for
	// callers that rewrite or guard the parse tree. Gomplate's `tmpl`/`tpl`
	// functions are unavailable when it is set, so run-time templates can't
	// escape it. It does not apply to ContextSources renders.
	Prepare func(*template.Template)
}

// FuncsWrapper transforms the assembled function map for one render.
type FuncsWrapper func(funcs template.FuncMap, sources *Data) (template.FuncMap, error)

// Renderer renders templates.
type Renderer interface {
	Render(ctx context.Context, req *Request) (string, error)
}

// Engine is the Renderer that serves Gomplate, Sprig and caller functions.
type Engine interface {
	Renderer
}

// Option configures an Engine.
type Option func(*engine)

// WithGomplate enables or disables the Gomplate function set (and datasources).
func WithGomplate(enabled bool) Option {
	defer perf.Track(nil, "render.WithGomplate")()

	return func(e *engine) { e.gomplate = enabled }
}

// WithSprig enables or disables the Sprig function set.
func WithSprig(enabled bool) Option {
	defer perf.Track(nil, "render.WithSprig")()

	return func(e *engine) { e.sprig = enabled }
}

// WithDeprecationReporter overrides the reporter notified when a template uses
// a gomplate v3 name kept alive by the compatibility shims. The default logs a
// warning via pkg/logger, deduplicated per template per name.
func WithDeprecationReporter(reporter compatibility.DeprecationReporter) Option {
	defer perf.Track(nil, "render.WithDeprecationReporter")()

	return func(e *engine) {
		if reporter != nil {
			e.reporter = reporter
		}
	}
}

// engine is the concrete Engine.
type engine struct {
	gomplate bool
	sprig    bool
	reporter compatibility.DeprecationReporter
}

// New creates an Engine with Gomplate and Sprig enabled by default.
func New(opts ...Option) Engine {
	defer perf.Track(nil, "render.New")()

	e := &engine{gomplate: true, sprig: true, reporter: compatibility.DefaultReporter()}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// renderPlan is a Request with its options resolved and the function map
// assembled, shared by both execution paths.
type renderPlan struct {
	req        *Request
	sources    *Data
	funcs      template.FuncMap
	left       string
	right      string
	missingKey string
}

// Render renders the request, choosing the fast path unless the template needs
// gomplate's renderer.
func (e *engine) Render(ctx context.Context, req *Request) (string, error) {
	defer perf.Track(nil, "render.Engine.Render")()

	plan, err := e.plan(ctx, req)
	if err != nil {
		return "", err
	}

	if len(req.ContextSources) > 0 {
		return e.renderWithContext(plan)
	}

	parsed, err := parsePlain(plan, e.gomplate, "")
	if err != nil {
		return "", err
	}
	if e.gomplate {
		compatibility.LintDeprecatedUsage(parsed, plan.req.Name, planSourceURLs(plan), e.reporter)
	}

	if req.Prepare != nil {
		req.Prepare(parsed)
	}
	return executePlain(parsed, req.Data)
}

// plan resolves delimiters, the missingkey option and the render's
// datasources, and assembles the function map for the request.
func (e *engine) plan(ctx context.Context, req *Request) (*renderPlan, error) {
	missingKey, err := missingKeyOption(req.MissingKey)
	if err != nil {
		return nil, err
	}

	sources := req.Sources
	if sources == nil {
		sources = &Data{}
	}
	// Bind the render context even with Gomplate disabled: ContextSources are
	// still read, and must honor this render's deadline.
	sources.bindContext(ctx)
	if err := sources.defineAll(req.Datasources); err != nil {
		return nil, err
	}
	if err := sources.defineAll(req.ContextSources); err != nil {
		return nil, err
	}

	funcs := e.baseFuncs(ctx, req.Name, req.Funcs, sources)
	if req.WrapFuncs != nil {
		if funcs, err = req.WrapFuncs(funcs, sources); err != nil {
			return nil, err
		}
	}

	plan := &renderPlan{
		req:        req,
		sources:    sources,
		funcs:      funcs,
		left:       req.LeftDelim,
		right:      req.RightDelim,
		missingKey: missingKey,
	}
	if plan.left == "" {
		plan.left = DefaultLeftDelim
	}
	if plan.right == "" {
		plan.right = DefaultRightDelim
	}
	return plan, nil
}

// missingKeyOption validates the missingkey option, defaulting to error.
func missingKeyOption(value string) (string, error) {
	switch value {
	case "":
		return MissingKeyError, nil
	case MissingKeyError, MissingKeyDefault, "zero", "invalid":
		return value, nil
	default:
		return "", fmt.Errorf("%w: unsupported missingkey option %q", errUtils.ErrInvalidTemplateSettings, value)
	}
}

// planSourceURLs returns the render's datasource URLs by alias for the
// compatibility lint. A plan built without Data falls back to the request's
// definitions; unparseable URLs are skipped, since defining them fails the
// render anyway.
func planSourceURLs(plan *renderPlan) map[string]*url.URL {
	sources := map[string]*url.URL{}
	if plan.sources != nil {
		for alias, source := range plan.sources.Sources {
			if source != nil && source.URL != nil {
				sources[alias] = source.URL
			}
		}
		return sources
	}
	for alias, definition := range plan.req.Datasources {
		if parsed, err := parseDatasourceURL(definition.URL); err == nil {
			sources[alias] = parsed
		}
	}
	return sources
}
