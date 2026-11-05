package report_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/schema"
)

// writeTemplate writes a template file under dir and returns its path relative to dir.
func writeTemplate(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func templateConfig(base string, artifacts map[string]string) *schema.AtmosConfiguration {
	config := enabledConfig()
	config.CI.Templates = schema.CITemplatesConfig{BasePath: base, Artifacts: artifacts}
	return config
}

func TestCustomTemplatePrecedence(t *testing.T) {
	base := t.TempDir()
	writeTemplate(t, base, filepath.Join("artifacts", "summary.md"), "CONVENTION {{.Total}}\n{{range .Sections}}{{.}}\n{{end}}")
	writeTemplate(t, base, "explicit-summary.md", "EXPLICIT {{.Total}}\n")
	writeTemplate(t, base, filepath.Join("artifacts", "publish.md"), "PUBLISH-SECTION {{.Ref}} {{.Status}}")
	one := entry("publish", "a", 1, published("a", "x/y:1"), nil)

	t.Run("explicit override wins over convention", func(t *testing.T) {
		markdown := flushOne(t, templateConfig(base, map[string]string{"summary": "explicit-summary.md"}), one)
		assert.Equal(t, "EXPLICIT 1\n", markdown)
	})
	t.Run("convention wins over the embedded default", func(t *testing.T) {
		markdown := flushOne(t, templateConfig(base, nil), one)
		assert.True(t, strings.HasPrefix(markdown, "CONVENTION 1"), markdown)
		assert.Contains(t, markdown, "PUBLISH-SECTION `x/y:1` Published")
		assert.NotContains(t, markdown, "## Artifacts")
	})
	t.Run("embedded default without overrides", func(t *testing.T) {
		markdown := flushOne(t, templateConfig("", nil), one)
		assert.Contains(t, markdown, "## Artifacts")
	})
	t.Run("a section override replaces only that action", func(t *testing.T) {
		only := t.TempDir()
		writeTemplate(t, only, "pull.md", "PULL-ONLY {{.Ref}}")
		markdown := flushOne(
			t, templateConfig(only, map[string]string{"pull": "pull.md"}),
			entry("publish", "a", 1, published("a", "x/y:1"), nil),
			entry("pull", "b", 1, result(&artifact.OperationResult{Action: artifact.ActionPull, Outcome: artifact.OutcomeRead, Ref: "x/z:1"}), nil),
		)
		assert.Contains(t, markdown, "## Artifacts", "the default frame still renders")
		assert.Contains(t, markdown, "### ✅ 1. Publish `x/y:1`", "publish keeps the default section")
		assert.Contains(t, markdown, "PULL-ONLY `x/z:1`")
		assert.NotContains(t, markdown, "### ✅ 2.", "the pull section was replaced")
	})
}

func TestMissingExplicitOverrideIsAnError(t *testing.T) {
	for _, key := range []string{"summary", "publish"} {
		t.Run(key, func(t *testing.T) {
			config := templateConfig(t.TempDir(), map[string]string{key: "does-not-exist.md"})
			fake := &fakeReporter{}
			session := newSession(config, fake)
			session.Record(entry("publish", "a", 1, published("a", "x/y:1"), nil))
			err := session.Flush()
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrArtifactReportRender)
			assert.ErrorIs(t, err, errUtils.ErrCITemplateNotFound)
			assert.Empty(t, fake.summaries)
		})
	}
}

func TestInvalidTemplateIsAReportingError(t *testing.T) {
	base := t.TempDir()
	writeTemplate(t, base, "broken.md", "{{ .Nope }")
	writeTemplate(t, base, "unknown.md", "{{ .NoSuchField }}")
	for _, name := range []string{"broken.md", "unknown.md"} {
		fake := &fakeReporter{}
		session := newSession(templateConfig(base, map[string]string{"summary": name}), fake)
		session.Record(entry("publish", "a", 1, published("a", "x/y:1"), nil))
		err := session.Flush()
		require.ErrorIs(t, err, errUtils.ErrArtifactReportRender, name)
		assert.Empty(t, fake.summaries)
	}
}
