package compatibility_test

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/template/compatibility"
	"github.com/cloudposse/atmos/pkg/template/render"
)

// errNoDatasource is returned by the stub datasource functions; lint tests
// only parse templates, so the stubs are never meant to run.
var errNoDatasource = errors.New("stub datasource")

// parseForLint parses text with stub ds, datasource and include functions,
// so a template calling them parses. Lint tests call LintDeprecatedUsage
// directly against the result, never Render, so the datasource cases below
// never touch the network.
func parseForLint(t *testing.T, text string) *template.Template {
	t.Helper()
	stub := func(string, ...string) (any, error) { return nil, errNoDatasource }
	tmpl, err := template.New("t").Funcs(template.FuncMap{"ds": stub, "datasource": stub, "include": stub}).Parse(text)
	require.NoError(t, err)
	return tmpl
}

// sourceURLs parses each datasource URL by alias, as the render engine does
// before linting.
func sourceURLs(t *testing.T, definitions map[string]string) map[string]*url.URL {
	t.Helper()
	sources := make(map[string]*url.URL, len(definitions))
	for alias, raw := range definitions {
		parsed, err := url.Parse(raw)
		require.NoError(t, err)
		sources[alias] = parsed
	}
	return sources
}

func TestLintDeprecatedUsage(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		datasources    map[string]string
		wantDeprecated string
		wantCount      int
	}{
		{
			name:           "dot value on an aws+smp datasource warns",
			text:           `{{ (ds "secret").Value }}`,
			datasources:    map[string]string{"secret": "aws+smp:///demo/param"},
			wantDeprecated: `ds "secret" .Value`,
			wantCount:      1,
		},
		{
			name:           "dot value via the datasource alias also warns",
			text:           `{{ (datasource "secret").Value }}`,
			datasources:    map[string]string{"secret": "aws+smp:///demo/param"},
			wantDeprecated: `datasource "secret" .Value`,
			wantCount:      1,
		},
		{
			name:        "dot value on a non-smp datasource is fine",
			text:        `{{ (ds "cfg").Value }}`,
			datasources: map[string]string{"cfg": "file:///tmp/cfg.yaml"},
			wantCount:   0,
		},
		{
			name:        "dot value on an unconfigured alias is skipped",
			text:        `{{ (ds "unknown").Value }}`,
			datasources: nil,
			wantCount:   0,
		},
		{
			name:        "dot value on a dynamic alias is skipped",
			text:        `{{ (ds .alias).Value }}`,
			datasources: map[string]string{"secret": "aws+smp:///demo/param"},
			wantCount:   0,
		},
		{
			name:           "sub-path without a trailing slash warns",
			text:           `{{ ds "dir" "a.yaml" }}`,
			datasources:    map[string]string{"dir": "file:///tmp/data"},
			wantDeprecated: `ds "dir" <sub-path>`,
			wantCount:      1,
		},
		{
			name:        "sub-path with a trailing slash is fine",
			text:        `{{ ds "dir" "a.yaml" }}`,
			datasources: map[string]string{"dir": "file:///tmp/data/"},
			wantCount:   0,
		},
		{
			name:        "no sub-path argument is fine",
			text:        `{{ ds "dir" }}`,
			datasources: map[string]string{"dir": "file:///tmp/data"},
			wantCount:   0,
		},
		{
			name:           "sub-path supplied through a pipe warns",
			text:           `{{ "a.yaml" | ds "dir" }}`,
			datasources:    map[string]string{"dir": "file:///tmp/data"},
			wantDeprecated: `ds "dir" <sub-path>`,
			wantCount:      1,
		},
		{
			name:        "directory URL with a query string is fine",
			text:        `{{ ds "dir" "a.yaml" }}`,
			datasources: map[string]string{"dir": "file:///tmp/data/?type=json"},
			wantCount:   0,
		},
		{
			name:           "non-directory URL whose query string ends in a slash warns",
			text:           `{{ ds "dir" "a.yaml" }}`,
			datasources:    map[string]string{"dir": "file:///tmp/data?redirect=/"},
			wantDeprecated: `ds "dir" <sub-path>`,
			wantCount:      1,
		},
		{
			name:           "boltdb datasource scheme warns even if unused in the template",
			text:           `hello`,
			datasources:    map[string]string{"kv": "boltdb:///tmp/db.bolt"},
			wantDeprecated: "boltdb:// datasource (kv)",
			wantCount:      1,
		},
		{
			name:        "unrelated datasource scheme is fine",
			text:        `hello`,
			datasources: map[string]string{"kv": "file:///tmp/db.yaml"},
			wantCount:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := parseForLint(t, tt.text)
			fake := &fakeReporter{}
			compatibility.LintDeprecatedUsage(parsed, "t", sourceURLs(t, tt.datasources), fake)

			require.Len(t, fake.calls, tt.wantCount)
			if tt.wantCount > 0 {
				assert.Equal(t, "t", fake.calls[0].templateName)
				assert.Equal(t, tt.wantDeprecated, fake.calls[0].deprecated)
			}
		})
	}
}

// TestLintDeprecatedUsage_RunsDuringRender verifies the lint pass is wired
// into Render itself (not just callable directly): a real render of a
// template with a sub-path datasource call missing a trailing slash warns,
// independent of whether gomplate itself can resolve that sub-path.
func TestLintDeprecatedUsage_RunsDuringRender(t *testing.T) {
	file := writeFixture(t, "a.yaml", "name: atmos\n")
	fake := &fakeReporter{}

	_, _ = render.New(render.WithDeprecationReporter(fake)).Render(context.Background(), &render.Request{
		Name:        "t",
		Text:        `{{ include "dir" "a.yaml" }}`,
		Datasources: map[string]render.Datasource{"dir": {URL: file}},
	})

	require.Len(t, fake.calls, 1)
	assert.Equal(t, `include "dir" <sub-path>`, fake.calls[0].deprecated)
}

// stubAtmosNamespace stands in for Atmos's `atmos` template namespace, which
// exposes GomplateDatasource; the lint only inspects the parse tree, so the
// method is never meant to run.
type stubAtmosNamespace struct{}

func (stubAtmosNamespace) GomplateDatasource(string, ...string) (any, error) {
	return nil, errNoDatasource
}

// TestLintDeprecatedUsage_AtmosGomplateDatasourceValue verifies the
// `.Value`-on-aws+smp finding also fires for the `atmos.GomplateDatasource`
// method-chain form, not just the bare `ds`/`datasource` functions:
// datasourceCallName resolves that chain through its own *parse.ChainNode
// case. The render fails on the stub without any AWS access, but the lint
// warning fires before execution.
func TestLintDeprecatedUsage_AtmosGomplateDatasourceValue(t *testing.T) {
	funcs := template.FuncMap{"atmos": func() any { return stubAtmosNamespace{} }}
	fake := &fakeReporter{}

	_, _ = render.New(render.WithDeprecationReporter(fake)).Render(context.Background(), &render.Request{
		Name:        "t",
		Text:        `{{ (atmos.GomplateDatasource "secret").Value }}`,
		Funcs:       funcs,
		Datasources: map[string]render.Datasource{"secret": {URL: "aws+smp:///demo/param"}},
	})

	require.Len(t, fake.calls, 1)
	assert.Equal(t, `atmos.GomplateDatasource "secret" .Value`, fake.calls[0].deprecated)
}

// TestLintDeprecatedUsage_ValueWithNestedField verifies the `.Value` finding
// still fires when more fields follow it, e.g. `(ds "secret").Value.host`,
// which is how a JSON parameter was read under gomplate v3.
func TestLintDeprecatedUsage_ValueWithNestedField(t *testing.T) {
	fake := &fakeReporter{}
	parsed := parseForLint(t, `{{ (ds "secret").Value.host }}`)

	compatibility.LintDeprecatedUsage(parsed, "t", sourceURLs(t, map[string]string{"secret": "aws+smp:///app/db"}), fake)

	require.Len(t, fake.calls, 1)
	assert.Equal(t, `ds "secret" .Value`, fake.calls[0].deprecated)
}
