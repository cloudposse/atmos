package starlark

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUIStatusMarkdown(t *testing.T) {
	initCIFormatter(t)
	stdout, stderr, err := executeWithStreams(t, `
ui.info("1. numbered with dot")
ui.info("  1. indented numbered")
ui.info("2) paren style")
ui.info("- dash bullet")
ui.info("* star bullet")
ui.info("Step 1. mid-line 1. not a list")
ui.info("see a@b.com now")
ui.success("1. success numbered")
ui.warning("1. warning numbered")
print("1. stdout numbered")
`)
	require.NoError(t, err)
	assert.Equal(t, "1. stdout numbered\n", stdout)
	assert.Equal(t, `▶ 1. numbered with dot
▶ 1. indented numbered
▶ 2. paren style
▶ • dash bullet
▶ • star bullet
▶ Step 1. mid-line 1. not a list
▶ see a@b.com now
✓ 1. success numbered
⚠ 1. warning numbered
`, ansi.Strip(stderr))
}

func TestUIStatusKeepsMarkdownFormatting(t *testing.T) {
	initCIFormatter(t)
	stdout, stderr, err := executeWithStreams(t, "ui.info(\"**Plan** the `vpc`\")\n"+
		`ui.success("*Created* the **token**")
ui.warning("see https://atmos.tools now")
`)
	require.NoError(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, "▶ Plan the vpc\n✓ Created the token\n⚠ see https://atmos.tools now\n", ansi.Strip(stderr))
}

func TestUIStatusListsAndEscaping(t *testing.T) {
	initCIFormatter(t)
	stdout, stderr, err := executeWithStreams(t, `
ui.info("2. First\n3. Second")
ui.success("- First\n- Second")
ui.info("1\\. literal")
ui.warning("\\* literal")
ui.warning("\\- literal")
ui.info("See [docs](https://atmos.tools)")
`)
	require.NoError(t, err)
	assert.Empty(t, stdout)
	assert.Equal(t, `▶ 2. First
  3. Second
✓ • First
  • Second
▶ 1. literal
⚠ * literal
⚠ - literal
▶ See docs https://atmos.tools
`, ansi.Strip(stderr))
}
