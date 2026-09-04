package templating

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"text/template"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// writeFixture writes a datasource fixture and returns its absolute path.
func writeFixture(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

type person struct {
	Name string
	Tags []string
}

func TestRender_FastPath(t *testing.T) {
	tests := []struct {
		name     string
		req      Request
		opts     []Option
		expected string
		wantErr  string
	}{
		{
			name:     "map data with sprig and gomplate funcs",
			req:      Request{Name: "t", Text: `{{ .name | upper }}-{{ strings.Title .name }}`, Data: map[string]any{"name": "vpc"}},
			expected: "VPC-Vpc",
		},
		{
			name:     "struct data",
			req:      Request{Name: "t", Text: `{{ .Name }}:{{ len .Tags }}`, Data: person{Name: "eks", Tags: []string{"a", "b"}}},
			expected: "eks:2",
		},
		{
			name:     "ints stay ints",
			req:      Request{Name: "t", Text: `{{ .count }}`, Data: map[string]any{"count": 1000000}},
			expected: "1000000",
		},
		{
			name:     "custom delimiters",
			req:      Request{Name: "t", Text: `[[ .name ]] {{ .name }}`, Data: map[string]any{"name": "x"}, LeftDelim: "[[", RightDelim: "]]"},
			expected: "x {{ .name }}",
		},
		{
			name:     "caller funcs override sprig and gomplate",
			req:      Request{Name: "t", Text: `{{ upper "a" }}`, Data: nil, Funcs: template.FuncMap{"upper": func(string) string { return "mine" }}},
			expected: "mine",
		},
		{
			name:    "missing key errors by default",
			req:     Request{Name: "cfg.yaml", Text: `{{ .missing }}`, Data: map[string]any{}},
			wantErr: `template: cfg.yaml:1:3: executing "cfg.yaml" at <.missing>: map has no entry for key "missing"`,
		},
		{
			name:     "missing key default",
			req:      Request{Name: "t", Text: `[{{ .missing }}]`, Data: map[string]any{}, MissingKey: MissingKeyDefault},
			expected: "[<no value>]",
		},
		{
			name:    "invalid missingkey option",
			req:     Request{Name: "t", Text: `x`, MissingKey: "bogus"},
			wantErr: "unsupported missingkey option",
		},
		{
			name:    "parse error is returned raw",
			req:     Request{Name: "bad.yaml", Text: `{{ .a `, Data: nil},
			wantErr: "template: bad.yaml:1: unclosed action",
		},
		{
			name:    "gomplate disabled: datasource funcs are undefined",
			req:     Request{Name: "t", Text: `{{ ds "x" }}`},
			opts:    []Option{WithGomplate(false)},
			wantErr: `function "ds" not defined`,
		},
		{
			name:    "sprig disabled: sprig funcs are undefined",
			req:     Request{Name: "t", Text: `{{ upper "a" }}`},
			opts:    []Option{WithSprig(false)},
			wantErr: `function "upper" not defined`,
		},
		{
			name:     "gomplate disabled: sprig still works",
			req:      Request{Name: "t", Text: `{{ upper "a" }}`},
			opts:     []Option{WithGomplate(false)},
			expected: "A",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := New(tt.opts...).Render(context.Background(), &tt.req)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, out)
		})
	}
}

func TestRender_GomplatePath(t *testing.T) {
	cfg := writeFixture(t, "config.yaml", "name: atmos\ncount: 42\nnested:\n  key: value\n")

	tests := []struct {
		name     string
		req      Request
		expected string
		wantErr  string
	}{
		{
			name: "ds with configured datasource and in-memory struct data",
			req: Request{
				Name:        "t",
				Text:        `{{ .Name }}/{{ (ds "cfg").name }}/{{ (datasource "cfg").count }}`,
				Data:        person{Name: "eks"},
				Datasources: map[string]Datasource{"cfg": {URL: cfg}},
			},
			expected: "eks/atmos/42",
		},
		{
			name: "bare path is a file datasource and ints survive",
			req: Request{
				Name:        "t",
				Text:        `{{ (ds "cfg").count }}:{{ .count }}`,
				Data:        map[string]any{"count": 1000000},
				Datasources: map[string]Datasource{"cfg": {URL: cfg}},
			},
			expected: "42:1000000",
		},
		{
			name: "include reads raw content",
			req: Request{
				Name:        "t",
				Text:        `{{ include "cfg" | strings.TrimSpace | len }}`,
				Datasources: map[string]Datasource{"cfg": {URL: "file://" + filepath.ToSlash(cfg)}},
			},
			expected: "42",
		},
		{
			name: "inline defineDatasource without configured datasources",
			req: Request{
				Name: "t",
				Text: `{{ defineDatasource "c" .path }}{{ (ds "c").nested.key }}`,
				Data: map[string]any{"path": cfg},
			},
			expected: "value",
		},
		{
			name: "datasourceExists",
			req: Request{
				Name:        "t",
				Text:        `{{ datasourceExists "cfg" }}/{{ datasourceExists "nope" }}`,
				Datasources: map[string]Datasource{"cfg": {URL: cfg}},
			},
			expected: "true/false",
		},
		{
			name: "custom delimiters on the gomplate path",
			req: Request{
				Name:        "t",
				Text:        `<< (ds "cfg").name >> {{ untouched }}`,
				LeftDelim:   "<<",
				RightDelim:  ">>",
				Datasources: map[string]Datasource{"cfg": {URL: cfg}},
			},
			expected: "atmos {{ untouched }}",
		},
		{
			name: "missing key errors on the gomplate path with the user's template name",
			req: Request{
				Name:        "stack.yaml",
				Text:        "{{ (ds \"cfg\").name }}\n{{ .missing }}",
				Data:        map[string]any{},
				Datasources: map[string]Datasource{"cfg": {URL: cfg}},
			},
			wantErr: `template: stack.yaml:2:3: executing "stack.yaml" at <.missing>: map has no entry for key "missing"`,
		},
		{
			name: "missing key default on the gomplate path",
			req: Request{
				Name:        "t",
				Text:        `{{ (ds "cfg").name }}[{{ .missing }}]`,
				Data:        map[string]any{},
				MissingKey:  MissingKeyDefault,
				Datasources: map[string]Datasource{"cfg": {URL: cfg}},
			},
			expected: "atmos[<no value>]",
		},
		{
			name: "unknown datasource alias",
			req: Request{
				Name: "t",
				Text: `{{ ds "nope" }}`,
			},
			wantErr: "nope",
		},
		{
			name: "invalid datasource URL",
			req: Request{
				Name:        "t",
				Text:        `{{ ds "bad" }}`,
				Datasources: map[string]Datasource{"bad": {URL: "http://[::1"}},
			},
			wantErr: errUtils.ErrInvalidDatasourceURL.Error(),
		},
		{
			name: "empty datasource alias",
			req: Request{
				Name:        "t",
				Text:        `{{ ds "x" }}`,
				Datasources: map[string]Datasource{"": {URL: cfg}},
			},
			wantErr: "alias must not be empty",
		},
		{
			name: "tmpl namespace works",
			req: Request{
				Name: "t",
				Text: `{{ tmpl.Inline "{{ .a }}" (dict "a" "b") }}`,
			},
			expected: "b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := New().Render(context.Background(), &tt.req)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, out)
		})
	}
}

func TestRender_GomplatePath_TimeoutReachesDatasource(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"late"}`))
	}))
	t.Cleanup(func() {
		close(release)
		server.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := New().Render(ctx, &Request{
		Name:        "t",
		Text:        `{{ (ds "api").name }}`,
		Datasources: map[string]Datasource{"api": {URL: server.URL + "/x.json"}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "api")
}

func TestRender_GomplatePath_HTTPHeaders(t *testing.T) {
	var gotAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ip":"203.0.113.7"}`))
	}))
	t.Cleanup(server.Close)

	out, err := New().Render(context.Background(), &Request{
		Name: "t",
		Text: `{{ (ds "ip").ip }}`,
		Datasources: map[string]Datasource{
			"ip": {URL: server.URL + "/", Headers: map[string][]string{"Accept": {"application/json"}}},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "203.0.113.7", out)
	assert.Equal(t, "application/json", gotAccept)
}

func TestRender_ContextSources(t *testing.T) {
	top := writeFixture(t, "top.json", `{"name":"atmos","Env":{"README_YAML":"x"}}`)
	cfg := writeFixture(t, "config.json", `{"name":"inner"}`)

	tests := []struct {
		name     string
		req      Request
		expected string
		wantErr  string
	}{
		{
			name: "dot comes from the '.' context source",
			req: Request{
				Name:           "t",
				Text:           `{{ .name }}/{{ .Env.README_YAML }}/{{ (ds "config").name }}`,
				ContextSources: map[string]Datasource{".": {URL: top}, "config": {URL: cfg}},
			},
			expected: "atmos/x/inner",
		},
		{
			name: "missingkey default honored",
			req: Request{
				Name:           "t",
				Text:           `[{{ .missing }}]`,
				MissingKey:     MissingKeyDefault,
				ContextSources: map[string]Datasource{".": {URL: top}},
			},
			expected: "[<no value>]",
		},
		{
			name: "missingkey error wraps ErrRenderTemplate",
			req: Request{
				Name:           "t",
				Text:           `{{ .missing }}`,
				ContextSources: map[string]Datasource{".": {URL: top}},
			},
			wantErr: errUtils.ErrRenderTemplate.Error(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := New().Render(context.Background(), &tt.req)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, out)
		})
	}
}

// atmosNamespace mimics how internal/exec exposes atmos.GomplateDatasource:
// a namespace object whose method delegates to the engine's DatasourceReader.
type atmosNamespace struct {
	reader DatasourceReader
}

func (n atmosNamespace) GomplateDatasource(alias string, args ...string) (any, error) {
	return n.reader.Datasource(alias, args...)
}

func TestDatasource_ViaTemplateFunction(t *testing.T) {
	cfg := writeFixture(t, "config.yaml", "name: atmos\nlist:\n  - a\n  - b\n")
	dir := filepath.Dir(cfg)

	newEngine := func(cache *DatasourceCache) (Engine, template.FuncMap) {
		e := New(WithDatasourceCache(cache))
		funcs := template.FuncMap{"atmos": func() any { return atmosNamespace{reader: e} }}
		return e, funcs
	}

	t.Run("reads a configured datasource and caches it", func(t *testing.T) {
		cache := NewDatasourceCache()
		e, funcs := newEngine(cache)
		req := Request{
			Name:        "t",
			Text:        `{{ (atmos.GomplateDatasource "cfg").name }}-{{ index (atmos.GomplateDatasource "cfg").list 1 }}`,
			Funcs:       funcs,
			Datasources: map[string]Datasource{"cfg": {URL: cfg}},
		}
		out, err := e.Render(context.Background(), &req)
		require.NoError(t, err)
		assert.Equal(t, "atmos-b", out)

		cached, ok := cache.load(cacheKey("cfg", nil))
		require.True(t, ok)
		assert.Equal(t, "atmos", cached.(map[string]any)["name"])

		// A cached alias is served without a live render.
		value, err := e.Datasource("cfg")
		require.NoError(t, err)
		assert.Equal(t, "atmos", value.(map[string]any)["name"])
	})

	t.Run("args are part of the cache key", func(t *testing.T) {
		cache := NewDatasourceCache()
		e, funcs := newEngine(cache)
		req := Request{
			Name:        "t",
			Text:        `{{ (atmos.GomplateDatasource "dir" "config.yaml").name }}`,
			Funcs:       funcs,
			Datasources: map[string]Datasource{"dir": {URL: filepath.ToSlash(dir) + "/"}},
		}
		out, err := e.Render(context.Background(), &req)
		require.NoError(t, err)
		assert.Equal(t, "atmos", out)

		_, okBare := cache.load(cacheKey("dir", nil))
		assert.False(t, okBare)
		_, okArgs := cache.load(cacheKey("dir", []string{"config.yaml"}))
		assert.True(t, okArgs)
	})

	t.Run("unknown alias errors and is not cached", func(t *testing.T) {
		cache := NewDatasourceCache()
		e, funcs := newEngine(cache)
		_, err := e.Render(context.Background(), &Request{
			Name:  "t",
			Text:  `{{ atmos.GomplateDatasource "nope" }}`,
			Funcs: funcs,
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "nope")
		_, ok := cache.load(cacheKey("nope", nil))
		assert.False(t, ok)
	})

	t.Run("unavailable outside a render", func(t *testing.T) {
		e := New(WithDatasourceCache(NewDatasourceCache()))
		_, err := e.Datasource("cfg")
		require.ErrorIs(t, err, errUtils.ErrGomplateDatasourceUnavailable)
	})

	t.Run("gomplate disabled: atmos.GomplateDatasource is unavailable", func(t *testing.T) {
		e := New(WithGomplate(false), WithDatasourceCache(NewDatasourceCache()))
		funcs := template.FuncMap{"atmos": func() any { return atmosNamespace{reader: e} }}
		_, err := e.Render(context.Background(), &Request{
			Name:  "t",
			Text:  `{{ atmos.GomplateDatasource "cfg" }}`,
			Funcs: funcs,
		})
		require.ErrorIs(t, err, errUtils.ErrGomplateDatasourceUnavailable)
	})
}

func TestRender_Parallel(t *testing.T) {
	cfg := writeFixture(t, "config.yaml", "name: atmos\n")

	var wg sync.WaitGroup
	errs := make(chan error, 64)
	for i := range 32 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			e := New(WithDatasourceCache(NewDatasourceCache()))
			var req Request
			if i%2 == 0 {
				req = Request{Name: "fast", Text: `{{ .n }}`, Data: map[string]any{"n": i}}
			} else {
				req = Request{Name: "slow", Text: `{{ (ds "cfg").name }}`, Datasources: map[string]Datasource{"cfg": {URL: cfg}}}
			}
			if _, err := e.Render(context.Background(), &req); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}

func TestParseDatasourceURL(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		wantScheme  string
		wantPath    string
		wantErr     bool
		windowsOnly bool
	}{
		{name: "stdin dash", in: "-", wantScheme: "stdin", wantPath: ""},
		{name: "https", in: "https://example.com/a.json?x=1", wantScheme: "https", wantPath: "/a.json"},
		{name: "absolute path becomes file", in: "/tmp/a.yaml", wantScheme: "file", wantPath: "/tmp/a.yaml"},
		{name: "relative path keeps no scheme", in: "configs/a.yaml", wantScheme: "", wantPath: "configs/a.yaml"},
		// filepath.VolumeName only recognizes drive letters and UNC prefixes on Windows.
		{name: "windows drive path", in: `C:\data\a.json`, wantScheme: "file", wantPath: "C:/data/a.json", windowsOnly: true},
		{name: "windows UNC path", in: `\\server\share\a.json`, wantScheme: "file", wantPath: "//server/share/a.json", windowsOnly: true},
		{name: "invalid", in: "http://[::1", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.windowsOnly && runtime.GOOS != "windows" {
				t.Skip("Windows volume-name parsing only applies on Windows")
			}
			u, err := parseDatasourceURL(tt.in)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantScheme, u.Scheme)
			assert.Equal(t, tt.wantPath, u.Path)
		})
	}
}

func TestToDataSources_Headers(t *testing.T) {
	out, err := toDataSources(map[string]Datasource{
		"api": {URL: "https://example.com/x.json", Headers: map[string][]string{"Accept": {"application/json"}}},
	})
	require.NoError(t, err)
	require.Contains(t, out, "api")
	assert.Equal(t, []string{"application/json"}, out["api"].Header["Accept"])
	assert.Equal(t, "https", out["api"].URL.Scheme)

	empty, err := toDataSources(nil)
	require.NoError(t, err)
	assert.Nil(t, empty)
}

func TestSprigFuncMap_IsCached(t *testing.T) {
	first := SprigFuncMap()
	second := SprigFuncMap()
	require.NotEmpty(t, first)
	// Both calls must return the same underlying map (identity, not just equality).
	assert.Equal(t, reflect.ValueOf(first).Pointer(), reflect.ValueOf(second).Pointer())
	_, ok := second["upper"]
	assert.True(t, ok)
}

func TestErrorsAreStatic(t *testing.T) {
	_, err := New(WithDatasourceCache(NewDatasourceCache())).Datasource("x")
	assert.True(t, errors.Is(err, errUtils.ErrGomplateDatasourceUnavailable))
}
