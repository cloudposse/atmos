package tui

// welcomeMessage introduces Atmos AI at the start of a new chat session, whether started
// via `atmos ai chat` or via the in-TUI "create session" form. It skips a top-level heading
// on purpose — the header bar above the transcript already reads "Atmos AI Assistant" and
// the message role label already reads "Atmos AI", so restating the name a third time here
// is just noise.
const welcomeMessage = `I'm here to help you with your Atmos infrastructure management. I can:

- Describe components and their configurations
- List available components and stacks
- Validate stack configurations
- Generate Terraform plans (read-only)
- Answer questions about Atmos concepts and best practices
- Help debug configuration issues

**Try asking me something like:**

- "List all available components"
- "Describe the vpc component in the dev stack"
- "What are Atmos stacks?"
- "How do I validate my stack configuration?"

What would you like to know?`

// addWelcomeMessage appends the standard welcome message as an assistant turn.
func (m *ChatModel) addWelcomeMessage() {
	m.addMessage(roleAssistant, welcomeMessage)
}
