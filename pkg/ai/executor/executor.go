package executor

import (
	"context"
	"errors"
	"fmt"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
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

	// History contains prior conversation messages to prepend before Prompt.
	// Callers resuming a persisted session (see cmd/ai/session_helpers.go)
	// populate this from the session's stored messages; it is empty for a
	// fresh, session-less execution.
	History []types.Message
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
		e.executeWithTools(ctx, prompt, opts.History, result)
	} else {
		e.executeSimple(ctx, prompt, opts.History, result)
	}

	// Calculate total duration.
	result.Metadata.DurationMs = time.Since(startTime).Milliseconds()

	return result
}

// executeSimple executes a prompt without tool support. When history is
// non-empty (a resumed session), it is sent along with the prompt so the
// model has the prior conversation as context.
func (e *Executor) executeSimple(ctx context.Context, prompt string, history []types.Message, result *formatter.ExecutionResult) {
	var response string
	var err error
	if len(history) > 0 {
		messages := make([]types.Message, 0, len(history)+1)
		messages = append(messages, history...)
		messages = append(messages, types.Message{Role: types.RoleUser, Content: prompt})
		response, err = e.client.SendMessageWithHistory(ctx, messages)
	} else {
		response, err = e.client.SendMessage(ctx, prompt)
	}
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
// The returned error is non-nil only for an infrastructure-level tool failure
// (see isInfrastructureToolError) that the caller should treat as fatal rather
// than feeding back to the model for another round.
func (e *Executor) handleToolCalls(
	ctx context.Context,
	response *types.Response,
	messages []types.Message,
	accumulatedResponse string,
	result *formatter.ExecutionResult,
) ([]types.Message, string, error) {
	toolResults, infraErr := e.executeTools(ctx, response.ToolCalls, result)
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

	return messages, accumulatedResponse, infraErr
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

Always use tools when needed rather than describing what you would do.

If atmos_list_stacks returns zero stacks, this is a new Atmos project that doesn't have any
stacks written yet. Treat this as an opportunity, not an error: proactively offer to help the
user create their first stack and component rather than just reporting that none exist.`

// executeWithTools executes a prompt with tool support, handling multiple tool execution rounds.
// When history is non-empty (a resumed session), it is prepended to the conversation.
func (e *Executor) executeWithTools(ctx context.Context, prompt string, history []types.Message, result *formatter.ExecutionResult) {
	availableTools := e.toolExecutor.ListTools()
	if len(availableTools) == 0 {
		e.executeSimple(ctx, prompt, history, result)
		return
	}

	messages := make([]types.Message, 0, len(history)+1)
	messages = append(messages, history...)
	messages = append(messages, types.Message{Role: types.RoleUser, Content: prompt})

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
			var infraErr error
			messages, accumulatedResponse, infraErr = e.handleToolCalls(ctx, response, messages, accumulatedResponse, result)
			if infraErr != nil {
				// Infrastructure-level failure (e.g. an unregistered tool name): the
				// model can't meaningfully retry around this, so stop immediately
				// instead of waiting for MaxToolIterations to exhaust.
				result.Success = false
				result.Error = &formatter.ErrorInfo{
					Message: infraErr.Error(),
					Type:    "tool_error",
				}
				return
			}
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

// executeTools executes a batch of tool calls and records results. It returns
// the per-call results and, if any call failed at the infrastructure level
// (e.g. an unregistered tool name), the first such error — distinct from an
// application-level error the model can see and potentially recover from.
func (e *Executor) executeTools(ctx context.Context, toolCalls []types.ToolCall, result *formatter.ExecutionResult) ([]formatter.ToolCallResult, error) {
	results := make([]formatter.ToolCallResult, len(toolCalls))
	var infraErr error

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
			if infraErr == nil && isInfrastructureToolError(err) {
				infraErr = fmt.Errorf("tool %q: %w", call.Name, err)
			}
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

	return results, infraErr
}

// isInfrastructureToolError reports whether err represents a failure to even
// invoke the requested tool — e.g. the model asked for a tool name that isn't
// registered — rather than an application-level error the tool itself
// returned. The model can meaningfully retry around an application-level
// error (it sees the failure and can adjust its next call); it cannot retry
// around an infrastructure-level one, so callers should stop immediately
// instead of exhausting MaxToolIterations.
func isInfrastructureToolError(err error) bool {
	return errors.Is(err, errUtils.ErrAIToolNotFound)
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
