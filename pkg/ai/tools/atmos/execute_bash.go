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

// shellMetaChars contains characters and sequences that are interpreted by the
// shell as special operators. If any of these appear in the raw command string,
// the command is rejected to prevent shell-injection attacks.
//
// Note: The tool now executes commands via exec.Command(binary, args...) instead
// of "sh -c <command>", so none of these characters are ever interpreted.  The
// check is therefore defense-in-depth: it gives the caller a clear error message
// rather than silently ignoring the metacharacters.
var shellMetaChars = []string{
	";",   // command separator: cmd1; cmd2
	"&&",  // logical AND: cmd1 && cmd2
	"||",  // logical OR:  cmd1 || cmd2
	"$(", // command substitution: $(cmd)
	"`",   // backtick command substitution: `cmd`
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
	return "Execute a command directly (without a shell) and return the output. Use this for git operations, file listing (ls), searching (grep), package management (npm, pip), and other system commands. Examples: 'git status', 'ls -la', 'grep pattern file.txt', 'npm install'. IMPORTANT: Destructive commands (rm, dd, kill, etc.) are blocked for safety. Shell metacharacters (;, &&, ||, $(...), backticks) are not supported; issue multiple separate commands instead."
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

// validateCommand checks that the command is not blacklisted or dangerous.
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

	if len(args) >= 2 && baseCommand == "rm" {
		for _, arg := range args[1:] {
			if strings.Contains(arg, "-rf") || strings.Contains(arg, "-fr") || arg == "-r" || arg == "-f" {
				log.Warnf("Blocked dangerous rm command: %s", command)
				return &tools.Result{
					Success: false,
					Error:   errUtils.ErrAICommandRmNotAllowed,
				}
			}
		}
	}

	// Defense-in-depth: reject commands that contain shell metacharacters.
	// Because commands are executed directly (exec.Command, not "sh -c"), these
	// characters would not be interpreted anyway — but raising an explicit error
	// prevents confusing silent failures and signals clearly that injection
	// attempts are detected.
	for _, meta := range shellMetaChars {
		if strings.Contains(command, meta) {
			log.Warnf("Blocked command with shell metacharacter %q: %s", meta, command)
			return &tools.Result{
				Success: false,
				Error:   fmt.Errorf("%w: %q found in command", errUtils.ErrAICommandShellInjection, meta),
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

	args := strings.Fields(command)
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
