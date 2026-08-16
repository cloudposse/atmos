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
// The permission mode is derived from the atmos configuration so that
// require_confirmation / yolo_mode settings are respected at the tool layer.
func NewExecuteAtmosCommandTool(atmosConfig *schema.AtmosConfiguration) *ExecuteAtmosCommandTool {
	// Resolve the current binary path so this works with both installed atmos and go run.
	binary, err := os.Executable()
	if err != nil {
		binary = "atmos"
	}

	return &ExecuteAtmosCommandTool{
		atmosConfig:    atmosConfig,
		binaryPath:     binary,
		permissionMode: permissionModeFromConfig(atmosConfig),
	}
}

// permissionModeFromConfig derives the tool-layer permission mode from atmos configuration.
// It mirrors the global permission-checker setup in cmd/ai so that the tool respects
// the same require_confirmation / yolo_mode settings as the rest of the AI subsystem.
func permissionModeFromConfig(atmosConfig *schema.AtmosConfiguration) permission.Mode {
	if atmosConfig == nil {
		return permission.ModePrompt
	}
	if atmosConfig.AI.Tools.YOLOMode {
		return permission.ModeYOLO
	}
	if atmosConfig.AI.Tools.RequireConfirmation != nil && !*atmosConfig.AI.Tools.RequireConfirmation {
		return permission.ModeAllow
	}
	return permission.ModePrompt
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
		"Safe read-only commands (terraform plan, show, output, validate, state list, workspace list, describe, list) " +
		"are not blocked by this validator, though RequiresPermission() still returns true and the global permission " +
		"checker (ModePrompt or ModeDeny) will still prompt or deny as appropriate. " +
		"State-modifying operations (terraform apply, destroy, import, force-unlock, state rm/mv/push, workspace new/delete) " +
		"require ModePrompt for user confirmation; ModeAllow, ModeDeny, and ModeYOLO block them at the tool layer."
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

// firstNonFlag returns the lowercased first token at or after start that is not
// a flag argument, along with its index. It handles both embedded-value flags
// (--flag=value) and space-separated flags (--flag value) by advancing past the
// potential value token when a valueless flag (no "=" in the token) is encountered.
// Returns ("", -1) if no non-flag token exists.
func firstNonFlag(args []string, start int) (string, int) {
	for i := start; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			return strings.ToLower(arg), i
		}
		// Embedded-value flag (e.g., --chdir=.): value is in the same token; skip only this.
		if strings.Contains(arg, "=") {
			continue
		}
		// Valueless flag token (e.g., --chdir or -chdir or -s):
		// treat the next token as the flag's space-separated value and skip it too.
		if i+1 < len(args) {
			i++
		}
	}
	return "", -1
}

// isDestructiveAtmosCommand reports whether args represent a state-modifying Terraform operation.
// It skips leading flags (tokens that start with "-") when searching for the subcommand so that
// commands like "terraform -s prod apply vpc" are correctly classified.
func isDestructiveAtmosCommand(args []string) bool {
	if len(args) < 2 {
		return false
	}

	if strings.ToLower(args[0]) != "terraform" {
		return false
	}

	subCmd, subIdx := firstNonFlag(args, 1)
	if subCmd == "" {
		return false
	}

	if destructiveTerraformSubcmds[subCmd] {
		return true
	}

	if subCmd == "state" {
		nested, _ := firstNonFlag(args, subIdx+1)
		return destructiveTerraformStateSubcmds[nested]
	}

	if subCmd == "workspace" {
		nested, _ := firstNonFlag(args, subIdx+1)
		return destructiveTerraformWorkspaceSubcmds[nested]
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
