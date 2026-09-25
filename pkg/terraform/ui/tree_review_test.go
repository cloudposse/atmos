package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestRenderAttributeDiff_FormattingOnlyChanges keeps differences in the actual
// Terraform string visible when pretty formatting would normalize them away.
func TestRenderAttributeDiff_FormattingOnlyChanges(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, before, after string
	}{
		{"JSON whitespace", `{"a":1}`, `{ "a": 1 }`},
		{"JSON key order", `{"a":1,"b":2}`, `{"b":2,"a":1}`},
		{"YAML indentation", "a:\n  b: true", "a:\n    b: true"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var b strings.Builder
			renderAttributeChanges(&b, []*AttributeChange{{Key: "policy", Before: tt.before, After: tt.after}}, "│   ", &RenderConfig{Width: 80})
			output := ansi.Strip(b.String())
			beforeLines, afterLines := strings.Split(tt.before, "\n"), strings.Split(tt.after, "\n")
			assert.Contains(t, output, "- "+beforeLines[len(beforeLines)-1])
			assert.Contains(t, output, "+ "+afterLines[len(afterLines)-1])
			assertTreeLayout(t, strings.Split(strings.TrimSuffix(output, "\n"), "\n"), 80)

			b.Reset()
			renderAttributeChanges(&b, []*AttributeChange{{Key: "policy", Before: tt.before, After: tt.before}}, "│   ", &RenderConfig{Width: 80})
			output = ansi.Strip(b.String())
			assert.NotContains(t, output, "- ", "identical raw values must keep the formatted view")
			assert.NotContains(t, output, "+ ")
		})
	}
}

// TestRenderAttributeDiff_FormattingOnlyChangesRespectMaxLines keeps literal document
// differences visible without expanding the omitted middle, with or without colors.
func TestRenderAttributeDiff_FormattingOnlyChangesRespectMaxLines(t *testing.T) {
	t.Parallel()
	jsonDocument := "{\n  \"a\": 1,\n  \"b\": 2,\n  \"c\": 3,\n  \"d\": 4,\n  \"e\": 5\n}"
	yamlDocument := "a: 1\nb: 2\nc:\n  child: true\nd: 4\ne: 5"
	for _, tt := range []struct {
		name, before, after, hidden string
	}{
		{"JSON whitespace", jsonDocument, strings.Replace(jsonDocument, "  \"c\"", "    \"c\"", 1), `"c"`},
		{"JSON key order", jsonDocument, strings.Replace(jsonDocument, "  \"b\": 2,\n  \"c\": 3,", "  \"c\": 3,\n  \"b\": 2,", 1), `"c"`},
		{"YAML indentation", yamlDocument, strings.Replace(yamlDocument, "  child", "    child", 1), "child"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			for _, maxLines := range []int{0, 3} {
				for _, noColor := range []bool{true, false} {
					atmosConfig := &schema.AtmosConfiguration{}
					atmosConfig.Settings.Terminal.NoColor = noColor
					var b strings.Builder
					renderAttributeChanges(&b, []*AttributeChange{{Key: "policy", Before: tt.before, After: tt.after}}, "│   ",
						&RenderConfig{Width: 80, MaxLines: maxLines, AtmosConfig: atmosConfig})
					output := ansi.Strip(b.String())
					assert.Contains(t, output, "- ")
					assert.Contains(t, output, "+ ")
					if maxLines > 0 {
						assert.Contains(t, output, "-     ... (")
						assert.Contains(t, output, "+     ... (")
						assert.NotContains(t, output, tt.hidden)
						assert.LessOrEqual(t, strings.Count(output, "\n"), 1+2*maxLines)
					} else {
						assert.Contains(t, output, tt.hidden)
						assert.NotContains(t, output, "lines omitted")
					}
					assertTreeLayout(t, strings.Split(strings.TrimSuffix(output, "\n"), "\n"), 80)
				}
			}
		})
	}
}

// TestRenderAttributeDiff_FormattingFallbackProtectsValues keeps normalized
// differences from exposing sensitive values or a value that is still unknown.
func TestRenderAttributeDiff_FormattingFallbackProtectsValues(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, placeholder  string
		sensitive, unknown bool
	}{
		{"sensitive", "(sensitive)", true, false},
		{"unknown", "(known after apply)", false, true},
		{"sensitive unknown", "(known after apply)", true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var b strings.Builder
			renderAttributeChanges(&b, []*AttributeChange{{
				Key: "policy", Before: `{"secret":true}`, After: `{ "secret": true }`,
				Sensitive: tt.sensitive, Unknown: tt.unknown,
			}}, "", &RenderConfig{Width: 80})
			output := ansi.Strip(b.String())
			assert.Contains(t, output, tt.placeholder)
			assert.NotContains(t, output, `{ "secret": true }`)
			if tt.sensitive {
				assert.NotContains(t, output, "secret")
			}
		})
	}
}

// TestRenderAttributeDiff_CollapsedChanges requires a visible change marker even
// when the only changed field lies inside the omitted middle of a document.
func TestRenderAttributeDiff_CollapsedChanges(t *testing.T) {
	t.Parallel()
	for _, input := range []string{
		`{"a":1,"b":2,"c":"old","d":4,"e":5,"f":6}`,
		"a: 1\nb: 2\nc: old\nd: 4\ne: 5\nf: 6",
	} {
		for _, changed := range []bool{true, false} {
			after := input
			if changed {
				after = strings.Replace(input, "old", "new", 1)
			}
			var b strings.Builder
			renderAttributeChanges(&b, []*AttributeChange{{Key: "policy", Before: input, After: after}}, "│   ", &RenderConfig{Width: 80, MaxLines: 3})
			output := ansi.Strip(b.String())
			assert.NotContains(t, output, "old")
			assert.NotContains(t, output, "new")
			assert.Contains(t, output, "lines omitted")
			if changed {
				assert.Contains(t, output, "-     ... (")
				assert.Contains(t, output, "+     ... (")
			} else {
				assert.NotContains(t, output, "- ")
				assert.NotContains(t, output, "+ ")
			}
			assertTreeLayout(t, strings.Split(strings.TrimSuffix(output, "\n"), "\n"), 80)
		}
	}
}

// TestRenderAttributeDiff_ProseStaysInline preserves scalar spelling and the
// compact old-to-new comparison for colon-containing single-line descriptions.
func TestRenderAttributeDiff_ProseStaysInline(t *testing.T) {
	t.Parallel()
	var b strings.Builder
	renderAttributeChanges(&b, []*AttributeChange{{Key: "description", Before: "Owner:   old", After: "Owner:   new"}}, "", &RenderConfig{Width: 120})
	output := ansi.Strip(b.String())
	assert.Contains(t, output, `"Owner:   old"  →  Owner:   new`)
	assert.Equal(t, 1, strings.Count(output, "\n"))
}
