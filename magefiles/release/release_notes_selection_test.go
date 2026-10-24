//go:build mage

package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/internal/ci/releasenotes"
	"github.com/cloudposse/atmos/magefiles/internal/mageutil"
)

func TestProductionReleaseNotesCategories(t *testing.T) {
	config, err := loadReleaseNotesSelection()
	require.NoError(t, err)
	for _, tc := range []struct{ label, heading string }{
		{"major", "## 💥 Breaking Changes"},
		{"minor", "## ✨ Features"},
		{"feature", "## ✨ Features"},
		{"enhancement", "## 🚀 Enhancements"},
		{"patch", "## 🐛 Bug Fixes"},
		{"fix", "## 🐛 Bug Fixes"},
		{"bugfix", "## 🐛 Bug Fixes"},
		{"bug", "## 🐛 Bug Fixes"},
		{"hotfix", "## 🐛 Bug Fixes"},
		{"auto-update", "## 🤖 Automatic Updates"},
		{"no-release", ""},
		{"documentation", ""},
		{"", ""},
	} {
		t.Run(tc.label, func(t *testing.T) {
			pr := planPull(42, tc.label, nil)
			require.Equal(t, releasenotes.PREntry{
				Number: 42, Category: tc.heading,
				Summary: "Fix credentials handling @contributor (#42)", Body: "PR body is data",
			}, config.noteEntry(&pr))
		})
	}
}

func TestProductionReleaseNotesUseFirstCategoryAndStableOrder(t *testing.T) {
	config, err := loadReleaseNotesSelection()
	require.NoError(t, err)
	pr := planPull(99, "patch", nil)
	pr.Labels = append(pr.Labels, struct {
		Name string `json:"name"`
	}{Name: "minor"})
	require.Equal(t, "## ✨ Features", config.noteEntry(&pr).Category, "configuration order takes precedence over API label order")
	var entries []releasenotes.PREntry
	for i, label := range []string{"auto-update", "patch", "enhancement", "minor", "major", "no-release", "patch"} {
		pr = planPull(i+1, label, nil)
		entries = append(entries, config.noteEntry(&pr))
	}
	config.sortEntries(entries)
	var numbers []int
	for _, entry := range entries {
		numbers = append(numbers, entry.Number)
	}
	require.Equal(t, []int{6, 5, 4, 3, 2, 7, 1}, numbers)
	body, err := releasenotes.RenderBody(entries, make([]string, len(entries)))
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(body, "## 📦 Other Changes\n\n- Fix credentials handling @contributor (#6)"))
	require.Equal(t, 1, strings.Count(body, "## 🐛 Bug Fixes"), "grouped notes must not repeat category headings")
}

func TestReleaseNotesConfigControlsCategoriesAndChangeTemplate(t *testing.T) {
	config, err := parseReleaseNotesSelection(strings.NewReader(`filter-by-commitish: true
categories:
  - title: Custom Fixes
    labels: [patch, minor]
  - title: Later Features
    labels: [minor]
change-template: '- #$NUMBER by @$AUTHOR: $TITLE'
template: '$CHANGES'
`))
	require.NoError(t, err)
	pr := planPull(42, "minor", nil)
	pr.Title = "Literal $AUTHOR\n  and $NUMBER"
	pr.Body = "Body stays unchanged\n\nwith Markdown."
	require.Equal(t, releasenotes.PREntry{
		Number: 42, Category: "## Custom Fixes",
		Summary: "#42 by @contributor: Literal $AUTHOR and $NUMBER", Body: pr.Body,
	}, config.noteEntry(&pr), "title tokens must remain data rather than being expanded twice")
}

func TestReleaseNotesConfigurationErrors(t *testing.T) {
	root, err := mageutil.RepoRoot()
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(root, ".github", "release-notes.yml"))
	require.NoError(t, err)
	production := string(data)
	for _, tc := range []struct{ name, content string }{
		{"invalid YAML", "{"},
		{"empty", ""},
		{"unrecognized option", production + "version-resolver: {}\n"},
		{"source filter disabled", strings.Replace(production, "filter-by-commitish: true", "filter-by-commitish: false", 1)},
		{"unsupported body template", strings.Replace(production, "$CHANGES", "$BODY", 1)},
		{"unsupported change token", strings.Replace(production, "$TITLE", "$BODY", 1)},
		{"non bullet", strings.Replace(production, "- $TITLE", "$TITLE", 1)},
		{"multiline bullet", strings.Replace(production, "- $TITLE @$AUTHOR (#$NUMBER)", "- $TITLE\n\n  @$AUTHOR (#$NUMBER)", 1)},
		{"empty category", strings.Replace(production, "💥 Breaking Changes", "", 1)},
		{"duplicate heading", strings.Replace(production, "✨ Features", "💥 Breaking Changes", 1)},
		{"missing labels", strings.Replace(production, "labels:\n  - 'major'", "labels: []", 1)},
		{"empty label", strings.Replace(production, "- 'major'", "- ''", 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config, err := parseReleaseNotesSelection(strings.NewReader(tc.content))
			require.ErrorIs(t, err, errReleaseNotesConfig)
			require.Nil(t, config)
		})
	}
}

func TestReleaseNotesConfigurationRequiresRepositoryFile(t *testing.T) {
	t.Chdir(t.TempDir())
	_, err := loadReleaseNotesSelection()
	require.ErrorIs(t, err, mageutil.ErrRepoRootNotFound)
	require.NoError(t, os.WriteFile("go.mod", []byte("module github.com/cloudposse/atmos\n"), 0o600))
	_, err = loadReleaseNotesSelection()
	require.ErrorIs(t, err, os.ErrNotExist)
}
