package atmos

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecuteAtmosCommandTool executes any Atmos CLI command.
type ExecuteAtmosCommandTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewExecuteAtmosCommandTool creates a new Atmos command execution tool.
func NewExecuteAtmosCommandTool(atmosConfig *schema.AtmosConfiguration) *ExecuteAtmosCommandTool {
	return &ExecuteAtmosCommandTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *ExecuteAtmosCommandTool) Name() string {
	return "execute_atmos_command"
}

// Description returns the tool description.
func (t *ExecuteAtmosCommandTool) Description() string {
	return "Execute any Atmos CLI command and return the output. Use this when the user explicitly asks to 'execute', 'run', or use a specific Atmos command. This includes ALL commands: 'describe stacks', 'describe component', 'list stacks', 'validate stacks', 'describe affected', 'terraform plan', 'terraform apply', 'workflow', etc. Examples: 'describe stacks', 'describe component vpc -s prod-us-east-1', 'list stacks', 'validate stacks', 'terraform plan vpc -s prod-us-east-1'."
}

// Parameters returns the tool parameters.
func (t *ExecuteAtmosCommandTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "command",
			Description: "The Atmos command to execute (without the 'atmos' prefix). Can be ANY Atmos command. Examples: 'describe stacks', 'describe component vpc -s prod-us-east-1', 'list stacks', 'validate stacks', 'describe affected', 'terraform plan vpc -s prod-us-east-1'.",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
	}
}

// Execute runs the Atmos command and returns the output.
func (t *ExecuteAtmosCommandTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	// Extract command parameter.
	command, ok := params["command"].(string)
	if !ok || command == "" {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: command", errUtils.ErrAIToolParameterRequired),
		}, nil
	}

	log.Debug(fmt.Sprintf("Executing Atmos command: atmos %s", command))

	// Split command into args.
	args := strings.Fields(command)
	if len(args) == 0 {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("command cannot be empty"),
		}, nil
	}

	// Create the command.
	cmd := exec.CommandContext(ctx, "atmos", args...)
	cmd.Dir = t.atmosConfig.BasePath

	// Capture output.
	output, err := cmd.CombinedOutput()

	// Even if there's an error, return the output for the AI to analyze.
	result := &tools.Result{
		Success: err == nil,
		Output:  string(output),
		Data: map[string]interface{}{
			"command":   fmt.Sprintf("atmos %s", command),
			"exit_code": cmd.ProcessState.ExitCode(),
		},
	}

	if err != nil {
		result.Error = fmt.Errorf("command failed with exit code %d: %w", cmd.ProcessState.ExitCode(), err)
	}

	return result, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *ExecuteAtmosCommandTool) RequiresPermission() bool {
	return true // Command execution requires user confirmation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *ExecuteAtmosCommandTool) IsRestricted() bool {
	// Check if this is a destructive command.
	// Apply, destroy, and workflow commands are always restricted.
	return false // Permission system will handle per-command restrictions.
}
