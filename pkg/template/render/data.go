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
// do. Reads go through gomplate's own datasource reader, captured once per
// alias from a private gomplate renderer, so datasource values keep gomplate's
// types (including integers and EJSON values) and the template being rendered
// never needs gomplate's renderer itself.
//
// Like a gomplate Renderer, a Data value must not be used concurrently.
type Data struct {
	Ctx          context.Context
	Sources      map[string]*Source
	ExtraHeaders map[string]http.Header

	readers map[string]gomplateReader
}

// gomplateReader is gomplate's datasource namespace, which its renderer exposes
// as the `_datasource` function. Its methods read through the renderer's source
// reader and never touch gomplate's package-global metrics, so reads made
// through it need no lock.
type gomplateReader interface {
	Datasource(alias string, args ...string) (any, error)
	Include(alias string, args ...string) (string, error)
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
	target, err := parseDefinitionURL(value)
	if err != nil {
		return "", err
	}
	d.define(alias, target, d.ExtraHeaders[alias])
	return "", nil
}

// parseDefinitionURL parses a datasource definition URL exactly as gomplate
// parses a `defineDatasource` value.
func parseDefinitionURL(value string) (*url.URL, error) {
	var node yaml.Node
	if err := node.Encode(map[string]string{"url": value}); err != nil {
		return nil, err
	}
	var source gomplate.DataSource
	if err := source.UnmarshalYAML(&node); err != nil {
		return nil, err
	}
	return source.URL, nil
}

// effectiveSource returns the definition a read of alias uses: the registered
// one, or for an undefined absolute URL the definition the read will register.
// It returns nil for an alias that cannot be read.
func (d *Data) effectiveSource(alias string) *Source {
	if source, ok := d.Sources[alias]; ok {
		return source
	}
	if target, err := url.Parse(alias); err != nil || !target.IsAbs() {
		return nil
	}
	target, err := parseDefinitionURL(alias)
	if err != nil {
		return nil
	}
	return &Source{Alias: alias, URL: target, Header: d.ExtraHeaders[alias]}
}

// define registers an already-parsed datasource URL, bypassing a string round trip.
func (d *Data) define(alias string, target *url.URL, header http.Header) {
	if d.Sources == nil {
		d.Sources = map[string]*Source{}
	}
	d.Sources[alias] = &Source{Alias: alias, URL: target, Header: header}
	// The reader holds the previous definition (and may have cached its
	// content), so the next read must create a new one.
	delete(d.readers, alias)
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

// initReader captures gomplate's datasource reader for alias. A gomplate
// renderer records timings in package-global metrics when it is created and
// when it renders, so both happen under rendererMu. The capture render reads
// nothing; datasource I/O happens later, through the reader, without the lock,
// so a slow datasource cannot block unrelated renders.
func (d *Data) initReader(alias string) error {
	sources := make(map[string]gomplate.DataSource, len(d.Sources))
	for name, source := range d.Sources {
		if source == nil || source.URL == nil {
			return fmt.Errorf("%w: datasource %q has no URL", errInvalidDatasource, name)
		}
		sources[name] = gomplate.DataSource{URL: source.URL, Header: source.Header}
	}
	var captured any
	funcs := template.FuncMap{"capture": func(value any) string { captured = value; return "" }}

	if err := captureReader(d.context(), sources, d.ExtraHeaders, funcs); err != nil {
		return err
	}
	reader, ok := captured.(gomplateReader)
	if !ok {
		return fmt.Errorf("%w: gomplate datasource reader is unavailable", errInvalidDatasource)
	}
	if d.readers == nil {
		d.readers = make(map[string]gomplateReader)
	}
	d.readers[alias] = reader
	return nil
}

// captureReader creates a gomplate renderer and renders a template that only
// hands its `_datasource` namespace to the capture function in funcs.
func captureReader(ctx context.Context, sources map[string]gomplate.DataSource, headers map[string]http.Header, funcs template.FuncMap) error {
	rendererMu.Lock()
	defer rendererMu.Unlock()
	renderer := gomplate.NewRenderer(gomplate.RenderOptions{Datasources: sources, ExtraHeaders: headers, Funcs: funcs})
	return renderer.Render(ctx, "datasource", "{{ capture (_datasource) }}", io.Discard)
}

// bindContext binds reads to a render's context. Readers hold the context they
// were created with, so binding drops them; each render's reads then honor its
// own deadline.
func (d *Data) bindContext(ctx context.Context) {
	d.Ctx = ctx
	d.readers = nil
}

// context is the context reads are bound to.
func (d *Data) context() context.Context {
	if d.Ctx == nil {
		return context.Background()
	}
	return d.Ctx
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
	if d.readers[alias] == nil {
		if err := d.initReader(alias); err != nil {
			return nil, err
		}
	}
	if function == "include" {
		return d.readers[alias].Include(alias, args...)
	}
	return d.readers[alias].Datasource(alias, args...)
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
	sources.bindContext(ctx)
	funcs := gomplateFuncs(ctx)
	if runtime.GOOS == "windows" {
		funcs["file"] = func() any { return &nativeFileFuncs{} }
	}
	for name, fn := range datasourceFuncs(sources) {
		funcs[name] = fn
	}
	return funcs
}

// JSON uses Gomplate's native JSON and EJSON decoder.
func JSON(value string) (map[string]any, error) {
	defer perf.Track(nil, "render.JSON")()

	return sharedFuncMap(false)["json"].(func(any) (map[string]any, error))(value)
}

// CSV uses Gomplate's native CSV decoder.
func CSV(value string) ([][]string, error) {
	defer perf.Track(nil, "render.CSV")()

	return sharedFuncMap(false)["csv"].(func(...string) ([][]string, error))(value)
}

// TOML uses Gomplate's native TOML decoder.
func TOML(value string) (any, error) {
	defer perf.Track(nil, "render.TOML")()

	return sharedFuncMap(false)["toml"].(func(any) (any, error))(value)
}
