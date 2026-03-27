package atmos

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/shell"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// blacklistedCommands are commands that are never allowed regardless of arguments.
// Note: "rm" is intentionally NOT in this list so that safe single-file deletion
// (e.g. "rm old-artifact.zip") remains available. Recursive/force rm is blocked
// separately by validateCommand via isRmRecursiveFlag.
var blacklistedCommands = map[string]bool{
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
	return "Execute a command directly (without a shell) and return the output. " +
		"Use this for git operations, file listing (ls), searching (grep), package management (npm, pip), " +
		"and other system commands. Examples: 'git status', 'ls -la', 'grep pattern file.txt', 'npm install'. " +
		"Quoted strings are supported for arguments that contain spaces or special characters, e.g. " +
		"git commit -m 'my message' or grep \"pattern;with;colons\" file.txt. " +
		"Shell operators (;, &&, ||, |, >, <, &, $(...), backticks) are only allowed inside quotes as " +
		"literal text — unquoted shell operators are rejected. " +
		"Destructive commands (dd, mkfs, kill, etc.) are blocked. " +
		"Recursive rm (rm -r, rm -rf) is blocked; single-file rm is allowed."
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

// isRmRecursiveFlag reports whether the given rm flag argument enables recursive
// or unconditionally-forced deletion.  It recognises long-form flags such as
// --recursive and --no-preserve-root, and any short-flag string that contains
// the letter 'r' or 'R' (covering -r, -R, -rf, -Rf, -fr, -fRv, etc.).
func isRmRecursiveFlag(arg string) bool {
	switch arg {
	case "--recursive", "--no-preserve-root":
		return true
	}
	// Short-form flag: starts with '-' but not '--'.
	if len(arg) > 1 && arg[0] == '-' && arg[1] != '-' {
		for _, c := range arg[1:] {
			if c == 'r' || c == 'R' {
				return true
			}
		}
	}
	return false
}

// validateCommand checks that the (already-tokenized) command is not blacklisted
// or otherwise unsafe.  Shell-operator injection is handled upstream by the
// tokenizer (shell.Fields), so this function focuses on blacklist and rm safety.
func validateCommand(args []string, command string) *tools.Result {
	baseCommand := args[0]

	if blacklistedCommands[baseCommand] {
		log.Warnf("Blocked blacklisted command: %s", baseCommand)
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: %s", errUtils.ErrAICommandBlacklisted, baseCommand),
		}
	}

	if strings.HasPrefix(baseCommand, "mkfs.") {
		log.Warnf("Blocked mkfs variant command: %s", baseCommand)
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: %s", errUtils.ErrAICommandBlacklisted, baseCommand),
		}
	}

	// Allow "rm" for single-file deletions, but block recursive/force-recursive patterns.
	if baseCommand == "rm" {
		for _, arg := range args[1:] {
			if isRmRecursiveFlag(arg) {
				log.Warnf("Blocked dangerous rm command: %s", command)
				return &tools.Result{
					Success: false,
					Error:   errUtils.ErrAICommandRmNotAllowed,
				}
			}
		}
	}

	return nil
}

// resolveWorkingDir determines the working directory for command execution.
func (t *ExecuteBashCommandTool) resolveWorkingDir(params map[string]interface{}) string {
	workingDir := t.atmosConfig.BasePath
	wd, ok := params["working_dir"].(string)
	if !ok || wd == "" {
		return workingDir
	}

	if !filepath.IsAbs(wd) {
		return filepath.Join(t.atmosConfig.BasePath, wd)
	}
	return wd
}

// Execute runs the shell command and returns the output.
func (t *ExecuteBashCommandTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	command, ok := params["command"].(string)
	if !ok || command == "" {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: command", errUtils.ErrAIToolParameterRequired),
		}, nil
	}

	// shell.Fields performs POSIX-like word splitting with full quote handling
	// (single quotes, double quotes, backslash escapes).  It also returns an
	// error for any unquoted shell operator (;, &&, ||, |, >, <, &, $(...),
	// backticks, >>, 2>&1, etc.), which eliminates the need for a separate
	// metacharacter check and -- critically -- supports operators INSIDE quotes
	// as literal text (e.g. grep "a;b" file or git commit -m "fix && support").
	args, shellErr := shell.Fields(command, nil)
	if shellErr != nil {
		log.Warnf("Rejected command with invalid shell syntax: %s: %v", command, shellErr)
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: %v", errUtils.ErrAICommandShellInjection, shellErr),
		}, nil
	}

	if len(args) == 0 {
		return &tools.Result{
			Success: false,
			Error:   errUtils.ErrAICommandEmpty,
		}, nil
	}

	if errResult := validateCommand(args, command); errResult != nil {
		return errResult, nil
	}

	workingDir := t.resolveWorkingDir(params)

	log.Debugf("Executing command: %s (in %s)", command, workingDir)

	// Execute the command directly — without a shell intermediary.
	// This eliminates shell-metacharacter interpretation (CWE-78) entirely:
	// args[0] is the binary, args[1:] are the literal arguments.
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = workingDir

	output, err := cmd.CombinedOutput()

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
