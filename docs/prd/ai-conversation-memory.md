# PRD: AI Conversation Memory

## Overview

Implement conversation history/memory for Atmos AI to enable multi-turn conversations where the AI remembers previous questions and answers within a session.

## Problem Statement

Currently, Atmos AI only sends the current user message to AI providers, without any conversation history. This means:

- The AI cannot reference previous questions or answers
- Users have to repeat context in every message
- Follow-up questions don't work ("yes", "tell me more about that", etc.)
- The AI appears to have amnesia between messages

**Example of current broken behavior:**
```
User: What are Atmos stacks?
AI: [Explains stacks in detail...]

User: yes
AI: Hello! I see you've said "yes" - I'd be happy to help, but I don't have the context...
```

## Current Implementation

### What Works
- ✅ Session storage with SQLite backend
- ✅ Message persistence to database
- ✅ Loading messages when resuming sessions
- ✅ Displaying message history in TUI

### What's Broken
- ❌ Messages are NOT sent to AI providers
- ❌ AI only receives the current message
- ❌ No conversation context

**Current code in `pkg/ai/agent/anthropic/client.go:103-105`:**
```go
Messages: []anthropic.MessageParam{
    anthropic.NewUserMessage(anthropic.NewTextBlock(message)),  // Only ONE message!
},
```

## Solution Design

### 1. Update Client Interface

Add message history support to the `ai.Client` interface:

```go
// pkg/ai/client.go
type Message struct {
    Role    string // "user", "assistant", "system"
    Content string
}

type Client interface {
    // New methods with history support
    SendMessageWithHistory(ctx context.Context, messages []Message) (string, error)
    SendMessageWithToolsAndHistory(ctx context.Context, messages []Message, availableTools []tools.Tool) (*types.Response, error)

    // Keep existing methods for backward compatibility
    SendMessage(ctx context.Context, message string) (string, error)
    SendMessageWithTools(ctx context.Context, message string, availableTools []tools.Tool) (*types.Response, error)

    GetModel() string
    GetMaxTokens() int
}
```

### 2. Provider Implementation

Each provider needs to convert `[]Message` to their SDK's message format:

#### Anthropic
```go
anthropicMessages := make([]anthropic.MessageParam, 0, len(messages))
for _, msg := range messages {
    switch msg.Role {
    case "user":
        anthropicMessages = append(anthropicMessages,
            anthropic.NewUserMessage(anthropic.NewTextBlock(msg.Content)))
    case "assistant":
        anthropicMessages = append(anthropicMessages,
            anthropic.NewAssistantMessage(anthropic.NewTextBlock(msg.Content)))
    }
}
```

#### OpenAI/Grok/Ollama/Azure OpenAI
```go
openaiMessages := []openai.ChatCompletionMessageParamUnion{}
for _, msg := range messages {
    switch msg.Role {
    case "user":
        openaiMessages = append(openaiMessages, openai.UserMessage(msg.Content))
    case "assistant":
        openaiMessages = append(openaiMessages, openai.AssistantMessage(msg.Content))
    case "system":
        openaiMessages = append(openaiMessages, openai.SystemMessage(msg.Content))
    }
}
```

#### Gemini
```go
session := model.StartChat()
session.History = []*genai.Content{}
for _, msg := range messages[:len(messages)-1] {  // All except last
    role := "user"
    if msg.Role == "assistant" {
        role = "model"
    }
    session.History = append(session.History, &genai.Content{
        Role: role,
        Parts: []genai.Part{genai.Text(msg.Content)},
    })
}
// Send last message as current prompt
lastMessage := messages[len(messages)-1]
resp, err := session.SendMessage(ctx, genai.Text(lastMessage.Content))
```

#### Bedrock
```go
// Bedrock uses Anthropic's message format
messages := []map[string]interface{}{}
for _, msg := range messages {
    messages = append(messages, map[string]interface{}{
        "role": msg.Role,
        "content": msg.Content,
    })
}
```

### 3. Update Chat TUI

Modify `pkg/ai/tui/chat.go:634` to build and send full message history:

```go
func (m *ChatModel) getAIResponse(userMessage string) tea.Cmd {
    return func() tea.Msg {
        ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
        defer cancel()

        // Build full message history from stored messages
        messages := make([]ai.Message, 0, len(m.messages)+1)
        for _, msg := range m.messages {
            // Skip system messages (UI-only notifications)
            if msg.Role != roleSystem {
                messages = append(messages, ai.Message{
                    Role:    msg.Role,
                    Content: msg.Content,
                })
            }
        }

        // Add current user message
        messages = append(messages, ai.Message{
            Role:    roleUser,
            Content: userMessage,
        })

        // Apply memory context if available
        if m.memoryMgr != nil {
            memoryContext := m.memoryMgr.GetContext()
            if memoryContext != "" {
                // Prepend system message with context
                messages = append([]ai.Message{{
                    Role:    "system",
                    Content: memoryContext,
                }}, messages...)
            }
        }

        // Get tools
        var availableTools []tools.Tool
        if m.executor != nil {
            availableTools = m.executor.ListTools()
        }

        // Send with full history
        if len(availableTools) > 0 {
            response, err := m.client.SendMessageWithToolsAndHistory(ctx, messages, availableTools)
            // ... handle response
        } else {
            response, err := m.client.SendMessageWithHistory(ctx, messages)
            // ... handle response
        }
    }
}
```

### 4. Conversation Limits

To prevent token limits from being exceeded:

1. **Default limit**: Last 20 messages (10 exchanges)
2. **Configurable in atmos.yaml**:
   ```yaml
   settings:
     ai:
       max_history_messages: 20
   ```
3. **Smart truncation**: Keep first 2 messages (context) + last N messages

## Implementation Plan

### Phase 1: Core Implementation (Step-by-step)
1. ✅ Create PRD document
2. Create `pkg/ai/types/message.go` with `Message` type
3. Update `pkg/ai/client.go` interface
4. Update Anthropic provider implementation
5. Update OpenAI provider implementation
6. Update Gemini provider implementation
7. Update Grok provider implementation
8. Update Ollama provider implementation
9. Update Bedrock provider implementation
10. Update Azure OpenAI provider implementation
11. Update chat TUI to use new methods

### Phase 2: Testing
1. Add unit tests for message history conversion
2. Add integration tests for multi-turn conversations
3. Test with all 7 providers

### Phase 3: Documentation
1. Update `website/docs/ai/getting-started.md`
2. Update `website/docs/ai/tools.mdx`
3. Add examples of multi-turn conversations

## Testing Strategy

### Unit Tests
```go
func TestAnthropicClient_SendMessageWithHistory(t *testing.T) {
    messages := []ai.Message{
        {Role: "user", Content: "What is Atmos?"},
        {Role: "assistant", Content: "Atmos is..."},
        {Role: "user", Content: "Tell me more"},
    }

    // Verify all messages are sent
    // Verify correct role mapping
}
```

### Integration Tests
```go
func TestMultiTurnConversation(t *testing.T) {
    // First message
    response1, _ := client.SendMessage(ctx, "What is 2+2?")
    assert.Contains(t, response1, "4")

    // Follow-up referring to previous answer
    messages := []ai.Message{
        {Role: "user", Content: "What is 2+2?"},
        {Role: "assistant", Content: response1},
        {Role: "user", Content: "What about if I multiply that by 3?"},
    }
    response2, _ := client.SendMessageWithHistory(ctx, messages)
    assert.Contains(t, response2, "12")  // Should understand "that" refers to 4
}
```

## Success Criteria

1. ✅ AI remembers previous messages in a session
2. ✅ Follow-up questions work correctly ("yes", "tell me more", etc.)
3. ✅ Resuming sessions maintains conversation context
4. ✅ All 7 providers support message history
5. ✅ Tests pass with >80% coverage
6. ✅ Documentation updated with examples

## Security Considerations

1. **Token limits**: Prevent sending too much history (configurable max)
2. **Data privacy**: History stored locally in SQLite, not sent to cloud
3. **PII protection**: System messages (with potential paths) excluded from AI context

## Backward Compatibility

- Existing `SendMessage()` and `SendMessageWithTools()` methods remain unchanged
- Old code continues to work without modification
- New functionality requires using new methods explicitly

## Future Enhancements

1. **Conversation summarization**: Summarize old messages to save tokens
2. **Selective memory**: Let users mark important messages to always include
3. **Export conversations**: Save complete conversations as markdown
4. **Conversation branching**: Fork conversations at any point

## References

- [Anthropic Messages API](https://docs.anthropic.com/en/api/messages)
- [OpenAI Chat Completions](https://platform.openai.com/docs/api-reference/chat)
- [Google Gemini Multi-turn Chat](https://ai.google.dev/gemini-api/docs/text-generation?lang=go#multi-turn)
