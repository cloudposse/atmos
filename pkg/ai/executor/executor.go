package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudposse/atmos/pkg/ai"
	"github.com/cloudposse/atmos/pkg/ai/formatter"
	"github.com/cloudposse/atmos/pkg/ai/memory"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// MaxToolIterations is the maximum number of tool execution loops to prevent infinite loops.
	MaxToolIterations = 10
)

// Executor handles non-interactive AI execution with tool support.
type Executor struct {
	client       ai.Client
	toolExecutor *tools.Executor
	atmosConfig  *schema.AtmosConfiguration
}

// NewExecutor creates a new non-interactive executor.
func NewExecutor(client ai.Client, toolExecutor *tools.Executor, atmosConfig *schema.AtmosConfiguration) *Executor {
	return &Executor{
		client:       client,
		toolExecutor: toolExecutor,
		atmosConfig:  atmosConfig,
	}
}

// Options contains execution options.
type Options struct {
	// Prompt is the user's prompt/question.
	Prompt string

	// ToolsEnabled indicates if tool execution is allowed.
	ToolsEnabled bool

	// SessionID is an optional session ID for conversation context.
	SessionID string

	// IncludeContext includes stack context in the prompt.
	IncludeContext bool
}

// Execute runs a single prompt and returns the formatted result.
func (e *Executor) Execute(ctx context.Context, opts Options) *formatter.ExecutionResult {
	startTime := time.Now()

	result := &formatter.ExecutionResult{
		Success: true,
		Metadata: formatter.ExecutionMetadata{
			Model:        e.client.GetModel(),
			Provider:     e.getProviderName(),
			SessionID:    opts.SessionID,
			Timestamp:    startTime,
			ToolsEnabled: opts.ToolsEnabled,
		},
	}

	// Prepare prompt with optional context.
	prompt := opts.Prompt
	if opts.IncludeContext {
		stackContext, err := ai.GatherStackContext(e.atmosConfig)
		if err == nil && stackContext != "" {
			prompt = fmt.Sprintf("%s\n\n%s", stackContext, opts.Prompt)
		}
	}

	// Execute with or without tools.
	if opts.ToolsEnabled {
		e.executeWithTools(ctx, prompt, result)
	} else {
		e.executeSimple(ctx, prompt, result)
	}

	// Calculate total duration.
	result.Metadata.DurationMs = time.Since(startTime).Milliseconds()

	return result
}

// executeSimple executes a prompt without tool support.
func (e *Executor) executeSimple(ctx context.Context, prompt string, result *formatter.ExecutionResult) {
	response, err := e.client.SendMessage(ctx, prompt)
	if err != nil {
		result.Success = false
		result.Error = &formatter.ErrorInfo{
			Message: err.Error(),
			Type:    "ai_error",
		}
		return
	}

	result.Response = response
}

// executeWithTools executes a prompt with tool support, handling multiple tool execution rounds.
func (e *Executor) executeWithTools(ctx context.Context, prompt string, result *formatter.ExecutionResult) {
	// Get available tools.
	availableTools := e.toolExecutor.ListTools()
	if len(availableTools) == 0 {
		// No tools available, fall back to simple execution.
		e.executeSimple(ctx, prompt, result)
		return
	}

	// Initialize conversation with user prompt.
	messages := []types.Message{
		{
			Role:    types.RoleUser,
			Content: prompt,
		},
	}

	// Load ATMOS.md content for caching (if available).
	var atmosMemory string
	if e.atmosConfig != nil && e.atmosConfig.Settings.AI.Memory.Enabled {
		// Try to load project memory for caching benefits.
		memConfig := &memory.Config{
			Enabled:      e.atmosConfig.Settings.AI.Memory.Enabled,
			FilePath:     e.atmosConfig.Settings.AI.Memory.FilePath,
			AutoUpdate:   e.atmosConfig.Settings.AI.Memory.AutoUpdate,
			CreateIfMiss: e.atmosConfig.Settings.AI.Memory.CreateIfMiss,
		}
		memoryMgr := memory.NewManager(e.atmosConfig.BasePath, memConfig)
		if memoryMgr != nil {
			_, _ = memoryMgr.Load(ctx) // Ignore error - it's OK if memory doesn't exist
			atmosMemory = memoryMgr.GetContext()
		}
	}

	// For non-interactive execution, we don't use agent system prompts.
	// Just use empty system prompt and ATMOS.md for caching.
	systemPrompt := ""

	// Tool execution loop (with iteration limit to prevent infinite loops).
	var accumulatedResponse string
	var totalUsage *types.Usage

	for iteration := 0; iteration < MaxToolIterations; iteration++ {
		// Call AI with tools and caching support.
		// Even without a custom system prompt, passing ATMOS.md enables caching for providers that support it.
		response, err := e.client.SendMessageWithSystemPromptAndTools(ctx, systemPrompt, atmosMemory, messages, availableTools)
		if err != nil {
			result.Success = false
			result.Error = &formatter.ErrorInfo{
				Message: err.Error(),
				Type:    "ai_error",
				Details: map[string]interface{}{
					"iteration": iteration,
				},
			}
			return
		}

		// Accumulate usage.
		totalUsage = combineUsage(totalUsage, response.Usage)

		// Check if AI wants to use tools.
		if response.StopReason == types.StopReasonToolUse && len(response.ToolCalls) > 0 {
			// Execute requested tools.
			toolResults := e.executeTools(ctx, response.ToolCalls, result)

			// Format tool results for AI.
			toolResultsText := formatToolResults(toolResults)

			// Add assistant's response (if any) to messages.
			if response.Content != "" {
				messages = append(messages, types.Message{
					Role:    types.RoleAssistant,
					Content: response.Content,
				})
				accumulatedResponse += response.Content + "\n\n"
			}

			// Add tool results as user message.
			messages = append(messages, types.Message{
				Role:    types.RoleUser,
				Content: fmt.Sprintf("Tool execution results:\n\n%s\n\nPlease provide your response based on these results.", toolResultsText),
			})

			// Continue loop to get AI's response to tool results.
			continue
		}

		// No more tool use - we have the final response.
		result.Response = accumulatedResponse + response.Content
		result.Metadata.StopReason = response.StopReason

		// Set token usage.
		if totalUsage != nil {
			result.Tokens = formatter.TokenUsage{
				Prompt:        totalUsage.InputTokens,
				Completion:    totalUsage.OutputTokens,
				Total:         totalUsage.TotalTokens,
				Cached:        totalUsage.CacheReadTokens,
				CacheCreation: totalUsage.CacheCreationTokens,
			}
		}

		return
	}

	// Exceeded max iterations.
	result.Success = false
	result.Error = &formatter.ErrorInfo{
		Message: fmt.Sprintf("exceeded maximum tool execution iterations (%d)", MaxToolIterations),
		Type:    "tool_error",
	}
}

// executeTools executes a batch of tool calls and records results.
func (e *Executor) executeTools(ctx context.Context, toolCalls []types.ToolCall, result *formatter.ExecutionResult) []formatter.ToolCallResult {
	results := make([]formatter.ToolCallResult, len(toolCalls))

	for i, call := range toolCalls {
		startTime := time.Now()

		toolResult, err := e.toolExecutor.Execute(ctx, call.Name, call.Input)

		results[i] = formatter.ToolCallResult{
			Tool:       call.Name,
			Args:       call.Input,
			DurationMs: time.Since(startTime).Milliseconds(),
		}

		if err != nil {
			results[i].Success = false
			results[i].Error = err.Error()
		} else if toolResult != nil {
			results[i].Success = toolResult.Success
			results[i].Result = toolResult.Data

			if toolResult.Error != nil {
				results[i].Error = toolResult.Error.Error()
			}
		}
	}

	// Append to result's tool calls.
	result.ToolCalls = append(result.ToolCalls, results...)

	return results
}

// formatToolResults formats tool execution results for the AI.
func formatToolResults(results []formatter.ToolCallResult) string {
	var text string
	for i, result := range results {
		text += fmt.Sprintf("Tool %d: %s\n", i+1, result.Tool)
		if result.Success {
			text += "Status: ✅ Success\n"
			text += fmt.Sprintf("Result: %v\n", result.Result)
		} else {
			text += "Status: ❌ Failed\n"
			text += fmt.Sprintf("Error: %s\n", result.Error)
		}
		text += "\n"
	}
	return text
}

// combineUsage combines two usage objects.
func combineUsage(a, b *types.Usage) *types.Usage {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}

	return &types.Usage{
		InputTokens:         a.InputTokens + b.InputTokens,
		OutputTokens:        a.OutputTokens + b.OutputTokens,
		TotalTokens:         a.TotalTokens + b.TotalTokens,
		CacheReadTokens:     a.CacheReadTokens + b.CacheReadTokens,
		CacheCreationTokens: a.CacheCreationTokens + b.CacheCreationTokens,
	}
}

// getProviderName returns the provider name from config.
func (e *Executor) getProviderName() string {
	if e.atmosConfig != nil && e.atmosConfig.Settings.AI.DefaultProvider != "" {
		return e.atmosConfig.Settings.AI.DefaultProvider
	}
	return "unknown"
}
