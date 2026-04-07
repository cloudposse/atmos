package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/cloudposse/atmos/pkg/ai"
	"github.com/cloudposse/atmos/pkg/ai/formatter"
	"github.com/cloudposse/atmos/pkg/ai/instructions"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// DefaultMaxToolIterations is the default maximum number of tool execution loops.
	DefaultMaxToolIterations = 25
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

// maxToolIterations returns the configured or default max tool iterations.
func (e *Executor) maxToolIterations() int {
	if e.atmosConfig != nil && e.atmosConfig.AI.MaxToolIterations > 0 {
		return e.atmosConfig.AI.MaxToolIterations
	}
	return DefaultMaxToolIterations
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

// loadAtmosInstructions loads ATMOS.md content for caching if available.
func (e *Executor) loadAtmosInstructions(ctx context.Context) string {
	if e.atmosConfig == nil || !e.atmosConfig.AI.Instructions.Enabled {
		return ""
	}

	memConfig := &instructions.Config{
		Enabled:  e.atmosConfig.AI.Instructions.Enabled,
		FilePath: e.atmosConfig.AI.Instructions.FilePath,
	}
	memoryMgr := instructions.NewManager(e.atmosConfig.BasePath, memConfig)
	if memoryMgr == nil {
		return ""
	}

	_, _ = memoryMgr.Load(ctx) // Ignore error - it's OK if instructions don't exist.
	return memoryMgr.GetContext()
}

// handleToolCalls executes tool calls and appends results to the message list.
func (e *Executor) handleToolCalls(
	ctx context.Context,
	response *types.Response,
	messages []types.Message,
	accumulatedResponse string,
	result *formatter.ExecutionResult,
) ([]types.Message, string) {
	toolResults := e.executeTools(ctx, response.ToolCalls, result)
	toolResultsText := formatToolResults(toolResults)

	if response.Content != "" {
		messages = append(messages, types.Message{
			Role:    types.RoleAssistant,
			Content: response.Content,
		})
		accumulatedResponse += response.Content + "\n\n"
	}

	messages = append(messages, types.Message{
		Role:    types.RoleUser,
		Content: fmt.Sprintf("Tool execution results:\n\n%s\n\nPlease provide your response based on these results.", toolResultsText),
	})

	return messages, accumulatedResponse
}

// setFinalResult sets the final response and token usage on the result.
func setFinalResult(result *formatter.ExecutionResult, accumulatedResponse string, response *types.Response, totalUsage *types.Usage) {
	result.Response = accumulatedResponse + response.Content
	result.Metadata.StopReason = response.StopReason

	if totalUsage == nil {
		return
	}

	result.Tokens = formatter.TokenUsage{
		Prompt:        totalUsage.InputTokens,
		Completion:    totalUsage.OutputTokens,
		Total:         totalUsage.TotalTokens,
		Cached:        totalUsage.CacheReadTokens,
		CacheCreation: totalUsage.CacheCreationTokens,
	}
}

// toolSystemPrompt is the system prompt guiding the AI to prefer specific tools.
const toolSystemPrompt = `You are an AI assistant for Atmos infrastructure management with access to tools.

Prefer specific tools over generic ones:
- Use atmos_list_stacks to list stacks (not execute_atmos_command)
- Use atmos_describe_component to describe components (not execute_atmos_command)
- Use read_file, read_stack_file, read_component_file for reading files
- Use search_files for searching
- Only use execute_atmos_command for commands that don't have a dedicated tool

Always use tools when needed rather than describing what you would do.`

// executeWithTools executes a prompt with tool support, handling multiple tool execution rounds.
func (e *Executor) executeWithTools(ctx context.Context, prompt string, result *formatter.ExecutionResult) {
	availableTools := e.toolExecutor.ListTools()
	if len(availableTools) == 0 {
		e.executeSimple(ctx, prompt, result)
		return
	}

	messages := []types.Message{
		{Role: types.RoleUser, Content: prompt},
	}

	atmosMemory := e.loadAtmosInstructions(ctx)

	var accumulatedResponse string
	var totalUsage *types.Usage

	maxIter := e.maxToolIterations()
	for iteration := 0; iteration < maxIter; iteration++ {
		response, err := e.client.SendMessageWithSystemPromptAndTools(ctx, toolSystemPrompt, atmosMemory, messages, availableTools)
		if err != nil {
			result.Success = false
			result.Error = &formatter.ErrorInfo{
				Message: err.Error(),
				Type:    "ai_error",
				Details: map[string]interface{}{"iteration": iteration},
			}
			return
		}

		totalUsage = combineUsage(totalUsage, response.Usage)

		if response.StopReason == types.StopReasonToolUse && len(response.ToolCalls) > 0 {
			messages, accumulatedResponse = e.handleToolCalls(ctx, response, messages, accumulatedResponse, result)
			continue
		}

		setFinalResult(result, accumulatedResponse, response, totalUsage)
		return
	}

	result.Success = false
	result.Error = &formatter.ErrorInfo{
		Message: fmt.Sprintf("exceeded maximum tool execution iterations (%d)", maxIter),
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
			Tool:        call.Name,
			DisplayName: e.toolExecutor.DisplayName(call.Name),
			Args:        call.Input,
			DurationMs:  time.Since(startTime).Milliseconds(),
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
	if e.atmosConfig != nil && e.atmosConfig.AI.DefaultProvider != "" {
		return e.atmosConfig.AI.DefaultProvider
	}
	return "unknown"
}
