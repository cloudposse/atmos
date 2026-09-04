// Package templating renders Go templates with the Gomplate, Sprig and Atmos
// function sets. It is the only package that imports gomplate, so the rest of
// the codebase is insulated from gomplate's library API.
//
// Two execution paths exist. The fast path parses and executes the template
// with text/template directly and is lock-free, which matters because stack
// files are rendered concurrently. The gomplate renderer path is used only when
// a template references functions that gomplate provides exclusively through
// its renderer (datasources and the `tmpl` namespace); gomplate's renderer
// updates package-global metrics, so that path is serialized.
package templating

import (
	"context"
	"fmt"
	"text/template"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
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
	Datasources map[string]Datasource
	// ContextSources are read eagerly before execution and exposed on `.`; the
	// alias "." replaces the whole context. When set, Data is ignored and the
	// template is rendered by gomplate's renderer directly.
	ContextSources map[string]Datasource
}

// Renderer renders templates.
type Renderer interface {
	Render(ctx context.Context, req *Request) (string, error)
}

// Engine is a Renderer that can also read the datasources of the render in
// progress, which is how the `atmos.GomplateDatasource` template function is
// served.
type Engine interface {
	Renderer
	DatasourceReader
}

// Option configures an Engine.
type Option func(*engine)

// WithGomplate enables or disables the Gomplate function set (and datasources).
func WithGomplate(enabled bool) Option {
	defer perf.Track(nil, "templating.WithGomplate")()

	return func(e *engine) { e.gomplate = enabled }
}

// WithSprig enables or disables the Sprig function set.
func WithSprig(enabled bool) Option {
	defer perf.Track(nil, "templating.WithSprig")()

	return func(e *engine) { e.sprig = enabled }
}

// WithDatasourceCache sets the cache used by Datasource. The default is a
// process-wide cache, matching the historical `atmos.GomplateDatasource` behavior.
func WithDatasourceCache(cache *DatasourceCache) Option {
	defer perf.Track(nil, "templating.WithDatasourceCache")()

	return func(e *engine) { e.cache = cache }
}

// engine is the concrete Engine.
type engine struct {
	gomplate bool
	sprig    bool
	cache    *DatasourceCache
	live     liveRenderHolder
}

// New creates an Engine with Gomplate and Sprig enabled by default.
func New(opts ...Option) Engine {
	defer perf.Track(nil, "templating.New")()

	e := &engine{gomplate: true, sprig: true, cache: defaultDatasourceCache}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// renderPlan is a Request with its options resolved and the function map
// assembled, shared by both execution paths.
type renderPlan struct {
	req        *Request
	funcs      template.FuncMap
	left       string
	right      string
	missingKey string
}

// Render renders the request, choosing the fast path unless the template needs
// gomplate's renderer.
func (e *engine) Render(ctx context.Context, req *Request) (string, error) {
	defer perf.Track(nil, "templating.Engine.Render")()

	plan, err := e.plan(ctx, req)
	if err != nil {
		return "", err
	}

	if len(req.ContextSources) > 0 {
		return e.renderWithContext(ctx, plan)
	}

	parsed, err := parsePlain(plan, e.gomplate)
	if err != nil {
		return "", err
	}

	if !e.gomplate || !needsGomplateRenderer(parsed) {
		return executePlain(parsed, req.Data)
	}

	return e.renderWithGomplate(ctx, plan)
}

// plan resolves delimiters and the missingkey option and assembles the base
// function map for the request.
func (e *engine) plan(ctx context.Context, req *Request) (*renderPlan, error) {
	missingKey, err := missingKeyOption(req.MissingKey)
	if err != nil {
		return nil, err
	}

	plan := &renderPlan{
		req:        req,
		funcs:      e.baseFuncs(ctx, req.Funcs),
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
