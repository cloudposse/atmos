package template

import (
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsesFunctions(t *testing.T) {
	funcNames := map[string]struct{}{"ds": {}, "datasource": {}, "include": {}}
	methods := map[string]struct{}{"atmos.GomplateDatasource": {}}

	stub := func(...any) (any, error) { return nil, nil }
	funcs := template.FuncMap{
		"ds":         stub,
		"datasource": stub,
		"include":    stub,
		"upper":      func(s string) string { return s },
		"atmos":      func() any { return struct{}{} },
	}

	tests := []struct {
		name     string
		template string
		expected bool
	}{
		{name: "plain text", template: "hello", expected: false},
		{name: "field only", template: "{{ .name }}", expected: false},
		{name: "unrelated function", template: `{{ upper .name }}`, expected: false},
		{name: "direct call", template: `{{ ds "cfg" }}`, expected: true},
		{name: "parenthesized chain", template: `{{ (ds "cfg").name }}`, expected: true},
		{name: "nested chain in pipe", template: `{{ (datasource "cfg").nested.key | upper }}`, expected: true},
		{name: "inside if", template: `{{ if .x }}{{ include "cfg" }}{{ end }}`, expected: true},
		{name: "inside range", template: `{{ range .items }}{{ ds "cfg" }}{{ end }}`, expected: true},
		{name: "inside with", template: `{{ with .x }}{{ ds "cfg" }}{{ end }}`, expected: true},
		{name: "inside define", template: `{{ define "sub" }}{{ ds "cfg" }}{{ end }}{{ template "sub" . }}`, expected: true},
		{name: "method chain", template: `{{ (atmos.GomplateDatasource "cfg").name }}`, expected: true},
		{name: "method chain without parens", template: `{{ atmos.GomplateDatasource "cfg" }}`, expected: true},
		{name: "other method on same namespace", template: `{{ atmos.Other "x" }}`, expected: false},
		{name: "variable declaration", template: `{{ $v := ds "cfg" }}{{ $v }}`, expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, err := template.New("t").Funcs(funcs).Parse(tt.template)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, UsesFunctions(parsed, funcNames, methods))
		})
	}

	t.Run("nil template", func(t *testing.T) {
		assert.False(t, UsesFunctions(nil, funcNames, methods))
	})
}

func TestExtractFieldRefs_ParenthesizedChain(t *testing.T) {
	// Field references inside a parenthesized expression are reached through
	// a ChainNode and must still be extracted.
	refs, err := ExtractFieldRefs(`{{ (index .locals.map "k").value }} {{ (.locals.other).x }}`)
	require.NoError(t, err)

	var paths []string
	for _, ref := range refs {
		paths = append(paths, ref.String())
	}
	assert.Contains(t, paths, "locals.map")
	assert.Contains(t, paths, "locals.other")
}
