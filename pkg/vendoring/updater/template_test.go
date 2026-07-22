package updater

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/vendoring"
)

func TestRenderPRTemplates(t *testing.T) {
	report := &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", CurrentVersion: "1", LatestVersion: "2"}}}

	t.Run("custom templates", func(t *testing.T) {
		templates := PRTemplates{Title: "update {{ .scope.name }}", Body: "{{ .updates | markdownTable }}"}
		title, body, err := RenderPRTemplates(templates, "all", report)
		require.NoError(t, err)
		assert.Equal(t, "update all", title)
		assert.Contains(t, body, "| vpc | 1 | 2 |")
	})

	t.Run("default templates when empty", func(t *testing.T) {
		title, body, err := RenderPRTemplates(PRTemplates{}, "group-platform", report)
		require.NoError(t, err)
		assert.Equal(t, "chore(components): update group-platform", title)
		assert.Contains(t, body, "| vpc | 1 | 2 |")
	})

	t.Run("invalid title template errors", func(t *testing.T) {
		_, _, err := RenderPRTemplates(PRTemplates{Title: "{{"}, "all", report)
		require.Error(t, err)
	})

	t.Run("invalid body template errors", func(t *testing.T) {
		_, _, err := RenderPRTemplates(PRTemplates{Body: "{{ .broken"}, "all", report)
		require.Error(t, err)
	})
}

func TestTemplateFunctionsMarkdownTable(t *testing.T) {
	fn, ok := TemplateFunctions()["markdownTable"].(func([]vendoring.SourceUpdateResult) string)
	require.True(t, ok)
	out := fn([]vendoring.SourceUpdateResult{{Component: "vpc", CurrentVersion: "1.0.0", LatestVersion: "1.1.0"}})
	assert.Contains(t, out, "| Component | Current | Latest |")
	assert.Contains(t, out, "| vpc | 1.0.0 | 1.1.0 |")
}
