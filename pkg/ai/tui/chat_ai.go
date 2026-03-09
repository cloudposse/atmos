package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	aiTypes "github.com/cloudposse/atmos/pkg/ai/types"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// statusMsg updates the loading status text.
type statusMsg string

// aiRequestCtx bundles the common parameters for AI request handling.
type aiRequestCtx struct {
	ctx                context.Context
	messages           []aiTypes.Message
	availableTools     []tools.Tool
	systemPrompt       string
	atmosMemory        string
	accumulatedContent string
	resultText         string
}

func (m *ChatModel) sendMessage(content string) tea.Cmd {
	return func() tea.Msg {
		return sendMessageMsg(content)
	}
}

// defaultSystemPrompt is the default system prompt used when no skill-specific prompt is configured.
const defaultSystemPrompt = `You are an AI assistant for Atmos infrastructure management. You have access to tools that allow you to perform actions.

IMPORTANT: When you need to perform an action (read files, edit files, search, execute commands, etc.), you MUST use the available tools. Do NOT just describe what you would do - actually use the tools to do it.

For example:
- If you need to read a file, use the read_file tool immediately
- If you need to edit a file, use the edit_file tool immediately
- If you need to search for files, use the search_files tool immediately
- If you need to execute an Atmos command, use the execute_atmos_command tool immediately

Always take action using tools rather than describing what action you would take.`

// buildFilteredMessages builds message history filtered by the current provider.
// Only includes messages from the current provider session for complete isolation.
func (m *ChatModel) buildFilteredMessages() []aiTypes.Message {
	currentProvider := ""
	if m.sess != nil {
		currentProvider = m.sess.Provider
	}

	messages := make([]aiTypes.Message, 0, len(m.messages)+1)
	for _, msg := range m.messages {
		// Skip system messages (UI-only notifications).
		if msg.Role == roleSystem {
			continue
		}

		// Include only messages from the current provider session.
		// This provides complete conversation isolation when switching providers.
		if msg.Provider == currentProvider {
			messages = append(messages, aiTypes.Message{
				Role:    msg.Role,
				Content: msg.Content,
			})
		}
	}

	return messages
}

// applyHistoryLimits applies sliding window limits (message-based and token-based) to conversation history.
// This helps prevent rate limiting and reduces token usage for long conversations.
func (m *ChatModel) applyHistoryLimits(messages []aiTypes.Message) []aiTypes.Message {
	pruneIndex := 0 // Start of messages to keep (0 = keep all).

	// Apply message-based limit if configured.
	if m.maxHistoryMessages > 0 && len(messages) > m.maxHistoryMessages {
		pruneIndex = len(messages) - m.maxHistoryMessages
	}

	// Apply token-based limit if configured.
	// Count backwards from most recent message and stop when token limit is exceeded.
	if m.maxHistoryTokens > 0 {
		totalTokens := 0
		tokenPruneIndex := len(messages)

		// Count backwards from most recent.
		for i := len(messages) - 1; i >= 0; i-- {
			msgTokens := estimateTokens(messages[i].Content)
			if totalTokens+msgTokens > m.maxHistoryTokens {
				tokenPruneIndex = i + 1 // Keep from i+1 onwards.
				break
			}
			totalTokens += msgTokens
		}

		// Use whichever prune index is more restrictive (further right/more pruning).
		if tokenPruneIndex > pruneIndex {
			pruneIndex = tokenPruneIndex
		}
	}

	// Apply the pruning if needed.
	if pruneIndex > 0 && pruneIndex < len(messages) {
		messages = messages[pruneIndex:]
	}

	return messages
}

// prependMemoryContext prepends a system message with ATMOS.md context if available.
func (m *ChatModel) prependMemoryContext(messages []aiTypes.Message) []aiTypes.Message {
	if m.memoryMgr == nil {
		return messages
	}

	memoryContext := m.memoryMgr.GetContext()
	if memoryContext == "" {
		return messages
	}

	return append([]aiTypes.Message{{
		Role:    aiTypes.RoleSystem,
		Content: memoryContext,
	}}, messages...)
}

// buildSystemPrompt constructs the system prompt from the current skill and skill registry.
func (m *ChatModel) buildSystemPrompt() string {
	systemPrompt := defaultSystemPrompt

	if m.currentSkill != nil && m.currentSkill.SystemPrompt != "" {
		systemPrompt = m.currentSkill.SystemPrompt
	}

	// Append available skills XML to system prompt (Agent Skills integration guide).
	// This helps the model understand what skills are available and their purposes.
	if m.skillRegistry != nil {
		currentSkillName := ""
		if m.currentSkill != nil {
			currentSkillName = m.currentSkill.Name
		}
		skillsXML := m.skillRegistry.ToPromptXML(currentSkillName)
		if skillsXML != "" {
			systemPrompt = systemPrompt + doubleNewline + skillsXML
		}
	}

	return systemPrompt
}

// getAtmosMemory returns the ATMOS.md content for prompt caching.
func (m *ChatModel) getAtmosMemory() string {
	if m.memoryMgr == nil {
		return ""
	}
	return m.memoryMgr.GetContext()
}

// handleNoToolsResponse handles the AI response path when no tools are available.
func (m *ChatModel) handleNoToolsResponse(ctx context.Context, messages []aiTypes.Message) tea.Msg {
	response, err := m.client.SendMessageWithHistory(ctx, messages)
	if err != nil {
		return aiErrorMsg(formatAPIError(err))
	}

	return aiResponseMsg{content: response, usage: nil}
}

// handleActionIntentRetry prompts the AI to use tools when it expressed intent but did not.
func (m *ChatModel) handleActionIntentRetry(reqCtx *aiRequestCtx, response *aiTypes.Response) tea.Msg {
	reqCtx.messages = append(reqCtx.messages, aiTypes.Message{
		Role:    aiTypes.RoleAssistant,
		Content: response.Content,
	})
	reqCtx.messages = append(reqCtx.messages, aiTypes.Message{
		Role:    aiTypes.RoleUser,
		Content: "Please use the available tools to perform that action now, rather than just describing what you would do.",
	})

	// Send the prompt again with caching.
	retryResponse, err := m.client.SendMessageWithSystemPromptAndTools(reqCtx.ctx, reqCtx.systemPrompt, reqCtx.atmosMemory, reqCtx.messages, reqCtx.availableTools)
	if err != nil {
		return aiErrorMsg(formatAPIError(err))
	}

	// Check if AI now uses tools.
	if retryResponse.StopReason == aiTypes.StopReasonToolUse && len(retryResponse.ToolCalls) > 0 {
		return m.handleToolExecutionFlow(reqCtx.ctx, retryResponse, reqCtx.messages, reqCtx.availableTools)
	}

	// Handle empty retry response.
	if retryResponse == nil || retryResponse.Content == "" {
		return aiResponseMsg{content: response.Content, usage: response.Usage}
	}

	// If still no tool use after retry, combine both responses.
	combinedContent := response.Content
	if retryResponse.Content != "" {
		if combinedContent != "" {
			combinedContent += doubleNewline
		}
		combinedContent += retryResponse.Content
	}
	return aiResponseMsg{content: combinedContent, usage: combineUsage(response.Usage, retryResponse.Usage)}
}

// handleToolsResponse handles the AI response path when tools are available.
func (m *ChatModel) handleToolsResponse(ctx context.Context, messages []aiTypes.Message, availableTools []tools.Tool) tea.Msg {
	reqCtx := &aiRequestCtx{
		ctx:            ctx,
		messages:       messages,
		availableTools: availableTools,
		systemPrompt:   m.buildSystemPrompt(),
		atmosMemory:    m.getAtmosMemory(),
	}

	// Send messages with system prompt and tools (enables caching).
	response, err := m.client.SendMessageWithSystemPromptAndTools(ctx, reqCtx.systemPrompt, reqCtx.atmosMemory, messages, availableTools)
	if err != nil {
		return aiErrorMsg(formatAPIError(err))
	}

	// Handle empty initial response.
	if response == nil {
		return aiErrorMsg("Received nil response from AI provider")
	}

	// Check if AI wants to use tools.
	if response.StopReason == aiTypes.StopReasonToolUse && len(response.ToolCalls) > 0 {
		return m.handleToolExecutionFlow(ctx, response, messages, availableTools)
	}

	// No tool use - check if AI expressed intent to take action but did not use tools.
	if detectActionIntent(response.Content) {
		return m.handleActionIntentRetry(reqCtx, response)
	}

	// No action intent detected, return the text response.
	return aiResponseMsg{content: response.Content, usage: response.Usage}
}

func (m *ChatModel) getAIResponseWithContext(userMessage string, ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		// Check if context is already cancelled before starting.
		if ctx.Err() != nil {
			return aiErrorMsg("Request cancelled")
		}

		// Build message history filtered by provider.
		messages := m.buildFilteredMessages()

		// Apply sliding window to limit conversation history if configured.
		messages = m.applyHistoryLimits(messages)

		// Add current user message.
		messages = append(messages, aiTypes.Message{
			Role:    aiTypes.RoleUser,
			Content: userMessage,
		})

		// Apply instructions context if available by prepending a system message.
		messages = m.prependMemoryContext(messages)

		// Check if tools are available.
		var availableTools []tools.Tool
		if m.executor != nil {
			availableTools = m.executor.ListTools()
		}

		// Use tool calling if tools are available.
		if len(availableTools) > 0 {
			return m.handleToolsResponse(ctx, messages, availableTools)
		}

		// Fallback to message with history but no tools.
		return m.handleNoToolsResponse(ctx, messages)
	}
}

// executeToolCalls executes tool calls and returns the results.
func (m *ChatModel) executeToolCalls(ctx context.Context, toolCalls []aiTypes.ToolCall) []*tools.Result {
	results := make([]*tools.Result, len(toolCalls))

	for i, toolCall := range toolCalls {
		log.Debugf("Executing tool: %s with params: %v", toolCall.Name, toolCall.Input)

		// Update UI status if possible.
		if m.program != nil {
			m.program.Send(statusMsg(fmt.Sprintf("Executing tool: %s...", toolCall.Name)))
		}

		// Execute the tool.
		result, err := m.executor.Execute(ctx, toolCall.Name, toolCall.Input)
		if err != nil {
			results[i] = &tools.Result{
				Success: false,
				Output:  fmt.Sprintf("Error: %v", err),
				Error:   err,
			}
		} else {
			results[i] = result
		}
	}

	return results
}

// handleToolExecutionFlow executes tools, sends results back to AI, and returns the combined response.
func (m *ChatModel) handleToolExecutionFlow(ctx context.Context, response *aiTypes.Response, messages []aiTypes.Message, availableTools []tools.Tool) tea.Msg {
	return m.handleToolExecutionFlowWithAccumulator(ctx, response, messages, availableTools, "")
}

// buildToolDisplayText builds the display output showing tool execution results for the user.
func buildToolDisplayText(response *aiTypes.Response, toolResults []*tools.Result) string {
	var resultText string
	if response.Content != "" {
		resultText = response.Content + doubleNewline
	}

	for i, result := range toolResults {
		if i > 0 {
			resultText += doubleNewline
		}

		displayOutput := resolveToolOutput(result)

		// Build tool header with name and parameters.
		toolHeader := fmt.Sprintf("**Tool:** `%s`", response.ToolCalls[i].Name)

		// Show the actual command/parameters being executed for better visibility.
		if toolParams := formatToolParameters(response.ToolCalls[i]); toolParams != "" {
			toolHeader += newlineChar + toolParams
		}

		// Detect output format and wrap in appropriate code block for syntax highlighting.
		format := detectOutputFormat(displayOutput)
		resultText += fmt.Sprintf("%s\n\n```%s\n%s\n```", toolHeader, format, displayOutput)
	}

	return resultText
}

// buildToolResultsContent builds the tool results content string to send back to the AI.
func buildToolResultsContent(response *aiTypes.Response, toolResults []*tools.Result) string {
	var toolResultsContent string
	for i, result := range toolResults {
		if i > 0 {
			toolResultsContent += doubleNewline
		}

		toolOutput := resolveToolOutput(result)
		toolResultsContent += fmt.Sprintf("Tool: %s\nResult:\n%s", response.ToolCalls[i].Name, toolOutput)
	}
	return toolResultsContent
}

// resolveToolOutput determines the output string from a tool result.
func resolveToolOutput(result *tools.Result) string {
	displayOutput := result.Output
	if displayOutput == "" && result.Error != nil {
		displayOutput = fmt.Sprintf("Error: %v", result.Error)
	}
	if displayOutput == "" {
		displayOutput = "No output returned"
	}
	return displayOutput
}

// appendAccumulated prepends accumulated content with a separator if non-empty.
func appendAccumulated(accumulated, newContent string) string {
	if accumulated != "" {
		return accumulated + doubleNewline + newContent
	}
	return newContent
}

// handleEmptyFollowUpResponse handles when the AI's follow-up response is empty or nil.
func handleEmptyFollowUpResponse(accumulated, resultText string, response *aiTypes.Response) tea.Msg {
	combinedResponse := appendAccumulated(accumulated, resultText+markdownSeparator+"*Note: AI response was empty. This might indicate rate limiting or a timeout.*")
	return aiResponseMsg{content: combinedResponse, usage: response.Usage}
}

// executeToolsAndBuildMessages runs tools, appends results to conversation, and returns display text.
func (m *ChatModel) executeToolsAndBuildMessages(ctx context.Context, response *aiTypes.Response, messages *[]aiTypes.Message) string {
	toolResults := m.executeToolCalls(ctx, response.ToolCalls)
	resultText := buildToolDisplayText(response, toolResults)
	toolResultsContent := buildToolResultsContent(response, toolResults)

	if response.Content != "" {
		*messages = append(*messages, aiTypes.Message{Role: aiTypes.RoleAssistant, Content: response.Content})
	}
	*messages = append(*messages, aiTypes.Message{
		Role:    aiTypes.RoleUser,
		Content: fmt.Sprintf("Tool execution results:\n\n%s\n\nPlease provide your final response based on these results.", toolResultsContent),
	})

	return resultText
}

// handleToolExecutionFlowWithAccumulator executes tools, sends results back to AI, and returns the combined response.
// The accumulatedContent parameter preserves intermediate AI thinking across recursive tool calls.
func (m *ChatModel) handleToolExecutionFlowWithAccumulator(ctx context.Context, response *aiTypes.Response, messages []aiTypes.Message, availableTools []tools.Tool, accumulatedContent string) tea.Msg {
	resultText := m.executeToolsAndBuildMessages(ctx, response, &messages)

	// Get system prompt and ATMOS.md for caching.
	systemPrompt := m.buildSystemPrompt()
	atmosMemory := m.getAtmosMemory()

	// Call AI again with tool results to get final response (with caching).
	finalResponse, err := m.client.SendMessageWithSystemPromptAndTools(ctx, systemPrompt, atmosMemory, messages, availableTools)
	if err != nil {
		return aiErrorMsg(formatAPIError(err))
	}

	// Handle empty or truncated response from AI.
	if finalResponse == nil || (finalResponse.Content == "" && finalResponse.StopReason != aiTypes.StopReasonToolUse) {
		return handleEmptyFollowUpResponse(accumulatedContent, resultText, finalResponse)
	}

	// Check if the final response wants to use more tools.
	if finalResponse.StopReason == aiTypes.StopReasonToolUse && len(finalResponse.ToolCalls) > 0 {
		newAccumulated := appendAccumulated(accumulatedContent, resultText)
		return m.handleToolExecutionFlowWithAccumulator(ctx, finalResponse, messages, availableTools, newAccumulated)
	}

	// Check if AI expressed intent to take more action in the final response.
	if detectActionIntent(finalResponse.Content) {
		reqCtx := &aiRequestCtx{
			ctx:                ctx,
			messages:           messages,
			availableTools:     availableTools,
			systemPrompt:       systemPrompt,
			atmosMemory:        atmosMemory,
			accumulatedContent: accumulatedContent,
			resultText:         resultText,
		}
		return m.handleFollowUpActionIntent(reqCtx, finalResponse)
	}

	// Combine accumulated content + tool execution display + final AI response.
	combinedResponse := appendAccumulated(accumulatedContent, resultText+markdownSeparator+finalResponse.Content)

	// Return the combined response (accumulated + tool results + AI's final analysis).
	return aiResponseMsg{content: combinedResponse, usage: finalResponse.Usage}
}

// handleFollowUpActionIntent handles when the AI expresses intent to take action in a follow-up response.
func (m *ChatModel) handleFollowUpActionIntent(reqCtx *aiRequestCtx, finalResponse *aiTypes.Response) tea.Msg {
	// AI said it would do something else but did not use tools. Prompt it again.
	reqCtx.messages = append(reqCtx.messages, aiTypes.Message{
		Role:    aiTypes.RoleAssistant,
		Content: finalResponse.Content,
	})
	reqCtx.messages = append(reqCtx.messages, aiTypes.Message{
		Role:    aiTypes.RoleUser,
		Content: "Please use the available tools to perform that action now, rather than just describing what you would do.",
	})

	// Retry with the prompt (with caching).
	retryResponse, err := m.client.SendMessageWithSystemPromptAndTools(reqCtx.ctx, reqCtx.systemPrompt, reqCtx.atmosMemory, reqCtx.messages, reqCtx.availableTools)
	if err != nil {
		return aiErrorMsg(formatAPIError(err))
	}

	// Handle empty retry response.
	if retryResponse == nil || (retryResponse.Content == "" && retryResponse.StopReason != aiTypes.StopReasonToolUse) {
		combinedResponse := appendAccumulated(reqCtx.accumulatedContent, reqCtx.resultText+markdownSeparator+finalResponse.Content+doubleNewline+"*Note: AI retry response was empty.*")
		return aiResponseMsg{content: combinedResponse, usage: combineUsage(finalResponse.Usage, retryResponse.Usage)}
	}

	// Check if AI now uses tools.
	if retryResponse.StopReason == aiTypes.StopReasonToolUse && len(retryResponse.ToolCalls) > 0 {
		newAccumulated := appendAccumulated(reqCtx.accumulatedContent, reqCtx.resultText)
		return m.handleToolExecutionFlowWithAccumulator(reqCtx.ctx, retryResponse, reqCtx.messages, reqCtx.availableTools, newAccumulated)
	}

	// If still no tool use, combine all responses.
	combinedResponse := appendAccumulated(reqCtx.accumulatedContent, reqCtx.resultText+markdownSeparator+finalResponse.Content+doubleNewline+retryResponse.Content)
	return aiResponseMsg{content: combinedResponse, usage: combineUsage(finalResponse.Usage, retryResponse.Usage)}
}
