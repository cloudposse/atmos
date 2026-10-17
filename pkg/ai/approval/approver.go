package approval

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	deniedMessage         = "Denied by user."
	abortedMessage        = "Aborted by user."
	defaultToolDescriptor = "Claude Code tool request"
)

// PermissionApprover answers provider tool requests with the Atmos permission
// system: ai.tools.mode, the allowed/restricted/blocked lists, the persistent
// decision cache, and the interactive prompt.
//
// Prompt hooks (for example to pause a spinner) are configured on the checker with
// permission.WithPromptHooks, so they fire only around real prompts.
type PermissionApprover struct {
	checker *permission.Checker
	// sem serializes Approve so parallel tool calls never overlap prompts.
	sem chan struct{}
}

// Compile-time check that PermissionApprover implements Approver.
var _ Approver = (*PermissionApprover)(nil)

// NewPermissionApprover creates an approver backed by the given permission checker.
func NewPermissionApprover(checker *permission.Checker) *PermissionApprover {
	return &PermissionApprover{
		checker: checker,
		sem:     make(chan struct{}, 1),
	}
}

// Approve decides whether the requested tool invocation may run. Calls are
// serialized; a call waiting for its turn returns the context error if ctx is
// canceled first.
func (a *PermissionApprover) Approve(ctx context.Context, req Request) (Decision, error) {
	defer perf.Track(nil, "approval.PermissionApprover.Approve")()

	select {
	case a.sem <- struct{}{}:
		defer func() { <-a.sem }()
	case <-ctx.Done():
		return Decision{}, ctx.Err()
	}

	tool := newRequestTool(req)
	allowed, err := a.checker.CheckPermission(ctx, tool, flattenInput(req.Input))
	return mapDecision(req.ToolName, allowed, err)
}

// mapDecision converts a permission checker outcome into an approval Decision.
func mapDecision(toolName string, allowed bool, err error) (Decision, error) {
	switch {
	case err == nil && allowed:
		return Decision{Allow: true}, nil
	case err == nil:
		return Decision{Message: deniedMessage}, nil
	case errors.Is(err, errUtils.ErrUserAborted):
		return Decision{Interrupt: true, Message: abortedMessage}, nil
	case errors.Is(err, errUtils.ErrInteractiveNotAvailable):
		return Decision{
			NonInteractive: true,
			Message: fmt.Sprintf("Approval for %s needs a terminal, and none is available. "+
				"To run it without prompting, set ai.tools.mode to allow or yolo, "+
				"or list the tool in ai.providers.claude-code.allowed_tools.", toolName),
		}, nil
	case errors.Is(err, errUtils.ErrAIToolBlocked):
		return Decision{Message: fmt.Sprintf("Tool %s is blocked by ai.tools.blocked.", toolName)}, nil
	case errors.Is(err, errUtils.ErrAIToolExecutionDenied):
		return Decision{Message: deniedMessage}, nil
	case errors.Is(err, errUtils.ErrAIToolsDisabled):
		return Decision{Message: "Tools are disabled by the AI tool settings."}, nil
	default:
		return Decision{}, fmt.Errorf("permission check for %s failed: %w", toolName, err)
	}
}

// requestTool adapts a provider Request to permission.Tool and permission.ScopedTool.
type requestTool struct {
	req Request
}

var _ permission.ScopedTool = (*requestTool)(nil)

func newRequestTool(req Request) *requestTool {
	return &requestTool{req: req}
}

// Name returns the provider's tool name, which the checker matches against the
// allowed, restricted, and blocked lists.
func (t *requestTool) Name() string { return t.req.ToolName }

// Description returns the request's own description when present.
func (t *requestTool) Description() string {
	if desc, ok := t.req.Input["description"].(string); ok && desc != "" {
		return desc
	}
	return defaultToolDescriptor
}

// IsRestricted is false: restrictions come from ai.tools.restricted.
func (t *requestTool) IsRestricted() bool { return false }

// CacheKey scopes remembered decisions to the specific command or target, such as
// "Bash(atmos list stacks)" or "Read(/repo/go.mod)". When the request has no stable
// specifier the key is the bare tool name, so a remembered decision then applies
// to every request of that tool that also lacks a specifier.
func (t *requestTool) CacheKey() string {
	if spec := specifier(t.req); spec != "" {
		return t.req.ToolName + "(" + spec + ")"
	}
	return t.req.ToolName
}

// specifier returns the input value that identifies what the tool will act on.
func specifier(req Request) string {
	if req.ToolName == "Bash" {
		return stringInput(req.Input, "command")
	}
	for _, key := range []string{"file_path", "notebook_path", "path", "url", "pattern"} {
		if v := stringInput(req.Input, key); v != "" {
			return v
		}
	}
	return ""
}

// stringInput returns the string value for key, or "" when absent or not a string.
func stringInput(input map[string]any, key string) string {
	v, _ := input[key].(string)
	return v
}

// flattenInput stringifies the input values for display and audit. Keys are copied
// as-is; maps and slices are rendered as JSON.
func flattenInput(input map[string]any) map[string]interface{} {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(input))
	for k, v := range input {
		out[k] = stringify(v)
	}
	return out
}

// stringify renders a single input value as text.
func stringify(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case nil:
		return ""
	case map[string]any, []any:
		if data, err := json.Marshal(val); err == nil {
			return string(data)
		}
	}
	return fmt.Sprint(v)
}
