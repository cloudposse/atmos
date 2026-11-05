package template

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deferTestFuncs declares the `atmos` namespace and a plain function so templates using them parse.
func deferTestFuncs() template.FuncMap {
	return template.FuncMap{
		"atmos": func() any { return nil },
		"upper": func(s string) string { return s },
	}
}

func TestDeferCalls(t *testing.T) {
	data := map[string]any{
		"locals": map[string]any{
			"str":   "prod",
			"num":   7,
			"flt":   1.5,
			"flag":  true,
			"map":   map[string]any{"k": "v"},
			"quote": `a"b`,
		},
		"vars": map[string]any{"stage": "dev"},
	}

	tests := []struct {
		name      string
		input     string
		roots     []string
		want      string
		wantCount int
	}{
		{
			name:      "no call is untouched",
			input:     "a: {{ .locals.str }}",
			roots:     []string{"locals"},
			want:      "a: prod",
			wantCount: 0,
		},
		{
			name:      "plain action is emitted as template text",
			input:     `a: {{ (atmos.Component "x" "y").vars.z }}`,
			roots:     []string{"locals"},
			want:      `a: {{(atmos.Component "x" "y").vars.z}}`,
			wantCount: 1,
		},
		{
			name:      "bare call without chained fields is deferred",
			input:     `{{ atmos.Component "x" "y" }}`,
			want:      `{{atmos.Component "x" "y"}}`,
			wantCount: 1,
		},
		{
			name:      "trim markers do not leak into the emitted text",
			input:     "a   {{- atmos.Component \"x\" \"y\" -}}   b",
			want:      `a{{atmos.Component "x" "y"}}b`,
			wantCount: 1,
		},
		{
			name:      "scalar locals roots are resolved",
			input:     `{{ (atmos.Component .locals.str .locals.num).a }} {{ (atmos.Component .locals.flt .locals.flag).a }}`,
			roots:     []string{"locals"},
			want:      `{{(atmos.Component "prod" 7).a}} {{(atmos.Component 1.5 true).a}}`,
			wantCount: 2,
		},
		{
			name:      "string values are quoted safely",
			input:     `{{ (atmos.Component .locals.quote "y").a }}`,
			roots:     []string{"locals"},
			want:      `{{(atmos.Component "a\"b" "y").a}}`,
			wantCount: 1,
		},
		{
			name:      "non-root and non-scalar references are left as is",
			input:     `{{ (atmos.Component .vars.stage .locals.map).a }} {{ (atmos.Component .locals.missing "y").a }}`,
			roots:     []string{"locals"},
			want:      `{{(atmos.Component .vars.stage .locals.map).a}} {{(atmos.Component .locals.missing "y").a}}`,
			wantCount: 2,
		},
		{
			name:      "references in nested pipelines are resolved",
			input:     `{{ (atmos.Component (upper .locals.str) "y").a }}`,
			roots:     []string{"locals"},
			want:      `{{(atmos.Component (upper "prod") "y").a}}`,
			wantCount: 1,
		},
		{
			name:      "actions in all branches of a control structure are deferred",
			input:     `{{ if .vars.stage }}{{ atmos.Component "x" "y" }}{{ else }}{{ atmos.Component "p" "q" }}{{ end }}`,
			want:      `{{atmos.Component "x" "y"}}`,
			wantCount: 2,
		},
		{
			name:      "range and with bodies are deferred",
			input:     `{{ range $i, $e := .vars }}{{ atmos.Component "x" "y" }}{{ end }}{{ with .vars }}{{ atmos.Component "p" "q" }}{{ end }}`,
			want:      `{{atmos.Component "x" "y"}}{{atmos.Component "p" "q"}}`,
			wantCount: 2,
		},
		{
			name:      "a different namespace function is not deferred",
			input:     `{{ upper "x" }}`,
			want:      "x",
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := template.New("t").Funcs(deferTestFuncs()).Parse(tt.input)
			require.NoError(t, err)

			count := DeferCalls(tmpl.Tree, "atmos", "Component", data, tt.roots...)

			assert.Equal(t, tt.wantCount, count)
			var out bytes.Buffer
			require.NoError(t, tmpl.Execute(&out, data))
			assert.Equal(t, tt.want, out.String())
		})
	}
}

// TestDeferCalls_NotDeferrable verifies the forms that must stay in the template so that executing
// them still reaches the (guarded) function instead of silently emitting half of a construct.
func TestDeferCalls_NotDeferrable(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "control structure condition", input: `{{ if atmos.Component "x" "y" }}a{{ end }}`},
		{name: "range pipeline", input: `{{ range atmos.Component "x" "y" }}a{{ end }}`},
		{name: "with pipeline", input: `{{ with atmos.Component "x" "y" }}a{{ end }}`},
		{name: "variable declaration", input: `{{ $c := atmos.Component "x" "y" }}{{ $c }}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := template.New("t").Funcs(deferTestFuncs()).Parse(tt.input)
			require.NoError(t, err)

			assert.Zero(t, DeferCalls(tmpl.Tree, "atmos", "Component", map[string]any{}))
		})
	}
}

func TestDeferCalls_NilTree(t *testing.T) {
	assert.Zero(t, DeferCalls(nil, "atmos", "Component", nil))
}
