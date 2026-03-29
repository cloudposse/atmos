package atmos

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/shell"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// allowedCommands is the explicit allow-list of binaries that may be executed.
// Any binary not in this list is rejected with ErrAICommandNotAllowed before
// any further validation is performed.  Adding an entry here is required but
// not sufficient: the command must also pass the blacklist and rm safety checks.
var allowedCommands = map[string]bool{
	"git":     true,
	"grep":    true,
	"ls":      true,
	"cat":     true,
	"echo":    true,
	"find":    true,
	"pwd":     true,
	"npm":     true,
	"pip":     true,
	"pip3":    true,
	"go":      true,
	"make":    true,
	"curl":    true,
	"wget":    true,
	"rm":      true,
	"cp":      true,
	"mv":      true,
	"mkdir":   true,
	"touch":   true,
	"head":    true,
	"tail":    true,
	"sort":    true,
	"uniq":    true,
	"wc":      true,
	"diff":    true,
	"which":   true,
	"env":     true,
	"date":    true,
	"uname":   true,
	"python":  true,
	"python3": true,
	"node":    true,
	"yarn":    true,
	"jq":      true,
	"awk":     true,
	"sed":     true,
}

// blacklistedCommands are commands that are never allowed regardless of arguments.
// Because allowedCommands is checked first, this list acts as defense-in-depth
// for commands that might inadvertently be added to allowedCommands in the future.
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
		"Only an explicit allow-list of commands may be run (git, grep, ls, cat, echo, find, " +
		"npm, pip, go, make, curl, wget, rm, cp, mv, mkdir, touch, and more). " +
		"Quoted strings are supported for arguments that contain spaces or special characters, e.g. " +
		"git commit -m 'my message' or grep \"pattern;with;colons\" file.txt. " +
		"Shell operators (;, &&, ||, |, >, <, &, $(...), backticks) are only allowed inside quotes as " +
		"literal text -- unquoted shell operators are rejected. " +
		"Destructive commands (dd, mkfs, kill, etc.) are blocked. " +
		"Recursive rm (rm -r, rm -rf, rm -d) is blocked; rm of files within the project or " +
		"temp directory is allowed. The working directory must be within the Atmos base path."
}

// Parameters returns the tool parameters.
func (t *ExecuteBashCommandTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "command",
			Description: "The shell command to execute. Examples: 'git status', 'ls -la components/', 'grep -r pattern stacks/', 'npm install'. Only allowed commands are executed.",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        "working_dir",
			Description: "Optional working directory for command execution. If not specified, uses the Atmos base path. Must be within the Atmos base path.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
	}
}

// isRmRecursiveFlag reports whether the given rm flag argument enables recursive,
// directory, or unconditionally-forced deletion.  It recognises long-form flags
// such as --recursive, --no-preserve-root, and --dir, and any short-flag string
// that contains the letter 'r' or 'R' (covering -r, -R, -rf, -Rf, -fr, -fRv, etc.).
func isRmRecursiveFlag(arg string) bool {
	switch arg {
	case "--recursive", "--no-preserve-root", "-d", "--dir":
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

// validateCommand checks that the (already-tokenized) command is permitted and
// safe.  Validation order: allow-list first, then deny-list (defense-in-depth),
// then rm recursive-flag safety.  Path-scope validation for rm targets is
// performed separately by validateRmPaths after the working directory is resolved.
func validateCommand(args []string, command string) *tools.Result {
	baseCommand := args[0]

	// Enforce the explicit allow-list before any other check.
	if !allowedCommands[baseCommand] {
		log.Warnf("Blocked command not in allow-list: %s", baseCommand)
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: %s", errUtils.ErrAICommandNotAllowed, baseCommand),
		}
	}

	// Defense-in-depth: deny explicitly blacklisted commands even if they are
	// ever added to allowedCommands by mistake.
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

	// Allow "rm" for single-file deletions, but block recursive and directory-removal flags.
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

// validateRmPaths checks that every non-flag path argument to rm resolves to a
// location within the tool's base path or the OS temporary directory.  This
// prevents the AI from deleting system files even when rm is used without
// recursive flags.  Relative paths are resolved against workingDir, not the
// process working directory, to match actual command execution semantics.
// Symlinks in the target path are resolved before the scope check to prevent
// symlink-based path traversal attacks.
func (t *ExecuteBashCommandTool) validateRmPaths(args []string, workingDir string) *tools.Result {
	basePath := t.atmosConfig.BasePath
	tmpDir := os.TempDir()

	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "-") {
			continue // flags are already checked by validateCommand.
		}

		// Resolve to an absolute path using the command's working directory
		// for relative paths (not the process working directory).
		var absPath string
		if filepath.IsAbs(arg) {
			absPath = filepath.Clean(arg)
		} else {
			absPath = filepath.Clean(filepath.Join(workingDir, arg))
		}

		// Resolve symlinks to prevent symlink-based path traversal attacks.
		// If the path does not exist yet, EvalSymlinks fails and we fall back
		// to the cleaned path (the OS will reject the rm itself).
		if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
			absPath = resolved
		}

		// Accept paths that are within the base path or the OS temp directory.
		relToBase, errBase := filepath.Rel(basePath, absPath)
		relToTmp, errTmp := filepath.Rel(tmpDir, absPath)

		withinBase := errBase == nil && !strings.HasPrefix(relToBase, "..")
		withinTmp := errTmp == nil && !strings.HasPrefix(relToTmp, "..")

		if !withinBase && !withinTmp {
			log.Warnf("Blocked rm of path outside allowed scope: %s (basePath=%s, tmpDir=%s)", absPath, basePath, tmpDir)
			return &tools.Result{
				Success: false,
				Error:   errUtils.ErrAICommandRmNotAllowed,
			}
		}
	}

	return nil
}

// resolveWorkingDir determines the working directory for command execution.
// The resolved directory must be within the tool's base path; if it is not,
// the base path is used as a fallback and a warning is logged.  Symlinks in
// the provided path are resolved before the scope check to prevent symlink-
// based path traversal attacks.
func (t *ExecuteBashCommandTool) resolveWorkingDir(params map[string]interface{}) string {
	basePath := t.atmosConfig.BasePath
	wd, ok := params["working_dir"].(string)
	if !ok || wd == "" {
		return basePath
	}

	var resolved string
	if !filepath.IsAbs(wd) {
		// Relative paths are always joined against basePath, so they are
		// guaranteed to be within it (unless they use ".." traversal).
		resolved = filepath.Join(basePath, wd)
	} else {
		resolved = wd
	}

	// Resolve symlinks to prevent symlink-based path traversal attacks.
	// Only resolve when the path actually exists; non-existent paths fall back
	// to basePath via the Rel check below.
	if realPath, err := filepath.EvalSymlinks(resolved); err == nil {
		resolved = realPath
	}

	// Validate that the resolved directory is within the base path.
	rel, err := filepath.Rel(basePath, resolved)
	if err != nil || strings.HasPrefix(rel, "..") {
		log.Warnf("Working directory %q is outside the base path %q; falling back to base path", resolved, basePath)
		return basePath
	}
	return resolved
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
		log.Debugf("shell.Fields error for %q: %v", command, shellErr)
		return &tools.Result{
			Success: false,
			Error:   errUtils.ErrAICommandShellInjection,
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

	// Enforce path-scope for rm: targets must be within basePath or os.TempDir().
	if args[0] == "rm" {
		if errResult := t.validateRmPaths(args, workingDir); errResult != nil {
			return errResult, nil
		}
	}

	log.Debugf("Executing command: %s (in %s)", command, workingDir)

	// Execute the command directly -- without a shell intermediary.
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

