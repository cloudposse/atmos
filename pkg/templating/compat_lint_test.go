package templating

import (
	"context"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseForLint parses text with the same stub functions Render's fast path
// uses for renderer-only functions (ds, datasource, include, ...), so a
// template calling them parses without needing a real gomplate render. Lint
// tests call lintDeprecatedUsage directly against the result, never Render,
// so the datasource cases below never touch the network.
func parseForLint(t *testing.T, text string) *template.Template {
	t.Helper()
	tmpl, err := template.New("t").Funcs(stubFuncs()).Parse(text)
	require.NoError(t, err)
	return tmpl
}

func TestLintDeprecatedUsage(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		datasources    map[string]Datasource
		wantDeprecated string
		wantCount      int
	}{
		{
			name:           "dot value on an aws+smp datasource warns",
			text:           `{{ (ds "secret").Value }}`,
			datasources:    map[string]Datasource{"secret": {URL: "aws+smp:///demo/param"}},
			wantDeprecated: `ds "secret" .Value`,
			wantCount:      1,
		},
		{
			name:           "dot value via the datasource alias also warns",
			text:           `{{ (datasource "secret").Value }}`,
			datasources:    map[string]Datasource{"secret": {URL: "aws+smp:///demo/param"}},
			wantDeprecated: `datasource "secret" .Value`,
			wantCount:      1,
		},
		{
			name:        "dot value on a non-smp datasource is fine",
			text:        `{{ (ds "cfg").Value }}`,
			datasources: map[string]Datasource{"cfg": {URL: "file:///tmp/cfg.yaml"}},
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
			datasources: map[string]Datasource{"secret": {URL: "aws+smp:///demo/param"}},
			wantCount:   0,
		},
		{
			name:           "sub-path without a trailing slash warns",
			text:           `{{ ds "dir" "a.yaml" }}`,
			datasources:    map[string]Datasource{"dir": {URL: "file:///tmp/data"}},
			wantDeprecated: `ds "dir" <sub-path>`,
			wantCount:      1,
		},
		{
			name:        "sub-path with a trailing slash is fine",
			text:        `{{ ds "dir" "a.yaml" }}`,
			datasources: map[string]Datasource{"dir": {URL: "file:///tmp/data/"}},
			wantCount:   0,
		},
		{
			name:        "no sub-path argument is fine",
			text:        `{{ ds "dir" }}`,
			datasources: map[string]Datasource{"dir": {URL: "file:///tmp/data"}},
			wantCount:   0,
		},
		{
			name:           "sub-path supplied through a pipe warns",
			text:           `{{ "a.yaml" | ds "dir" }}`,
			datasources:    map[string]Datasource{"dir": {URL: "file:///tmp/data"}},
			wantDeprecated: `ds "dir" <sub-path>`,
			wantCount:      1,
		},
		{
			name:        "directory URL with a query string is fine",
			text:        `{{ ds "dir" "a.yaml" }}`,
			datasources: map[string]Datasource{"dir": {URL: "file:///tmp/data/?type=json"}},
			wantCount:   0,
		},
		{
			name:           "non-directory URL whose query string ends in a slash warns",
			text:           `{{ ds "dir" "a.yaml" }}`,
			datasources:    map[string]Datasource{"dir": {URL: "file:///tmp/data?redirect=/"}},
			wantDeprecated: `ds "dir" <sub-path>`,
			wantCount:      1,
		},
		{
			name:           "boltdb datasource scheme warns even if unused in the template",
			text:           `hello`,
			datasources:    map[string]Datasource{"kv": {URL: "boltdb:///tmp/db.bolt"}},
			wantDeprecated: "boltdb:// datasource (kv)",
			wantCount:      1,
		},
		{
			name:        "unrelated datasource scheme is fine",
			text:        `hello`,
			datasources: map[string]Datasource{"kv": {URL: "file:///tmp/db.yaml"}},
			wantCount:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := parseForLint(t, tt.text)
			fake := &fakeReporter{}
			plan := &renderPlan{req: &Request{Name: "t", Datasources: tt.datasources}}

			lintDeprecatedUsage(parsed, plan, fake)

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

	_, _ = New(WithDeprecationReporter(fake)).Render(context.Background(), &Request{
		Name:        "t",
		Text:        `{{ include "dir" "a.yaml" }}`,
		Datasources: map[string]Datasource{"dir": {URL: file}},
	})

	require.Len(t, fake.calls, 1)
	assert.Equal(t, `include "dir" <sub-path>`, fake.calls[0].deprecated)
}

// TestLintDeprecatedUsage_AtmosGomplateDatasourceValue verifies the
// `.Value`-on-aws+smp finding also fires for the `atmos.GomplateDatasource`
// method-chain form, not just the bare `ds`/`datasource` functions:
// datasourceCallName resolves that chain through its own *parse.ChainNode
// case, which parseForLint's stub function map can't exercise (it has no
// "atmos" stub), so this test wires a real "atmos" func and renders instead.
// The render itself is expected to fail (no live AWS SSM access in tests),
// but the lint warning fires before that, during parsing.
func TestLintDeprecatedUsage_AtmosGomplateDatasourceValue(t *testing.T) {
	e := New()
	funcs := template.FuncMap{"atmos": func() any { return atmosNamespace{reader: e} }}
	fake := &fakeReporter{}

	_, _ = New(WithDeprecationReporter(fake)).Render(context.Background(), &Request{
		Name:        "t",
		Text:        `{{ (atmos.GomplateDatasource "secret").Value }}`,
		Funcs:       funcs,
		Datasources: map[string]Datasource{"secret": {URL: "aws+smp:///demo/param"}},
	})

	require.Len(t, fake.calls, 1)
	assert.Equal(t, `atmos.GomplateDatasource "secret" .Value`, fake.calls[0].deprecated)
}

// TestLintDeprecatedUsage_ValueWithNestedField verifies the `.Value` finding
// still fires when more fields follow it, e.g. `(ds "secret").Value.host`,
// which is how a JSON parameter was read under gomplate v3.
func TestLintDeprecatedUsage_ValueWithNestedField(t *testing.T) {
	fake := &fakeReporter{}
	e := New(WithDeprecationReporter(fake), WithDatasourceCache(NewDatasourceCache()))

	parsed, err := parsePlain(&renderPlan{
		req: &Request{
			Name:        "t",
			Text:        `{{ (ds "secret").Value.host }}`,
			Datasources: map[string]Datasource{"secret": {URL: "aws+smp:///app/db"}},
		},
		funcs:      e.(*engine).baseFuncs(context.Background(), "t", nil),
		left:       DefaultLeftDelim,
		right:      DefaultRightDelim,
		missingKey: MissingKeyError,
	}, true)
	require.NoError(t, err)

	lintDeprecatedUsage(parsed, &renderPlan{req: &Request{
		Name:        "t",
		Datasources: map[string]Datasource{"secret": {URL: "aws+smp:///app/db"}},
	}}, fake)

	require.Len(t, fake.calls, 1)
	assert.Equal(t, `ds "secret" .Value`, fake.calls[0].deprecated)
}
