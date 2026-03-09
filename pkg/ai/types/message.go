package types

// Message represents a single message in a conversation.
type Message struct {
	Role    string // "user", "assistant", or "system"
	Content string // The message content
}

// Role constants for messages.
const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
)
