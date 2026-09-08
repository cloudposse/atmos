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
		title, body, err := RenderPRTemplates(templates, "all", report, "")
		require.NoError(t, err)
		assert.Equal(t, "update all", title)
		assert.Contains(t, body, "| vpc | 1 | 2 |")
	})

	t.Run("default templates when empty", func(t *testing.T) {
		title, body, err := RenderPRTemplates(PRTemplates{}, "group-platform", report, "")
		require.NoError(t, err)
		assert.Equal(t, "chore(components): update group-platform", title)
		assert.Contains(t, body, "| vpc | 1 | 2 |")
	})

	// TestRenderPRTemplates/default_body_is_not_just_a_bare_table proves the default body isn't
	// just an unexplained table -- confirmed by manual review of a real generated pull request
	// that a bare table with no branding or context reads as broken/unhelpful to a reviewer.
	t.Run("default body is not just a bare table", func(t *testing.T) {
		_, body, err := RenderPRTemplates(PRTemplates{}, "all", report, "")
		require.NoError(t, err)
		assert.Contains(t, body, `<a href="https://atmos.tools/ci">`, "default body must carry the Atmos CI badge")
		assert.Contains(t, body, "atmos vendor update --pull-request", "default body must explain what generated this PR")
		assert.Contains(t, body, "| vpc | 1 | 2 |")
	})

	// TestRenderPRTemplates/default_body_uses_the_configured_provider's_badge proves a provider
	// present in badgesByProvider gets its own badge instead of atmosCIBadge's raw HTML, which was
	// confirmed (against a real generated pull request) not to render for at least one provider.
	t.Run("default body uses the configured provider's badge", func(t *testing.T) {
		_, body, err := RenderPRTemplates(PRTemplates{}, "all", report, "azuredevops")
		require.NoError(t, err)
		assert.Contains(t, body, "**[Atmos CI](https://atmos.tools/ci)**", "must use the image-free badge")
		assert.NotContains(t, body, "<picture>", "must not carry markup the provider doesn't render")
		assert.NotContains(t, body, "![", "must not reference an image the provider can't load")
	})

	t.Run("invalid title template errors", func(t *testing.T) {
		_, _, err := RenderPRTemplates(PRTemplates{Title: "{{"}, "all", report, "")
		require.Error(t, err)
	})

	t.Run("invalid body template errors", func(t *testing.T) {
		_, _, err := RenderPRTemplates(PRTemplates{Body: "{{ .broken"}, "all", report, "")
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
