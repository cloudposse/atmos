package updater

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/vendoring"
)

// fakePublisher is a minimal atmosgit.PullRequestPublisher that does NOT implement
// atmosgit.PullRequestBodyBadger, used to prove RenderPRTemplates falls back to the static badge
// for a publisher that hasn't opted into its own.
type fakePublisher struct{}

func (fakePublisher) Reconcile(context.Context, *atmosgit.PullRequestOptions) (*atmosgit.PullRequestResult, error) {
	return nil, nil
}

// fakeBadgedPublisher additionally implements atmosgit.PullRequestBodyBadger, used to prove its
// own badge wins over the static fallback.
type fakeBadgedPublisher struct{ fakePublisher }

func (fakeBadgedPublisher) PullRequestBodyBadge() string { return "CUSTOM-BADGE" }

func TestRenderPRTemplates(t *testing.T) {
	report := &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", CurrentVersion: "1", LatestVersion: "2"}}}

	t.Run("custom templates", func(t *testing.T) {
		templates := PRTemplates{Title: "update {{ .scope.name }}", Body: "{{ .updates | markdownTable }}"}
		title, body, err := RenderPRTemplates(templates, "all", report, fakePublisher{})
		require.NoError(t, err)
		assert.Equal(t, "update all", title)
		assert.Contains(t, body, "| vpc | 1 | 2 |")
	})

	t.Run("default templates when empty", func(t *testing.T) {
		title, body, err := RenderPRTemplates(PRTemplates{}, "group-platform", report, fakePublisher{})
		require.NoError(t, err)
		assert.Equal(t, "chore(components): update group-platform", title)
		assert.Contains(t, body, "| vpc | 1 | 2 |")
	})

	// TestRenderPRTemplates/default_body_is_not_just_a_bare_table proves the default body isn't
	// just an unexplained table -- confirmed by manual review of a real generated pull request
	// that a bare table with no branding or context reads as broken/unhelpful to a reviewer.
	t.Run("default body is not just a bare table", func(t *testing.T) {
		_, body, err := RenderPRTemplates(PRTemplates{}, "all", report, fakePublisher{})
		require.NoError(t, err)
		assert.Contains(t, body, "Atmos CI", "default body must carry the Atmos CI badge")
		assert.Contains(t, body, "atmos vendor update --pull-request", "default body must explain what generated this PR")
		assert.Contains(t, body, "| vpc | 1 | 2 |")
	})

	// TestRenderPRTemplates/default_body_falls_back_to_the_static_badge proves a publisher that
	// doesn't implement atmosgit.PullRequestBodyBadger gets the plain, dependency-free fallback --
	// not a hardcoded, forge-specific badge some other provider might not be able to render.
	t.Run("default body falls back to the static badge", func(t *testing.T) {
		_, body, err := RenderPRTemplates(PRTemplates{}, "all", report, fakePublisher{})
		require.NoError(t, err)
		assert.Contains(t, body, defaultBadge)
	})

	// TestRenderPRTemplates/default_body_uses_the_publisher's_own_badge proves a publisher that
	// does implement atmosgit.PullRequestBodyBadger has its own badge used instead of the static
	// fallback -- this is the mechanism GitHub's and Azure DevOps' own providers rely on to each
	// supply markdown appropriate to their own pull request rendering.
	t.Run("default body uses the publisher's own badge", func(t *testing.T) {
		_, body, err := RenderPRTemplates(PRTemplates{}, "all", report, fakeBadgedPublisher{})
		require.NoError(t, err)
		assert.Contains(t, body, "CUSTOM-BADGE")
		assert.NotContains(t, body, defaultBadge, "the publisher's own badge must replace, not just append to, the static fallback")
	})

	t.Run("invalid title template errors", func(t *testing.T) {
		_, _, err := RenderPRTemplates(PRTemplates{Title: "{{"}, "all", report, fakePublisher{})
		require.Error(t, err)
	})

	t.Run("invalid body template errors", func(t *testing.T) {
		_, _, err := RenderPRTemplates(PRTemplates{Body: "{{ .broken"}, "all", report, fakePublisher{})
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
