package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/internal/tui/templates"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestFormatAttributeValue_Documents recognizes complete JSON/YAML collections and preserves their format.
func TestFormatAttributeValue_Documents(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, input, format, expected string
	}{
		{"JSON object", `{"a":[1,2]}`, "json", "{\n   \"a\": [\n      1,\n      2\n   ]\n}"},
		{"JSON array", `["hello",true]`, "json", "[\n   \"hello\",\n   true\n]"},
		{"large JSON integer", `{"id":9007199254740993}`, "json", "{\n   \"id\": 9007199254740993\n}"},
		{"YAML mapping", "a:\n  b: true", "yaml", "a:\n  b: true"},
		{"YAML sequence", "- a\n- b\n", "yaml", "- a\n- b"},
		{"YAML flow collection", "{a: [b, c]}", "yaml", "a:\n  - b\n  - c"},
		{"YAML comments and tags", "# comment\na: !example text", "yaml", "# comment\na: !example text"},
		{"YAML block scalar", "script: |\n  echo hi\n  echo bye\n", "yaml", "script: |\n  echo hi\n  echo bye"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			value := formatAttributeValue(tt.input, nil)
			assert.Equal(t, tt.format, value.format)
			assert.Equal(t, tt.expected, strings.Join(value.lines, "\n"))
		})
	}
}

// TestFormatAttributeValue_OrdinaryText keeps scalar prose and invalid documents verbatim.
func TestFormatAttributeValue_OrdinaryText(t *testing.T) {
	t.Parallel()
	for _, input := range []string{
		"", "true", "123", "null", `"a JSON scalar"`, "plain text",
		"arn:aws:iam::000000000000:root", "https://example.com/path",
		"Note: managed by Terraform", "Owner:   team", "Owner:   team\n",
		"line one\nline two\n", "{invalid JSON", "a: [invalid YAML",
		"a: 1\na: 2", "a: *missing", "a: 1\n---\nb: 2", "a: 1\n---\n[invalid",
	} {
		t.Run(input, func(t *testing.T) {
			t.Parallel()
			value := formatAttributeValue(input, nil)
			assert.Empty(t, value.format)
			assert.Equal(t, input, strings.Join(value.lines, "\n"))
		})
	}
}

// TestRenderStructuredAttribute_UpdatesAndDeletions shows changes once while keeping shared lines.
func TestRenderStructuredAttribute_UpdatesAndDeletions(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, before, after, shared string
	}{
		{"JSON", `{"a":"old","b":"same"}`, `{"a":"new","b":"same"}`, `"b": "same"`},
		{"YAML", "a: old\nb: same", "a: new\nb: same", "b: same"},
		{"plain multiline", "old\nsame", "new\nsame", "same"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var b strings.Builder
			renderAttributeChanges(&b, []*AttributeChange{{Key: "value", Before: tt.before, After: tt.after}}, "│   ", &RenderConfig{Width: 80})
			output := ansi.Strip(b.String())
			assert.Contains(t, output, "- ")
			assert.Contains(t, output, "+ ")
			assert.Contains(t, output, "old")
			assert.Contains(t, output, "new")
			assert.Equal(t, 1, strings.Count(output, tt.shared), "unchanged lines must not be repeated")
			assert.NotContains(t, output, "→")
			assertTreeLayout(t, strings.Split(strings.TrimSuffix(output, "\n"), "\n"), 80)

			b.Reset()
			renderAttributeChanges(&b, []*AttributeChange{{Key: "value", Before: tt.before}}, "│   ", &RenderConfig{Width: 80})
			output = ansi.Strip(b.String())
			assert.Contains(t, output, "- ")
			assert.Contains(t, output, "old")
			assert.Contains(t, output, tt.shared)
			assert.NotContains(t, output, "+ ")
		})
	}
}

// TestRenderStructuredAttribute_TypeTransitions renders changes between scalars, collections, and unknowns.
func TestRenderStructuredAttribute_TypeTransitions(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name          string
		before, after any
		unknown       bool
		expected      []string
	}{
		{"scalar to JSON", "plain", `{"a":true}`, false, []string{"- plain", `+ {`, `"a": true`}},
		{"JSON to scalar", `{"a":true}`, "plain", false, []string{"- {", "+ plain"}},
		{"native collection to unknown", map[string]any{"a": true}, nil, true, []string{"- {", "+ (known after apply)"}},
		{"array to mapping", []any{"old"}, map[string]any{"a": "new"}, false, []string{"- [", "+ {", "old", "new"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var b strings.Builder
			renderAttributeChanges(&b, []*AttributeChange{{Key: "value", Before: tt.before, After: tt.after, Unknown: tt.unknown}}, "│   ", &RenderConfig{Width: 80})
			for _, expected := range tt.expected {
				assert.Contains(t, ansi.Strip(b.String()), expected)
			}
		})
	}
}

// TestRenderAttributeColumns_UnicodeAlignment aligns old-value columns by terminal display width.
func TestRenderAttributeColumns_UnicodeAlignment(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	renderAttributeChanges(&b, []*AttributeChange{
		{Key: "界", Before: "界", After: "first"},
		{Key: "abcd", Before: "abcd", After: "second"},
	}, "", &RenderConfig{Width: 120})
	rows := strings.Split(strings.TrimSuffix(ansi.Strip(b.String()), "\n"), "\n")
	require.Len(t, rows, 2)
	column1 := ansi.StringWidth(strings.SplitN(rows[0], "→", 2)[0])
	column2 := ansi.StringWidth(strings.SplitN(rows[1], "→", 2)[0])
	assert.Equal(t, column1, column2)
}

// TestResolveRenderConfig_FormattingContext retains explicit settings and resolves the default terminal width.
func TestResolveRenderConfig_FormattingContext(t *testing.T) {
	t.Parallel()
	config := &RenderConfig{Width: 97, AtmosConfig: &schema.AtmosConfiguration{}}
	resolved := resolveRenderConfig(config)
	assert.Equal(t, 97, resolved.Width)
	assert.Same(t, config.AtmosConfig, resolved.AtmosConfig)
	assert.Equal(t, templates.GetTerminalWidth(), resolveRenderConfig(nil).Width)
}

// TestRenderStructuredAttribute_FormattingSettings honors indentation and explicit syntax-disable settings.
func TestRenderStructuredAttribute_FormattingSettings(t *testing.T) {
	t.Parallel()
	config := &schema.AtmosConfiguration{}
	config.Settings.Terminal.ForceColor = true
	config.Settings.Terminal.TabWidth = 4
	config.Settings.Terminal.SyntaxHighlighting = schema.SyntaxHighlighting{Enabled: false, Theme: "dracula"}
	renderConfig := &RenderConfig{Width: 120, AtmosConfig: config}
	value := formatAttributeValue("a:\n  b: true", renderConfig)
	assert.Equal(t, "a:\n    b: true", strings.Join(value.lines, "\n"))
	assert.Equal(t, value.lines, value.displayLines(renderConfig), "disabled highlighting must remain disabled")
	assert.False(t, config.Settings.Terminal.SyntaxHighlighting.Enabled)
}

// TestRenderStructuredAttribute_HighlightingDoesNotChangeDiff keeps diff semantics independent of syntax coloring.
func TestRenderStructuredAttribute_HighlightingDoesNotChangeDiff(t *testing.T) {
	t.Parallel()
	for _, value := range []string{`{"a":"old","shared":true}`, "a: old\nshared: true\nscript: |\n  echo hi"} {
		change := &AttributeChange{Key: "policy", Before: value, After: strings.Replace(value, "old", "new", 1)}
		config := &schema.AtmosConfiguration{}
		config.Settings.Terminal.ForceColor = true
		var colored, plain strings.Builder
		renderAttributeChanges(&colored, []*AttributeChange{change}, "│   ", &RenderConfig{Width: 60, AtmosConfig: config})
		renderAttributeChanges(&plain, []*AttributeChange{change}, "│   ", &RenderConfig{Width: 60})
		assert.Equal(t, ansi.Strip(plain.String()), ansi.Strip(colored.String()))
		assert.Equal(t, 1, strings.Count(ansi.Strip(colored.String()), "shared"))
	}
}
