package atmos

import (
	"context"
	"fmt"
	"os"
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
	binaryPath  string
}

// NewExecuteAtmosCommandTool creates a new Atmos command execution tool.
func NewExecuteAtmosCommandTool(atmosConfig *schema.AtmosConfiguration) *ExecuteAtmosCommandTool {
	// Resolve the current binary path so this works with both installed atmos and go run.
	binary, err := os.Executable()
	if err != nil {
		binary = "atmos"
	}

	return &ExecuteAtmosCommandTool{
		atmosConfig: atmosConfig,
		binaryPath:  binary,
	}
}

// Name returns the tool name.
func (t *ExecuteAtmosCommandTool) Name() string {
	return "execute_atmos_command"
}

// Description returns the tool description.
func (t *ExecuteAtmosCommandTool) Description() string {
	return "LAST RESORT: Execute an Atmos CLI command as a subprocess. Only use this for commands that do NOT have a dedicated tool. Do NOT use this for: listing stacks (use atmos_list_stacks), describing components (use atmos_describe_component), describing affected (use atmos_describe_affected), or validating stacks (use atmos_validate_stacks). Use this only for commands like 'terraform plan', 'terraform apply', 'workflow', etc."
}

// Parameters returns the tool parameters.
func (t *ExecuteAtmosCommandTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "command",
			Description: "The Atmos command to execute (without the 'atmos' prefix). Examples: 'terraform plan vpc -s prod-us-east-1', 'terraform apply vpc -s prod-us-east-1', 'workflow deploy'.",
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

	// Create the command using the resolved binary path.
	cmd := exec.CommandContext(ctx, t.binaryPath, args...) //nolint:gosec // binaryPath is resolved from os.Executable() at construction time, not user input.
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
