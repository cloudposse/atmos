package atmos

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Blacklisted commands that are never allowed.
var blacklistedCommands = map[string]bool{
	"rm":       true,
	"dd":       true,
	"mkfs":     true,
	"format":   true,
	"fdisk":    true,
	"parted":   true,
	"kill":     true,
	"killall":  true,
	"pkill":    true,
	"reboot":   true,
	"shutdown": true,
	"halt":     true,
	"poweroff": true,
	"init":     true,
}

// ExecuteBashCommandTool executes any shell command.
type ExecuteBashCommandTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewExecuteBashCommandTool creates a new bash command execution tool.
func NewExecuteBashCommandTool(atmosConfig *schema.AtmosConfiguration) *ExecuteBashCommandTool {
	return &ExecuteBashCommandTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *ExecuteBashCommandTool) Name() string {
	return "execute_bash_command"
}

// Description returns the tool description.
func (t *ExecuteBashCommandTool) Description() string {
	return "Execute any shell command and return the output. Use this for git operations, file listing (ls), searching (grep), package management (npm, pip), and other system commands. Examples: 'git status', 'ls -la', 'grep pattern file.txt', 'npm install'. IMPORTANT: Destructive commands (rm, dd, kill, etc.) are blocked for safety."
}

// Parameters returns the tool parameters.
func (t *ExecuteBashCommandTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "command",
			Description: "The shell command to execute. Examples: 'git status', 'ls -la components/', 'grep -r pattern stacks/', 'npm install'. Destructive commands are blocked.",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "working_dir",
			Description: "Optional working directory for command execution. If not specified, uses the Atmos base path. Can be relative to base path or absolute.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// Execute runs the shell command and returns the output.
func (t *ExecuteBashCommandTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	// Extract command parameter.
	command, ok := params["command"].(string)
	if !ok || command == "" {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: command", errUtils.ErrAIToolParameterRequired),
		}, nil
	}

	// Split command into args.
	args := strings.Fields(command)
	if len(args) == 0 {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("command cannot be empty"),
		}, nil
	}

	// Security check: ensure command is not blacklisted.
	baseCommand := args[0]
	if blacklistedCommands[baseCommand] {
		log.Warn(fmt.Sprintf("Blocked blacklisted command: %s", baseCommand))
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("command '%s' is blacklisted for security reasons", baseCommand),
		}, nil
	}

	// Also check for mkfs variants (mkfs.ext4, mkfs.xfs, etc.).
	if strings.HasPrefix(baseCommand, "mkfs.") {
		log.Warn(fmt.Sprintf("Blocked mkfs variant command: %s", baseCommand))
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("command '%s' is blacklisted for security reasons", baseCommand),
		}, nil
	}

	// Additional safety check: block rm with dangerous flags.
	if len(args) >= 2 && baseCommand == "rm" {
		for _, arg := range args[1:] {
			if strings.Contains(arg, "-rf") || strings.Contains(arg, "-fr") || arg == "-r" || arg == "-f" {
				log.Warn(fmt.Sprintf("Blocked dangerous rm command: %s", command))
				return &tools.Result{
					Success: false,
					Error:   fmt.Errorf("rm with recursive or force flags is not allowed"),
				}, nil
			}
		}
	}

	// Determine working directory.
	workingDir := t.atmosConfig.BasePath
	if wd, ok := params["working_dir"].(string); ok && wd != "" {
		// If relative path, make it relative to base path.
		if !filepath.IsAbs(wd) {
			workingDir = filepath.Join(t.atmosConfig.BasePath, wd)
		} else {
			workingDir = wd
		}
	}

	log.Debug(fmt.Sprintf("Executing shell command: %s (in %s)", command, workingDir))

	// Create the command.
	// Use sh -c to properly handle pipes, redirects, and other shell features.
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = workingDir

	// Capture output.
	output, err := cmd.CombinedOutput()

	// Even if there's an error, return the output for the AI to analyze.
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	result := &tools.Result{
		Success: err == nil,
		Output:  string(output),
		Data: map[string]interface{}{
			"command":     command,
			"working_dir": workingDir,
			"exit_code":   exitCode,
		},
	}

	if err != nil {
		result.Error = fmt.Errorf("command failed with exit code %d: %w", exitCode, err)
	}

	return result, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *ExecuteBashCommandTool) RequiresPermission() bool {
	return true // All shell command execution requires user confirmation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *ExecuteBashCommandTool) IsRestricted() bool {
	return false // Permission system will handle per-command restrictions.
}
