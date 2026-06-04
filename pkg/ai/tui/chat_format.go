package tui

import (
	"fmt"
	"strings"

	aiTypes "github.com/cloudposse/atmos/pkg/ai/types"
)

// detectOutputFormat detects the format of command output and returns the appropriate markdown language tag.
func detectOutputFormat(output string) string {
	trimmed := strings.TrimSpace(output)

	if isJSON(trimmed) {
		return "json"
	}

	lines := strings.Split(trimmed, newlineChar)

	if isYAML(lines) {
		return "yaml"
	}

	if isHCL(trimmed) {
		return "hcl"
	}

	if isTableFormat(lines) {
		return "text" // Tables render better as text in glamour.
	}

	return "text"
}

// isJSON checks whether output looks like JSON.
func isJSON(trimmed string) bool {
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

// isYAML checks whether output looks like YAML by counting key-value patterns.
func isYAML(lines []string) bool {
	yamlPatterns := 0
	for i, line := range lines {
		if i > yamlDetectionMaxLines {
			break
		}
		trimmedLine := strings.TrimSpace(line)
		if strings.Contains(trimmedLine, ": ") || strings.HasPrefix(trimmedLine, "- ") {
			yamlPatterns++
		}
	}
	return yamlPatterns >= 3
}

// isHCL checks whether output looks like HCL/Terraform.
func isHCL(trimmed string) bool {
	return strings.Contains(trimmed, "resource \"") || strings.Contains(trimmed, "data \"") ||
		strings.Contains(trimmed, "module \"") || strings.Contains(trimmed, "variable \"")
}

// isTableFormat checks whether output looks like a table with consistent pipe separators.
func isTableFormat(lines []string) bool {
	if len(lines) <= 2 {
		return false
	}

	pipeCount := strings.Count(lines[0], pipeChar)
	if pipeCount <= 1 {
		return false
	}

	checkLines := tablePipeCheckMaxLines
	if len(lines) < checkLines {
		checkLines = len(lines)
	}

	for i := 1; i < checkLines; i++ {
		if strings.Count(lines[i], pipeChar) != pipeCount {
			return false
		}
	}

	return true
}

// combineUsage combines two Usage structs by adding their token counts.
func combineUsage(u1, u2 *aiTypes.Usage) *aiTypes.Usage {
	if u1 == nil && u2 == nil {
		return nil
	}
	if u1 == nil {
		return u2
	}
	if u2 == nil {
		return u1
	}

	return &aiTypes.Usage{
		InputTokens:         u1.InputTokens + u2.InputTokens,
		OutputTokens:        u1.OutputTokens + u2.OutputTokens,
		TotalTokens:         u1.TotalTokens + u2.TotalTokens,
		CacheReadTokens:     u1.CacheReadTokens + u2.CacheReadTokens,
		CacheCreationTokens: u1.CacheCreationTokens + u2.CacheCreationTokens,
	}
}

// formatTokenCount formats a token count into a human-readable string (e.g., "7.1k").
func formatTokenCount(count int64) string {
	if count == 0 {
		return "0"
	}
	if count < tokensPerK {
		return fmt.Sprintf("%d", count)
	}
	if count < tokensPerM {
		// Format as k (thousands) with one decimal place.
		k := float64(count) / tokensPerKF
		if k < tokenDisplayThreshold {
			return fmt.Sprintf("%.1fk", k)
		}
		return fmt.Sprintf("%.0fk", k)
	}
	// Format as M (millions) with one decimal place.
	m := float64(count) / tokensPerMF
	if m < tokenDisplayThreshold {
		return fmt.Sprintf("%.1fM", m)
	}
	return fmt.Sprintf("%.0fM", m)
}

// formatUsage formats usage information for display.
func formatUsage(usage *aiTypes.Usage) string {
	if usage == nil || usage.TotalTokens == 0 {
		return ""
	}

	var parts []string

	// Show input/output breakdown.
	if usage.InputTokens > 0 {
		parts = append(parts, fmt.Sprintf("↑ %s", formatTokenCount(usage.InputTokens)))
	}
	if usage.OutputTokens > 0 {
		parts = append(parts, fmt.Sprintf("↓ %s", formatTokenCount(usage.OutputTokens)))
	}

	// Show cache info if available.
	if usage.CacheReadTokens > 0 {
		parts = append(parts, fmt.Sprintf("cache: %s", formatTokenCount(usage.CacheReadTokens)))
	}

	return strings.Join(parts, " · ")
}

// estimateTokens provides an approximate token count for text using a heuristic approach.
// This uses a simple word-based estimation: tokens ≈ words × 1.3
// This is accurate enough (±10-20%) for rate limit prevention without requiring
// provider-specific tokenizers. More sophisticated approaches could use:
//   - characters / 4 (simpler but less accurate)
//   - word count × 1.5 (more conservative)
//   - tiktoken library (accurate but adds 15MB+ dependency and only works for OpenAI)
func estimateTokens(text string) int {
	if text == "" {
		return 0
	}

	// Count words (split on whitespace).
	words := strings.Fields(text)
	wordCount := len(words)

	// Apply multiplier to estimate tokens.
	// Based on empirical observation:
	// - English text: ~1.3 tokens per word on average
	// - Code: ~1.5 tokens per word (more punctuation)
	// - Technical text: ~1.4 tokens per word
	// We use 1.3 as a reasonable middle ground.
	estimatedTokens := float64(wordCount) * tokenEstimationMultiplier

	return int(estimatedTokens)
}

// apiErrorMapping defines a known error pattern and its user-friendly message.
type apiErrorMapping struct {
	patterns []string
	message  string
}

// knownAPIErrors lists recognized API error patterns in priority order.
var knownAPIErrors = []apiErrorMapping{
	{
		patterns: []string{"Function calling is not enabled"},
		message:  "This model doesn't support function calling (tool use). Please switch to a different model using Ctrl+P. Try gemini-2.5-flash or gemini-1.5-pro.",
	},
	{
		patterns: []string{"429", "Too Many Requests", "rate_limit_error"},
		message:  "Rate limit exceeded. Please wait a moment and try again, or contact your provider to increase your rate limit.",
	},
	{
		patterns: []string{"401", "Unauthorized", "authentication_error"},
		message:  "Authentication failed. Please check your API key configuration.",
	},
	{
		patterns: []string{"403", "Forbidden", "permission_error"},
		message:  "Permission denied. Your API key may not have access to this model or feature.",
	},
	{
		patterns: []string{"404", "Not Found", "model not found"},
		message:  "Model not found. Please check your model configuration.",
	},
	{
		patterns: []string{"timeout", "deadline exceeded"},
		message:  "Request timed out. The AI provider took too long to respond. Please try again.",
	},
	{
		patterns: []string{"context_length_exceeded", "maximum context length"},
		message:  "Context length exceeded. Your conversation is too long. Try starting a new session.",
	},
}

// matchKnownAPIError checks if the error string matches any known API error pattern.
func matchKnownAPIError(errStr string) (string, bool) {
	for _, mapping := range knownAPIErrors {
		for _, pattern := range mapping.patterns {
			if strings.Contains(errStr, pattern) {
				return mapping.message, true
			}
		}
	}

	// Special case: function calling with compound condition.
	if strings.Contains(errStr, "function calling") &&
		(strings.Contains(errStr, "not enabled") || strings.Contains(errStr, "not supported")) {
		return knownAPIErrors[0].message, true
	}

	return "", false
}

// cleanupErrorString removes technical details from an error string.
func cleanupErrorString(errStr string) string {
	// Remove request IDs.
	for _, prefix := range []string{"(Request-ID:", "(request-id:"} {
		if idx := strings.Index(errStr, prefix); idx != -1 {
			errStr = strings.TrimSpace(errStr[:idx])
		}
	}

	// Remove JSON response bodies.
	if idx := strings.Index(errStr, `{"type":"error"`); idx != -1 {
		errStr = strings.TrimSpace(errStr[:idx])
	}

	// Remove HTTP method and URL from error messages.
	errStr = stripHTTPMethodPrefix(errStr)

	// Clean up nested error prefixes.
	for _, prefix := range []string{
		"failed to send message: ",
		"failed to send message with tools: ",
		"failed to send messages with history: ",
		"failed to send messages with history and tools: ",
	} {
		errStr = strings.TrimPrefix(errStr, prefix)
	}

	return errStr
}

// stripHTTPMethodPrefix removes HTTP method and URL prefix from error strings.
func stripHTTPMethodPrefix(errStr string) string {
	httpMethods := []string{"POST ", "GET ", "PUT ", "DELETE "}
	for _, method := range httpMethods {
		if !strings.HasPrefix(errStr, method) {
			continue
		}
		if idx := strings.Index(errStr, `":`); idx != -1 {
			return strings.TrimSpace(errStr[idx+2:])
		}
	}
	return errStr
}

// formatAPIError formats API errors in a user-friendly way.
func formatAPIError(err error) string {
	if err == nil {
		return ""
	}

	errStr := err.Error()

	// Check for known error patterns first.
	if msg, found := matchKnownAPIError(errStr); found {
		return msg
	}

	// For other errors, clean up the message by removing technical details.
	return cleanupErrorString(errStr)
}

// formatToolParameters formats tool call parameters for display in the UI.
func formatToolParameters(toolCall aiTypes.ToolCall) string {
	if len(toolCall.Input) == 0 {
		return ""
	}

	// Try special formatting for known tools first.
	if result := formatKnownToolParams(toolCall); result != "" {
		return result
	}

	// For other tools, show all parameters in a generic format.
	return formatGenericToolParams(toolCall.Input)
}

// formatKnownToolParams returns a formatted string for well-known tool types, or empty string.
func formatKnownToolParams(toolCall aiTypes.ToolCall) string {
	switch toolCall.Name {
	case "execute_atmos_command":
		return formatInputField(toolCall.Input, "command", "Command", "atmos ")
	case "read_file", "read_component_file", "read_stack_file":
		return formatPathOrComponent(toolCall.Input)
	case "edit_file", "write_component_file", "write_stack_file":
		return formatPathOrComponent(toolCall.Input)
	case "search_files":
		return formatInputField(toolCall.Input, "pattern", "Pattern", "")
	case "execute_bash":
		return formatBashCommand(toolCall.Input)
	case "describe_component":
		return formatDescribeComponentParams(toolCall.Input)
	default:
		return ""
	}
}

// formatInputField formats a single input field with a label prefix.
func formatInputField(input map[string]interface{}, field, label, prefix string) string {
	if val, ok := input[field].(string); ok {
		return fmt.Sprintf("**%s:** `%s%s`", label, prefix, val)
	}
	return ""
}

// formatPathOrComponent formats path or component from input.
func formatPathOrComponent(input map[string]interface{}) string {
	if path, ok := input["path"].(string); ok {
		return fmt.Sprintf("**Path:** `%s`", path)
	}
	if component, ok := input["component"].(string); ok {
		return fmt.Sprintf("**Component:** `%s`", component)
	}
	return ""
}

// formatBashCommand formats a bash command with truncation.
func formatBashCommand(input map[string]interface{}) string {
	command, ok := input["command"].(string)
	if !ok {
		return ""
	}
	if len(command) > commandDisplayMaxLen {
		command = command[:commandTruncatedLen] + "..."
	}
	return fmt.Sprintf("**Command:** `%s`", command)
}

// formatDescribeComponentParams formats describe_component parameters.
func formatDescribeComponentParams(input map[string]interface{}) string {
	var parts []string
	if component, ok := input["component"].(string); ok {
		parts = append(parts, component)
	}
	if stack, ok := input["stack"].(string); ok {
		parts = append(parts, "-s "+stack)
	}
	if len(parts) > 0 {
		return fmt.Sprintf("**Args:** `%s`", strings.Join(parts, " "))
	}
	return ""
}

// formatGenericToolParams formats tool parameters in a generic key=value format.
func formatGenericToolParams(input map[string]interface{}) string {
	var params []string
	for key, value := range input {
		valueStr := formatParamValue(value)
		params = append(params, fmt.Sprintf("%s=`%s`", key, valueStr))
	}
	if len(params) > 0 {
		return fmt.Sprintf("**Parameters:** %s", strings.Join(params, ", "))
	}
	return ""
}

// formatParamValue converts a parameter value to a display string with truncation.
func formatParamValue(value interface{}) string {
	var valueStr string
	switch v := value.(type) {
	case string:
		valueStr = v
	default:
		valueStr = fmt.Sprintf("%v", v)
	}

	// Truncate long values.
	if len(valueStr) > valueDisplayMaxLen {
		valueStr = valueStr[:valueTruncatedLen] + "..."
	}

	return valueStr
}

// actionPhrases are phrases that indicate the AI intends to take action.
var actionPhrases = []string{
	"i'll",
	"i will",
	"let me",
	"i'm going to",
	"i am going to",
	"now i'll",
	"now i will",
	"first, i'll",
	"first, i will",
	"i'll now",
	"i will now",
}

// actionVerbs are verbs that indicate the AI is about to perform an action.
var actionVerbs = []string{
	"read", "edit", "fix", "update", "modify", "change", "create", "delete",
	"search", "find", "execute", "run", "check", "validate", "describe", "list",
	"use", "start", "begin", "try", "call", "invoke", "get", "fetch", "retrieve", "query",
}

// containsAnyPhrase checks if content contains any of the given phrases.
func containsAnyPhrase(content string, phrases []string) bool {
	for _, phrase := range phrases {
		if strings.Contains(content, phrase) {
			return true
		}
	}
	return false
}

// containsActionVerb checks if content contains any action verb as a whole word.
func containsActionVerb(content string) bool {
	for _, verb := range actionVerbs {
		if strings.Contains(content, " "+verb+" ") ||
			strings.Contains(content, " "+verb+".") ||
			strings.Contains(content, " "+verb+",") ||
			strings.HasPrefix(content, verb+" ") ||
			strings.HasSuffix(content, " "+verb) {
			return true
		}
	}
	return false
}

// detectActionIntent checks if the AI response contains phrases indicating intent to take action.
// Returns true if the AI says it will do something but did not use tools.
func detectActionIntent(content string) bool {
	contentLower := strings.ToLower(content)

	if !containsAnyPhrase(contentLower, actionPhrases) {
		return false
	}

	return containsActionVerb(contentLower)
}
