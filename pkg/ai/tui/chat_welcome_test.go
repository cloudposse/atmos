package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/glamour"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWelcomeMessage_UsesMarkdown(t *testing.T) {
	assert.Contains(t, welcomeMessage, "**")
	assert.Contains(t, welcomeMessage, "- ")
	assert.NotContains(t, welcomeMessage, "•") // No literal "•" bullets.
	// No top-level heading: the header bar and message role label already say "Atmos AI".
	assert.NotContains(t, welcomeMessage, "##")
}

func TestChatModel_AddWelcomeMessage(t *testing.T) {
	client := &mockAIClient{
		model:     "test-model",
		maxTokens: 4096,
	}

	model, err := NewChatModel(ChatModelParams{Client: client})
	require.NoError(t, err)

	initialCount := len(model.messages)
	model.addWelcomeMessage()

	require.Len(t, model.messages, initialCount+1)
	lastMsg := model.messages[len(model.messages)-1]
	assert.Equal(t, roleAssistant, lastMsg.Role)
	assert.Equal(t, welcomeMessage, lastMsg.Content)
	assert.True(t, strings.Contains(lastMsg.Content, "infrastructure management"))
}

// TestBuildGlamourOptions_AppliesStyling guards against regressing to glamour.WithAutoStyle(),
// which silently renders headings/emphasis with zero ANSI styling (leaving literal "##"/"**"
// in the output) whenever it can't complete its live terminal background probe — exactly what
// happens in wrapped/embedded terminals, and in this test's own non-TTY process too. This
// function must keep using Atmos' own theme-derived style instead, which applies styling
// unconditionally.
func TestBuildGlamourOptions_AppliesStyling(t *testing.T) {
	content := "## Heading\n\n**bold**"

	renderer, err := glamour.NewTermRenderer(buildGlamourOptions(80, nil)...)
	require.NoError(t, err)

	rendered, err := renderer.Render(content)
	require.NoError(t, err)

	assert.Contains(t, rendered, "\x1b[", "theme-derived glamour style must emit ANSI styling, not render flat text")
}
