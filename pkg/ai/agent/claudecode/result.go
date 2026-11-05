package claudecode

import (
	"encoding/json"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
)

// claudeResponse is the final "result" object of `claude -p --output-format stream-json`
// (and the single object of `--output-format json`).
type claudeResponse struct {
	Type              string             `json:"type"`
	Subtype           string             `json:"subtype"`
	Result            string             `json:"result"`
	CostUSD           float64            `json:"cost_usd"`
	TotalCostUSD      float64            `json:"total_cost_usd"`
	DurationMS        int                `json:"duration_ms"`
	IsError           bool               `json:"is_error"`
	SessionID         string             `json:"session_id"`
	NumTurns          int                `json:"num_turns"`
	Errors            []string           `json:"errors"`
	PermissionDenials []permissionDenial `json:"permission_denials"`
}

// permissionDenial describes a tool call Claude refused to run.
type permissionDenial struct {
	ToolName  string         `json:"tool_name"`
	ToolUseID string         `json:"tool_use_id"`
	ToolInput map[string]any `json:"tool_input"`
}

// subtypeMaxTurns is the substring of the result subtype Claude reports when --max-turns is reached
// (observed value: "error_max_turns").
const subtypeMaxTurns = "max_turns"

// parseResponse extracts the result text from a Claude Code result object.
func parseResponse(output []byte) (string, error) {
	var resp claudeResponse
	if err := json.Unmarshal(output, &resp); err != nil {
		// If not valid JSON, return raw text (Claude Code may output plain text on some errors).
		trimmed := strings.TrimSpace(string(output))
		if trimmed != "" {
			return trimmed, nil
		}
		return "", fmt.Errorf("%w: %w", errUtils.ErrCLIProviderParseResponse, err)
	}

	return evaluateResult(&resp)
}

// evaluateResult turns a decoded result object into the answer text or the matching error.
func evaluateResult(resp *claudeResponse) (string, error) {
	if len(resp.PermissionDenials) > 0 {
		denial := resp.PermissionDenials[0]
		return "", toolDeniedError(denial.ToolName, summarize(denial.ToolName, denial.ToolInput))
	}

	if strings.Contains(resp.Subtype, subtypeMaxTurns) {
		return "", maxTurnsError(resp.NumTurns)
	}

	if resp.IsError {
		return "", fmt.Errorf("%w: %s: %s", errUtils.ErrCLIProviderExecFailed, ProviderName, failureText(resp))
	}

	if strings.TrimSpace(resp.Result) == "" {
		return "", errUtils.Build(errUtils.ErrCLIProviderEmptyResponse).
			WithContext("provider", ProviderName).
			WithContext("turns", resp.NumTurns).
			WithHint("Retry the request").
			WithHint("Raise `ai.providers.claude-code.max_turns` if the model ran out of turns").
			Err()
	}

	return resp.Result, nil
}

// failureText returns the best available description of a failed result.
func failureText(resp *claudeResponse) string {
	if resp.Result != "" {
		return resp.Result
	}
	if len(resp.Errors) > 0 {
		return strings.Join(resp.Errors, "; ")
	}
	if resp.Subtype != "" {
		return resp.Subtype
	}
	return "unknown error"
}

// maxTurnsError reports that Claude hit its turn limit before producing an answer.
func maxTurnsError(turns int) error {
	return errUtils.Build(errUtils.ErrCLIProviderMaxTurns).
		WithContext("provider", ProviderName).
		WithContext("turns", turns).
		WithHint("Raise `ai.providers.claude-code.max_turns` in atmos.yaml").
		Err()
}

// toolDeniedError reports that a tool needed approval that could not be given.
func toolDeniedError(tool, summary string) error {
	return errUtils.Build(errUtils.ErrCLIProviderToolDenied).
		WithContext("provider", ProviderName).
		WithContext("tool", tool).
		WithExplanationf("Claude Code needed approval to run: %s", summary).
		WithHint("Run the command in a terminal so the tool call can be approved interactively").
		WithHint("Set `ai.tools.mode: yolo` or `allow` in atmos.yaml to skip approval prompts").
		WithHint("Add the tool to `ai.providers.claude-code.allowed_tools` in atmos.yaml").
		Err()
}
