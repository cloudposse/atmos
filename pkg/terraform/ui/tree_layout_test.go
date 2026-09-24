package ui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	uitree "github.com/cloudposse/atmos/pkg/ui/tree"
)

const kmsPolicy = `{"Statement":[{"Action":"kms:*","Effect":"Allow","Principal":{"AWS":"arn:aws:iam::000000000000:root"},"Resource":"*","Sid":"EnableRootAccess"},{"Action":["kms:ReEncrypt*","kms:GenerateDataKey*","kms:Encrypt","kms:DescribeKey","kms:Decrypt"],"Condition":{"ArnLike":{"kms:EncryptionContext:aws:logs:arn":"arn:aws:logs:us-east-1:000000000000:log-group:*"}},"Effect":"Allow","Principal":{"Service":"logs.us-east-1.amazonaws.com"},"Resource":"*","Sid":"AllowCloudWatchLogs"}],"Version":"2012-10-17"}`

func kmsPolicyTree() *DependencyTree {
	return &DependencyTree{
		Stack: "dev", Component: "kms",
		Root: &TreeNode{Children: []*TreeNode{{
			Address: "aws_kms_key.this", Action: "create",
			Changes: []*AttributeChange{
				{Key: "bypass_policy_lockout_safety_check", After: false},
				{Key: "policy", After: kmsPolicy},
			},
			UnchangedAttrCount: 4,
			Children:           []*TreeNode{{Address: "aws_kms_alias.this", Action: "create"}},
		}}},
	}
}

func TestRenderTree_KMSPolicyWrapping(t *testing.T) {
	t.Parallel()
	for _, width := range []int{60, 80, 120, 180} {
		for _, compact := range []bool{false, true} {
			for _, bar := range []bool{false, true} {
				t.Run(fmt.Sprintf("width=%d/compact=%t/bar=%t", width, compact, bar), func(t *testing.T) {
					t.Parallel()
					config := &RenderConfig{Width: width, Compact: compact, ShowAttributeBar: bar}
					output := ansi.Strip(kmsPolicyTree().RenderTreeWithConfig(config))
					rows := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
					assertTreeLayout(t, rows, width)
					assert.Contains(t, output, "# (4 unchanged attributes hidden)")
					assertKMSPolicyContent(t, rows)
				})
			}
		}
	}
}

func assertTreeLayout(t *testing.T, rows []string, width int) {
	t.Helper()
	for _, row := range rows {
		assert.LessOrEqual(t, ansi.StringWidth(row), width, "overflow: %s", row)
	}
	assert.Empty(t, uitree.Violations(rows), strings.Join(rows, "\n"))
}

func assertKMSPolicyContent(t *testing.T, rows []string) {
	t.Helper()
	start := -1
	for i, row := range rows {
		if strings.Contains(row, "policy ") {
			start = i
			break
		}
	}
	require.NotEqual(t, -1, start)
	require.Contains(t, rows[start], "policy (none) →")
	require.NotContains(t, rows[start], "{")
	start++
	column := ansi.StringWidth(strings.SplitN(rows[start], "{", 2)[0])
	var content strings.Builder
	for _, row := range rows[start:] {
		if strings.Contains(row, "# (") {
			break
		}
		content.WriteString(ansi.Cut(row, column, ansi.StringWidth(row)))
	}
	var actual, expected any
	require.NoError(t, json.Unmarshal([]byte(content.String()), &actual), content.String())
	require.NoError(t, json.Unmarshal([]byte(kmsPolicy), &expected))
	assert.Equal(t, expected, actual, "wrapping must preserve every policy field and ARN")
}

func TestRenderAttributeDocuments_BelowHeaderOnWideTerminal(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, firstLine string
		value           any
	}{
		{"JSON", "{", `{"enabled":true}`},
		{"YAML", "a:", "a:\n  b: true"},
		{"native map", "{", map[string]any{"enabled": true}},
		{"multiline text", "first line", "first line\nsecond line"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var b strings.Builder
			renderAttributeChanges(&b, []*AttributeChange{{Key: "value", After: tt.value}}, "│   ", &RenderConfig{Width: 240})
			rows := strings.Split(strings.TrimSuffix(ansi.Strip(b.String()), "\n"), "\n")
			require.GreaterOrEqual(t, len(rows), 3)
			assert.Equal(t, "     │   value (none) →", rows[0])
			assert.Equal(t, "     │     "+tt.firstLine, rows[1])
			assertTreeLayout(t, rows, 240)
		})
	}
}

func TestRenderAttributeDocuments_SingleLineValuesStayInline(t *testing.T) {
	t.Parallel()
	for _, value := range []any{"short", "{}", "[]", true} {
		var b strings.Builder
		renderAttributeChanges(&b, []*AttributeChange{{Key: "value", After: value}}, "│   ", &RenderConfig{Width: 240})
		rows := strings.Split(strings.TrimSuffix(ansi.Strip(b.String()), "\n"), "\n")
		require.Len(t, rows, 1)
		assert.Contains(t, rows[0], "→  "+fmt.Sprint(value))
	}
}

func TestRenderAttributeWrapping_PreservesTextAndRails(t *testing.T) {
	t.Parallel()
	for _, text := range []string{
		strings.Repeat("arn:aws:kms:us-east-1:000000000000:key/", 8),
		strings.Repeat("界é👩‍💻é", 40),
		strings.Repeat("words and spaces ", 30),
		"\x1b[31m" + strings.Repeat("colored", 40) + "\x1b[0m",
	} {
		t.Run(fmt.Sprintf("%.20s", ansi.Strip(text)), func(t *testing.T) {
			t.Parallel()
			var b strings.Builder
			const prefix = "     │   "
			writeWrappedAttributeLine(&b, text, prefix, prefix, 60)
			var recovered strings.Builder
			rows := strings.Split(strings.TrimSuffix(ansi.Strip(b.String()), "\n"), "\n")
			for _, row := range rows {
				require.True(t, strings.HasPrefix(row, prefix), row)
				assert.True(t, utf8.ValidString(row))
				assert.LessOrEqual(t, ansi.StringWidth(row), 60)
				recovered.WriteString(strings.TrimPrefix(row, prefix))
			}
			assert.Equal(t, ansi.Strip(text), recovered.String())
			if strings.Contains(text, "\x1b[") {
				for _, row := range strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n") {
					assert.Contains(t, row, "\x1b[31m", "each segment must retain the token color")
					assert.Contains(t, row, "\x1b[0m", "styles must end before the next gutter")
				}
			}
		})
	}
}

func TestRenderAttributeWrapping_LongUpdateAndKey(t *testing.T) {
	t.Parallel()
	before, after := strings.Repeat("x", 160), strings.Repeat("y", 180)
	var b strings.Builder
	renderAttributeChanges(&b, []*AttributeChange{{
		Key: strings.Repeat("k", 80), Before: before, After: after, ForcesReplacement: true,
	}}, "│   ", &RenderConfig{Width: 60, ShowAttributeBar: true})
	output := ansi.Strip(b.String())
	assert.Equal(t, 160, strings.Count(output, "x"))
	assert.Equal(t, 180, strings.Count(output, "y"))
	assert.Equal(t, 80, strings.Count(output, "k"))
	assert.Contains(t, output, "# forces replacement")
	assert.Contains(t, output, "- x")
	assert.Contains(t, output, "+ y")
	assertTreeLayout(t, strings.Split(strings.TrimSuffix(output, "\n"), "\n"), 60)
}

func TestRenderAttributeWrapping_SensitiveAndUnknown(t *testing.T) {
	t.Parallel()
	for _, value := range []any{`{"secret":"hidden-value"}`, "secret: hidden-value", map[string]any{"secret": "hidden-value"}} {
		for _, unknown := range []bool{false, true} {
			var b strings.Builder
			renderAttributeChanges(&b, []*AttributeChange{{Key: "policy", Before: value, After: value, Sensitive: true, Unknown: unknown}}, "│   ", &RenderConfig{Width: 60})
			assert.NotContains(t, b.String(), "hidden-value")
			assert.Contains(t, b.String(), "(sensitive)")
			if unknown {
				assert.Contains(t, b.String(), "(known after apply)")
			}
		}
		var b strings.Builder
		renderAttributeChanges(&b, []*AttributeChange{{Key: "policy", After: value, Unknown: true}}, "│   ", &RenderConfig{Width: 60})
		assert.NotContains(t, b.String(), "hidden-value")
		assert.Contains(t, b.String(), "(known after apply)")
	}
}

func TestRenderStructuredAttribute_ColorAndCollapsing(t *testing.T) {
	t.Parallel()
	for _, noColor := range []bool{false, true} {
		t.Run(fmt.Sprintf("noColor=%t", noColor), func(t *testing.T) {
			t.Parallel()
			atmosConfig := &schema.AtmosConfiguration{}
			atmosConfig.Settings.Terminal.ForceColor = true
			atmosConfig.Settings.Terminal.NoColor = noColor
			atmosConfig.Settings.Terminal.MaxWidth = 123
			config := &RenderConfig{Width: 120, MaxLines: 10, AtmosConfig: atmosConfig}
			output := kmsPolicyTree().RenderTreeWithConfig(config)
			assert.Contains(t, ansi.Strip(output), "lines omitted")
			assert.Contains(t, ansi.Strip(output), `"Version": "2012-10-17"`)
			assertTreeLayout(t, strings.Split(strings.TrimSuffix(ansi.Strip(output), "\n"), "\n"), 120)
			value := formatAttributeValue(kmsPolicy, nil)
			colored := strings.Join(value.displayLines(config), "\n")
			assert.Equal(t, !noColor, strings.Contains(colored, "\x1b["))
			assert.Equal(t, 123, atmosConfig.Settings.Terminal.MaxWidth, "formatting must not mutate caller settings")
		})
	}
}
