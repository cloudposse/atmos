package step

import (
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func upperRenderer(name, input string, data any) (string, error) {
	tmpl, err := template.New(name).Funcs(template.FuncMap{"upper": strings.ToUpper}).Parse(input)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	if err := tmpl.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}

func TestResolveWithDataUsesTheVariablesRendererAndPassCount(t *testing.T) {
	vars := NewVariables()
	vars.SetTemplateRenderer(upperRenderer)
	vars.SetTemplatePasses(2)
	data := map[string]any{"word": "hello"}

	t.Run("template functions from the renderer are available", func(t *testing.T) {
		got, err := vars.ResolveWithData("child", `{{ .word | upper }}`, data)
		require.NoError(t, err)
		assert.Equal(t, "HELLO", got)
	})

	t.Run("a second pass renders text produced by the first", func(t *testing.T) {
		got, err := vars.ResolveWithData("child", `{{ "{{ .word | upper }}" }}`, data)
		require.NoError(t, err)
		assert.Equal(t, "HELLO", got)
	})

	t.Run("a variables instance with one pass renders once", func(t *testing.T) {
		single := NewVariables()
		single.SetTemplateRenderer(upperRenderer)
		got, err := single.ResolveWithData("child", `{{ "{{ .word | upper }}" }}`, data)
		require.NoError(t, err)
		assert.Equal(t, "{{ .word | upper }}", got)
	})

	t.Run("empty input stays empty", func(t *testing.T) {
		got, err := vars.ResolveWithData("child", "", data)
		require.NoError(t, err)
		assert.Empty(t, got)
	})

	t.Run("render errors are returned", func(t *testing.T) {
		_, err := vars.ResolveWithData("child", `{{ .word | missing }}`, data)
		require.Error(t, err)
	})
}
