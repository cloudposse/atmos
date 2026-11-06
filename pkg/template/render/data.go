package render

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"sort"
	"strings"
	"text/template"

	"github.com/hairyhenderson/gomplate/v5"
	"github.com/hairyhenderson/yaml"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

var errInvalidDatasource = errors.New("invalid datasource")

// Source retains the URL and headers so policy admission can inspect a datasource before reading it.
type Source struct {
	Alias  string
	URL    *url.URL
	Header http.Header
}

// Data holds the datasources belonging to a single template evaluation and
// reads them lazily, the way gomplate's `datasource`/`ds`/`include` functions
// do. Each read goes through a private gomplate renderer whose only functions
// capture the native result, so datasource values keep gomplate's types
// (including integers and EJSON values) and the template being rendered never
// needs gomplate's renderer itself.
//
// Like a gomplate Renderer, a Data value must not be used concurrently.
type Data struct {
	Ctx          context.Context
	Sources      map[string]*Source
	ExtraHeaders map[string]http.Header

	renderers map[string]gomplate.Renderer
	alias     string
	args      []string
	result    any
	// rendererHeld is set while the engine executes a template through
	// gomplate's renderer with this Data bound to it. The engine holds
	// rendererMu for that whole render, so reads made from inside it must not
	// lock rendererMu again.
	rendererHeld bool
}

// NewData builds datasources from alias=URL and alias=Header: value definitions.
func NewData(sources, headers []string) (*Data, error) {
	defer perf.Track(nil, "render.NewData")()

	d := &Data{Ctx: context.Background(), Sources: map[string]*Source{}, ExtraHeaders: map[string]http.Header{}}
	for _, definition := range headers {
		alias, header, ok := strings.Cut(definition, "=")
		name, value, valid := strings.Cut(header, ":")
		if !ok || !valid || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("%w: invalid header definition", errInvalidDatasource)
		}
		if d.ExtraHeaders[alias] == nil {
			d.ExtraHeaders[alias] = http.Header{}
		}
		d.ExtraHeaders[alias].Add(strings.TrimSpace(name), strings.TrimSpace(value))
	}
	for _, definition := range sources {
		alias, value, ok := strings.Cut(definition, "=")
		if !ok {
			return nil, fmt.Errorf("%w: expected alias=URL", errInvalidDatasource)
		}
		if _, err := d.DefineDatasource(alias, value); err != nil {
			return nil, err
		}
	}
	return d, nil
}

// DefineDatasource registers an alias once, using Gomplate's own URL parsing rules.
func (d *Data) DefineDatasource(alias, value string) (string, error) {
	defer perf.Track(nil, "render.DefineDatasource")()

	if alias == "" {
		return "", fmt.Errorf("%w: alias must be provided", errInvalidDatasource)
	}
	if d.DatasourceExists(alias) {
		return "", nil
	}
	var node yaml.Node
	if err := node.Encode(map[string]string{"url": value}); err != nil {
		return "", err
	}
	var source gomplate.DataSource
	if err := source.UnmarshalYAML(&node); err != nil {
		return "", err
	}
	d.define(alias, source.URL, d.ExtraHeaders[alias])
	return "", nil
}

// define registers an already-parsed datasource URL, bypassing a string round trip.
func (d *Data) define(alias string, target *url.URL, header http.Header) {
	if d.Sources == nil {
		d.Sources = map[string]*Source{}
	}
	d.Sources[alias] = &Source{Alias: alias, URL: target, Header: header}
}

// defineAll registers datasource definitions, failing on the first invalid URL.
func (d *Data) defineAll(sources map[string]Datasource) error {
	for alias, source := range sources {
		if alias == "" {
			return fmt.Errorf("%w: datasource alias must not be empty", errUtils.ErrInvalidDatasourceURL)
		}
		parsed, err := parseDatasourceURL(source.URL)
		if err != nil {
			return fmt.Errorf("%w: %s: %w", errUtils.ErrInvalidDatasourceURL, alias, err)
		}
		var header http.Header
		if len(source.Headers) > 0 {
			header = http.Header(source.Headers)
		}
		d.define(alias, parsed, header)
	}
	return nil
}

// DatasourceExists reports whether the alias has been registered.
func (d *Data) DatasourceExists(alias string) bool {
	defer perf.Track(nil, "render.DatasourceExists")()

	_, exists := d.Sources[alias]
	return exists
}

// ListDatasources returns the registered aliases in stable order.
func (d *Data) ListDatasources() []string {
	defer perf.Track(nil, "render.ListDatasources")()

	aliases := make([]string, 0, len(d.Sources))
	for alias := range d.Sources {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}

// Datasource reads and parses a datasource without serializing its native return value.
func (d *Data) Datasource(alias string, args ...string) (any, error) {
	defer perf.Track(nil, "render.Datasource")()

	return d.read("datasource", alias, args)
}

// Include reads a datasource without parsing its contents.
func (d *Data) Include(alias string, args ...string) (string, error) {
	defer perf.Track(nil, "render.Include")()

	value, err := d.read("include", alias, args)
	if err != nil {
		return "", err
	}
	return value.(string), nil
}

// DatasourceReachable preserves Gomplate's registered-alias probe behavior.
func (d *Data) DatasourceReachable(alias string, args ...string) bool {
	defer perf.Track(nil, "render.DatasourceReachable")()

	if !d.DatasourceExists(alias) {
		return false
	}
	_, err := d.Include(alias, args...)
	return err == nil
}

func (d *Data) initRenderer(alias string) error {
	sources := make(map[string]gomplate.DataSource, len(d.Sources))
	for name, source := range d.Sources {
		if source == nil || source.URL == nil {
			return fmt.Errorf("%w: datasource %q has no URL", errInvalidDatasource, name)
		}
		sources[name] = gomplate.DataSource{URL: source.URL, Header: source.Header}
	}
	// A capture function preserves Gomplate's native data types, including integers and EJSON values.
	// Arguments are functions, never template source, so URLs cannot inject template expressions.
	funcs := template.FuncMap{
		"capture": func(value any) string { d.result = value; return "" },
		"alias":   func() string { return d.alias },
		"arg":     func(i int) string { return d.args[i] },
	}
	if d.renderers == nil {
		d.renderers = make(map[string]gomplate.Renderer)
	}
	d.renderers[alias] = gomplate.NewRenderer(gomplate.RenderOptions{Datasources: sources, ExtraHeaders: d.ExtraHeaders, Funcs: funcs})
	return nil
}

func (d *Data) read(function, alias string, args []string) (any, error) {
	if !d.DatasourceExists(alias) {
		target, err := url.Parse(alias)
		if err != nil || !target.IsAbs() {
			return nil, fmt.Errorf("%w: undefined datasource %q", errInvalidDatasource, alias)
		}
		if _, err := d.DefineDatasource(alias, alias); err != nil {
			return nil, err
		}
	}
	if d.renderers[alias] == nil {
		if err := d.initRenderer(alias); err != nil {
			return nil, err
		}
	}
	d.alias, d.args, d.result = alias, args, nil
	text := "{{ capture (" + function + " (alias)"
	for i := range args {
		text += fmt.Sprintf(" (arg %d)", i)
	}
	text += ") }}"
	ctx := d.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if err := d.render(ctx, alias, text); err != nil {
		return nil, err
	}
	return d.result, nil
}

// render runs the alias's private renderer. Gomplate's renderer records
// timings in a package-global map, so every use is serialized on rendererMu
// unless this goroutine already holds it (see rendererHeld).
func (d *Data) render(ctx context.Context, alias, text string) error {
	if !d.rendererHeld {
		rendererMu.Lock()
		defer rendererMu.Unlock()
	}
	return d.renderers[alias].Render(ctx, "datasource", text, io.Discard)
}

// datasourceFuncs returns the gomplate datasource functions served by d.
func datasourceFuncs(d *Data) template.FuncMap {
	return template.FuncMap{
		"datasource":          d.Datasource,
		"ds":                  d.Datasource,
		"include":             d.Include,
		"datasourceExists":    d.DatasourceExists,
		"datasourceReachable": d.DatasourceReachable,
		"defineDatasource":    d.DefineDatasource,
		"listDatasources":     d.ListDatasources,
	}
}

// GomplateFuncs returns Gomplate's function map bound to ctx, with the
// datasource functions served by sources (an empty Data when nil) and, on
// Windows, the native file namespace. It does not include the `tmpl`
// namespace, which gomplate only provides through its renderer.
func GomplateFuncs(ctx context.Context, sources *Data) template.FuncMap {
	defer perf.Track(nil, "render.GomplateFuncs")()

	if sources == nil {
		sources = &Data{}
	}
	sources.Ctx = ctx
	funcs := gomplate.CreateFuncs(ctx)
	if runtime.GOOS == "windows" {
		funcs["file"] = func() any { return &nativeFileFuncs{} }
	}
	for name, fn := range datasourceFuncs(sources) {
		funcs[name] = fn
	}
	return funcs
}

// WithExperimental enables gomplate's experimental functions for renders bound to ctx.
func WithExperimental(ctx context.Context) context.Context {
	defer perf.Track(nil, "render.WithExperimental")()

	return gomplate.SetExperimental(ctx)
}

// JSON uses Gomplate's native JSON and EJSON decoder.
func JSON(value string) (map[string]any, error) {
	defer perf.Track(nil, "render.JSON")()

	return gomplate.CreateFuncs(context.Background())["json"].(func(any) (map[string]any, error))(value)
}

// CSV uses Gomplate's native CSV decoder.
func CSV(value string) ([][]string, error) {
	defer perf.Track(nil, "render.CSV")()

	return gomplate.CreateFuncs(context.Background())["csv"].(func(...string) ([][]string, error))(value)
}

// TOML uses Gomplate's native TOML decoder.
func TOML(value string) (any, error) {
	defer perf.Track(nil, "render.TOML")()

	return gomplate.CreateFuncs(context.Background())["toml"].(func(any) (any, error))(value)
}
