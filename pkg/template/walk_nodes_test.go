package template

import (
	"testing"
	"text/template"
	"text/template/parse"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWalkNodes verifies the exported traversal helper visits every node in
// the main template and in templates declared with `define`, matching the
// coverage UsesFunctions relies on internally.
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
