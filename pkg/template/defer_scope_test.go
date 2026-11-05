package template

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type scopedComponentLookup struct{}

func (scopedComponentLookup) Component(name, stack string) string { return name + ":" + stack }

func TestDeferCallsPreservesExecutionScope(t *testing.T) {
	data := map[string]any{
		"locals": map[string]any{"str": "root"},
		"items":  []any{map[string]any{"locals": map[string]any{"str": "first"}}, map[string]any{"locals": map[string]any{"str": "second"}}},
	}
	for _, tt := range []struct{ name, input, want string }{
		{"variable", `{{ $name := .locals.str }}{{ atmos.Component $name "dev" }}`, "root:dev"},
		{"nested variable", `{{ $name := .locals.str }}{{ atmos.Component (printf "%s" $name) "dev" }}`, "root:dev"},
		{"root variable", `{{ atmos.Component $.locals.str "dev" }}`, "root:dev"},
		{"range dot", `{{ range .items }}{{ atmos.Component .locals.str "dev" }};{{ end }}`, "first:dev;second:dev;"},
		{"nested if", `{{ range .items }}{{ if .locals }}{{ atmos.Component .locals.str "dev" }};{{ end }}{{ end }}`, "first:dev;second:dev;"},
		{"with dot", `{{ with index .items 0 }}{{ atmos.Component .locals.str "dev" }}{{ end }}`, "first:dev"},
		{"dot argument", `{{ with .locals.str }}{{ atmos.Component . "dev" }}{{ end }}`, "root:dev"},
		{"nested pipeline", `{{ range .items }}{{ atmos.Component (printf "%s" .locals.str) "dev" }};{{ end }}`, "first:dev;second:dev;"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tmpl, err := template.New("scope").Funcs(template.FuncMap{"atmos": func() any { return scopedComponentLookup{} }}).Parse(tt.input)
			require.NoError(t, err)
			assert.Zero(t, DeferCalls(tmpl.Tree, "atmos", "Component", data, "locals"))
			var out bytes.Buffer
			require.NoError(t, tmpl.Execute(&out, data))
			assert.Equal(t, tt.want, out.String())
		})
	}
}

func TestDeferCallsElseKeepsOuterDot(t *testing.T) {
	for _, input := range []string{
		`{{ range .empty }}unused{{ else }}{{ atmos.Component .locals.str "dev" }}{{ end }}`,
		`{{ with .empty }}unused{{ else }}{{ atmos.Component .locals.str "dev" }}{{ end }}`,
	} {
		tmpl, err := template.New("else").Funcs(deferTestFuncs()).Parse(input)
		require.NoError(t, err)
		data := map[string]any{"locals": map[string]any{"str": "root"}}
		assert.Equal(t, 1, DeferCalls(tmpl.Tree, "atmos", "Component", data, "locals"))
		var out bytes.Buffer
		require.NoError(t, tmpl.Execute(&out, data))
		assert.Equal(t, `{{atmos.Component "root" "dev"}}`, out.String())
	}
}
