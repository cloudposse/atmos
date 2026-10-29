// Package gomplate adapts Gomplate's public rendering API to Atmos's datasource functions.
package gomplate

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

	gomplate "github.com/hairyhenderson/gomplate/v4"
	"github.com/hairyhenderson/yaml"

	"github.com/cloudposse/atmos/pkg/perf"
)

var errInvalidDatasource = errors.New("invalid datasource")

// Source retains the URL and headers so policy admission can inspect a datasource before reading it.
type Source struct {
	Alias  string
	URL    *url.URL
	Header http.Header
}

// Data holds the datasources belonging to a single template evaluation.
// Like Gomplate Renderer, a Data value must not be used concurrently.
type Data struct {
	Ctx          context.Context
	Sources      map[string]*Source
	ExtraHeaders map[string]http.Header

	renderers map[string]gomplate.Renderer
	alias     string
	args      []string
	result    any
}

// NewData builds datasources from alias=URL and alias=Header: value definitions.
func NewData(sources, headers []string) (*Data, error) {
	defer perf.Track(nil, "gomplate.NewData")()

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
	defer perf.Track(nil, "gomplate.DefineDatasource")()

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
	if d.Sources == nil {
		d.Sources = map[string]*Source{}
	}
	d.Sources[alias] = &Source{Alias: alias, URL: source.URL, Header: d.ExtraHeaders[alias]}
	return "", nil
}

// DatasourceExists reports whether the alias has been registered.
func (d *Data) DatasourceExists(alias string) bool {
	defer perf.Track(nil, "gomplate.DatasourceExists")()

	_, exists := d.Sources[alias]
	return exists
}

// ListDatasources returns the registered aliases in stable order.
func (d *Data) ListDatasources() []string {
	defer perf.Track(nil, "gomplate.ListDatasources")()

	aliases := make([]string, 0, len(d.Sources))
	for alias := range d.Sources {
		aliases = append(aliases, alias)
	}
	sort.Strings(aliases)
	return aliases
}

// Datasource reads and parses a datasource without serializing its native return value.
func (d *Data) Datasource(alias string, args ...string) (any, error) {
	defer perf.Track(nil, "gomplate.Datasource")()

	return d.read("datasource", alias, args)
}

// Include reads a datasource without parsing its contents.
func (d *Data) Include(alias string, args ...string) (string, error) {
	defer perf.Track(nil, "gomplate.Include")()

	value, err := d.read("include", alias, args)
	if err != nil {
		return "", err
	}
	return value.(string), nil
}

// DatasourceReachable preserves Gomplate's registered-alias probe behavior.
func (d *Data) DatasourceReachable(alias string, args ...string) bool {
	defer perf.Track(nil, "gomplate.DatasourceReachable")()

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
	if err := d.renderers[alias].Render(ctx, "datasource", text, io.Discard); err != nil {
		return nil, err
	}
	return d.result, nil
}

// CreateFuncs adds datasource functions to Gomplate's public namespace map.
func CreateFuncs(ctx context.Context, d *Data) template.FuncMap {
	defer perf.Track(nil, "gomplate.CreateFuncs")()

	d.Ctx = ctx
	funcs := gomplate.CreateFuncs(ctx)
	if runtime.GOOS == "windows" {
		funcs["file"] = func() any { return &nativeFileFuncs{} }
	}
	funcs["datasource"] = d.Datasource
	funcs["ds"] = d.Datasource
	funcs["include"] = d.Include
	funcs["datasourceExists"] = d.DatasourceExists
	funcs["datasourceReachable"] = d.DatasourceReachable
	funcs["defineDatasource"] = d.DefineDatasource
	funcs["listDatasources"] = d.ListDatasources
	return funcs
}

// JSON uses Gomplate's native JSON and EJSON decoder.
func JSON(value string) (map[string]any, error) {
	defer perf.Track(nil, "gomplate.JSON")()

	return gomplate.CreateFuncs(context.Background())["json"].(func(any) (map[string]any, error))(value)
}

// CSV uses Gomplate's native CSV decoder.
func CSV(value string) ([][]string, error) {
	defer perf.Track(nil, "gomplate.CSV")()

	return gomplate.CreateFuncs(context.Background())["csv"].(func(...string) ([][]string, error))(value)
}

// TOML uses Gomplate's native TOML decoder.
func TOML(value string) (any, error) {
	defer perf.Track(nil, "gomplate.TOML")()

	return gomplate.CreateFuncs(context.Background())["toml"].(func(any) (any, error))(value)
}
