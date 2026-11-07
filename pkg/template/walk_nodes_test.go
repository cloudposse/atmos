package template

import (
	"testing"
	"text/template"
	"text/template/parse"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWalkNodes verifies the exported traversal helper visits every node in
// the main template, in templates declared with `define`, and inside chained
// expressions.
func TestWalkNodes(t *testing.T) {
	tests := []struct {
		name         string
		text         string
		wantCommands []string
	}{
		{
			name:         "nil template",
			text:         "",
			wantCommands: nil,
		},
		{
			name:         "plain text has no commands",
			text:         "hello",
			wantCommands: nil,
		},
		{
			name:         "single call",
			text:         `{{ upper .name }}`,
			wantCommands: []string{"upper"},
		},
		{
			name:         "call inside define is visited",
			text:         `{{ define "sub" }}{{ ds "cfg" }}{{ end }}{{ template "sub" . }}`,
			wantCommands: []string{"ds"},
		},
		{
			name:         "call inside a chain is visited",
			text:         `{{ (ds "cfg").name }}`,
			wantCommands: []string{"ds"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var tmpl *template.Template
			if tt.name != "nil template" {
				parsed, err := template.New("t").Funcs(template.FuncMap{
					"upper": func(s string) string { return s },
					"ds":    func(...any) (any, error) { return nil, nil },
				}).Parse(tt.text)
				require.NoError(t, err)
				tmpl = parsed
			}

			var commands []string
			WalkNodes(tmpl, func(node parse.Node) {
				ident, ok := node.(*parse.IdentifierNode)
				if !ok {
					return
				}
				commands = append(commands, ident.Ident)
			})

			assert.Equal(t, tt.wantCommands, commands)
		})
	}
}

// Parse trees hold typed-nil pointers for absent parts (the else list of an
// `if` without `else`, the pipe of `{{ template "x" }}`). WalkNodes must skip
// them, so a callback can use every node it receives.
func TestWalkNodesSkipsTypedNilNodes(t *testing.T) {
	tmpl, err := template.New("t").Parse(`{{ define "x" }}x{{ end }}{{ if .a }}a{{ end }}{{ range .b }}b{{ end }}{{ with .c }}c{{ end }}{{ template "x" }}`)
	require.NoError(t, err)

	visited := 0
	require.NotPanics(t, func() {
		WalkNodes(tmpl, func(n parse.Node) {
			visited++
			_ = n.String()
		})
	})
	assert.Positive(t, visited)
}
