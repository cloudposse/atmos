package atmos

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/ai/tools/permission"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// destructiveTerraformSubcmds is the set of Terraform subcommands that modify infrastructure state.
// These operations are never auto-executed; they require ModePrompt with explicit user confirmation.
var destructiveTerraformSubcmds = map[string]bool{
	"apply":        true,
	"destroy":      true,
	"import":       true,
	"force-unlock": true,
}

// destructiveTerraformStateSubcmds is the set of "terraform state" subcommands that modify state.
var destructiveTerraformStateSubcmds = map[string]bool{
	"rm":   true,
	"mv":   true,
	"push": true,
}

// destructiveTerraformWorkspaceSubcmds is the set of "terraform workspace" subcommands that modify state.
var destructiveTerraformWorkspaceSubcmds = map[string]bool{
	"new":    true,
	"delete": true,
}

// ExecuteAtmosCommandTool executes any Atmos CLI command.
type ExecuteAtmosCommandTool struct {
	atmosConfig    *schema.AtmosConfiguration
	binaryPath     string
	permissionMode permission.Mode
}

// NewExecuteAtmosCommandTool creates a new Atmos command execution tool.
func NewExecuteAtmosCommandTool(atmosConfig *schema.AtmosConfiguration) *ExecuteAtmosCommandTool {
	// Resolve the current binary path so this works with both installed atmos and go run.
	binary, err := os.Executable()
	if err != nil {
		binary = "atmos"
	}

	return &ExecuteAtmosCommandTool{
		atmosConfig:    atmosConfig,
		binaryPath:     binary,
		permissionMode: permission.ModePrompt, // default to safest mode
	}
}

// NewExecuteAtmosCommandToolWithPermission creates a new Atmos command execution tool with explicit permission mode.
func NewExecuteAtmosCommandToolWithPermission(atmosConfig *schema.AtmosConfiguration, mode permission.Mode) *ExecuteAtmosCommandTool {
	t := NewExecuteAtmosCommandTool(atmosConfig)
	t.permissionMode = mode
	return t
}

// Name returns the tool name.
func (t *ExecuteAtmosCommandTool) Name() string {
	return "execute_atmos_command"
}

// Description returns the tool description.
func (t *ExecuteAtmosCommandTool) Description() string {
	return "LAST RESORT: Execute an Atmos CLI command as a subprocess. " +
		"Only use this for commands that do NOT have a dedicated tool. " +
		"Do NOT use this for: listing stacks (use atmos_list_stacks), describing components (use atmos_describe_component), " +
		"describing affected (use atmos_describe_affected), or validating stacks (use atmos_validate_stacks). " +
		"Safe read-only commands (terraform plan, show, output, validate, state list, workspace list, describe, list) are always allowed. " +
		"State-modifying operations (terraform apply, destroy, import, force-unlock, state rm/mv/push, workspace new/delete) " +
		"require ModePrompt for user confirmation; all other modes block them at the tool layer."
}

// Parameters returns the tool parameters.
func (t *ExecuteAtmosCommandTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "command",
			Description: "The Atmos command to execute (without the 'atmos' prefix). Examples: 'terraform plan vpc -s prod-us-east-1', 'terraform show vpc -s prod-us-east-1', 'workflow deploy'. State-modifying commands (apply, destroy, import, etc.) require ModePrompt permission mode.",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
	}
}

// isDestructiveAtmosCommand reports whether args represent a state-modifying Terraform operation.
func isDestructiveAtmosCommand(args []string) bool {
	if len(args) < 2 {
		return false
	}

	if strings.ToLower(args[0]) != "terraform" {
		return false
	}

	subCmd := strings.ToLower(args[1])

	if destructiveTerraformSubcmds[subCmd] {
		return true
	}

	if subCmd == "state" && len(args) >= 3 {
		stateSubCmd := strings.ToLower(args[2])
		return destructiveTerraformStateSubcmds[stateSubCmd]
	}

	if subCmd == "workspace" && len(args) >= 3 {
		wsSubCmd := strings.ToLower(args[2])
		return destructiveTerraformWorkspaceSubcmds[wsSubCmd]
	}

	return false
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

	log.Debugf("Executing Atmos command: atmos %s", command)

	// Split command into args.
	args := strings.Fields(command)
	if len(args) == 0 {
		return &tools.Result{
			Success: false,
			Error:   errUtils.ErrAICommandEmpty,
		}, nil
	}

	// Validate subcommand: block state-modifying operations unless the permission mode
	// explicitly requires user confirmation (ModePrompt). This prevents prompt-injection
	// and LLM-jacking attacks from triggering destructive operations automatically.
	if isDestructiveAtmosCommand(args) {
		if t.permissionMode != permission.ModePrompt {
			log.Warnf("Blocked destructive Atmos command in non-interactive mode: atmos %s", command)
			return &tools.Result{
				Success: false,
				Error:   fmt.Errorf("%w: atmos %s", errUtils.ErrAICommandDestructive, command),
			}, nil
		}
		log.Warnf("Destructive Atmos command will require user confirmation: atmos %s", command)
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
	// Permission system will handle per-command restrictions.
	return false
}
