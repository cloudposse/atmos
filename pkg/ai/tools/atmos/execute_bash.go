package atmos

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"mvdan.cc/sh/v3/shell"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/tools"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// execTimeout is the maximum wall-clock time allowed for a single command execution.
// Any command that does not complete within this window is killed. The parent
// context passed to Execute() may impose a stricter deadline that fires sooner.
const execTimeout = 30 * time.Second

// allowedCommands is the explicit allow-list of binaries that may be executed.
// Interpreter-class binaries (python, node, awk, sed) and network exfiltration
// tools (curl, wget) are intentionally excluded: they can run arbitrary
// sub-commands or send data to remote hosts, bypassing every security check in
// this file.  make is excluded because it executes arbitrary shell recipes.
// env is excluded because it can be used as "env binary args" to run any binary.
// sleep is included so that the execution-timeout mechanism can be tested; it
// cannot exfiltrate data or execute arbitrary code.
var allowedCommands = map[string]bool{
	"git":   true,
	"grep":  true,
	"ls":    true,
	"cat":   true,
	"echo":  true,
	"find":  true,
	"pwd":   true,
	"npm":   true,
	"pip":   true,
	"pip3":  true,
	"go":    true,
	"rm":    true,
	"cp":    true,
	"mv":    true,
	"mkdir": true,
	"touch": true,
	"head":  true,
	"tail":  true,
	"sort":  true,
	"uniq":  true,
	"wc":    true,
	"diff":  true,
	"which": true,
	"date":  true,
	"uname": true,
	"yarn":  true,
	"jq":    true,
	"sleep": true,
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

// interpreterBypassFlags are inline execution flags that allow arbitrary code to
// run inside otherwise-safe build/package-management tools.  They are blocked as
// defense-in-depth even though the enclosing commands are in the allow-list.
var interpreterBypassFlags = map[string]bool{
	"-c":     true,
	"--eval": true,
	"-e":     true,
	"--exec": true,
}

// bypassCheckedCommands are the allow-listed commands that could be abused via
// inline-execution flags.  All args for these commands are scanned against
// interpreterBypassFlags.
var bypassCheckedCommands = map[string]bool{
	"go":   true,
	"git":  true,
	"npm":  true,
	"pip":  true,
	"pip3": true,
	"yarn": true,
}

// sourceScopedCommands are the allow-listed commands whose non-flag path
// arguments must reside within the tool's base path or the OS temporary
// directory.  This prevents the AI from reading sensitive system files.
var sourceScopedCommands = map[string]bool{
	"cp":   true,
	"mv":   true,
	"cat":  true,
	"head": true,
	"tail": true,
	"diff": true,
	"grep": true,
}

// blockedSubcommands maps binary names to sets of subcommands that are never
// permitted.  These subcommands can execute arbitrary code, download untrusted
// packages, or perform write/network operations outside the safe read-only
// inspection use case.
var blockedSubcommands = map[string]map[string]bool{
	"git": {
		"commit":    true,
		"push":      true,
		"clone":     true,
		"submodule": true,
		"remote":    true,
		"config":    true,
	},
	"npm": {
		"exec":    true,
		"run":     true,
		"scripts": true,
	},
	"yarn": {
		"run":  true,
		"exec": true,
	},
	"pip": {
		"install":  true,
		"download": true,
	},
	"pip3": {
		"install":  true,
		"download": true,
	},
	"go": {
		"test":     true,
		"generate": true,
		"get":      true,
	},
}

// ExecuteBashCommandTool executes commands directly (no shell intermediary).
type ExecuteBashCommandTool struct {
	atmosConfig    *schema.AtmosConfiguration
	commandTimeout time.Duration   // defaults to execTimeout (30 s); override in tests.
	allowedCmds    map[string]bool // when non-nil, overrides the package-level allowedCommands; for tests only.
}

// NewExecuteBashCommandTool creates a new direct command execution tool.
func NewExecuteBashCommandTool(atmosConfig *schema.AtmosConfiguration) *ExecuteBashCommandTool {
	return &ExecuteBashCommandTool{
		atmosConfig:    atmosConfig,
		commandTimeout: execTimeout,
	}
}

// Name returns the tool name.
func (t *ExecuteBashCommandTool) Name() string {
	return "execute_direct_command"
}

// Description returns the tool description.
func (t *ExecuteBashCommandTool) Description() string {
	return "Execute a direct command (no shell) and return the output. " +
		"Only an explicit allow-list of commands may be run (git, grep, ls, cat, echo, find, " +
		"npm, pip, go, rm, cp, mv, mkdir, touch, and more). " +
		"Quoted strings are supported for arguments that contain spaces or special characters, e.g. " +
		"git commit -m 'my message' or grep \"pattern;with;colons\" file.txt. " +
		"Shell operators (;, &&, ||, |, >, <, &, $(...), backticks) are only allowed inside quotes as " +
		"literal text -- unquoted shell operators are rejected. " +
		"Environment variable references ($VAR) are not supported; use literal values. " +
		"Destructive commands (dd, mkfs, kill, etc.) are blocked. " +
		"Recursive rm (rm -r, rm -rf, rm -d) is blocked; rm of files within the project or " +
		"temp directory is allowed. The working directory must be within the Atmos base path."
}

// Parameters returns the tool parameters.
func (t *ExecuteBashCommandTool) Parameters() []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        "command",
			Description: "The direct command (no shell) to execute. Examples: 'git status', 'ls -la components/', 'grep -r pattern stacks/', 'npm install'. Only allowed commands are executed.",
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

// containsUnquotedDollar reports whether s contains a '$' character that is not
// protected by single quotes.  Dollar signs outside single quotes indicate
// environment variable references, which are not supported and must be rejected.
//
// Single quotes suppress all special character interpretation including $.
// Double quotes and unquoted context both allow $ to be interpreted as a variable
// reference in a real shell, so they are treated as unquoted for this check.
func containsUnquotedDollar(s string) bool {
	// Fast path: skip the byte-by-byte scan when there is no '$' at all.
	if !strings.ContainsRune(s, '$') {
		return false
	}
	inSingle := false
	inDouble := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '\\':
			if !inSingle {
				i++ // skip the next character (backslash-escaped).
			}
		case '$':
			if !inSingle {
				return true
			}
		}
	}
	return false
}

// resolveArg resolves a command argument to a clean absolute path.  Relative
// arguments are resolved against workingDir.  Symlinks are evaluated to prevent
// symlink-based path traversal attacks; if EvalSymlinks fails (path does not
// exist), the cleaned path is returned as-is.
func resolveArg(arg, workingDir string) string {
	var absPath string
	if filepath.IsAbs(arg) {
		absPath = filepath.Clean(arg)
	} else {
		absPath = filepath.Clean(filepath.Join(workingDir, arg))
	}
	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		absPath = resolved
	}
	return absPath
}

// isPathLike reports whether s looks like a file-system path: an absolute path,
// a relative path starting with "./" or "../" (or the Windows equivalents ".\\"
// and "..\\"), or a string containing the OS path separator.  The check is
// intentionally conservative so that command arguments that are not paths
// (e.g., search patterns, format strings) are not misclassified and subjected
// to an unnecessary scope check.
func isPathLike(s string) bool {
	return filepath.IsAbs(s) ||
		strings.HasPrefix(s, "./") ||
		strings.HasPrefix(s, "../") ||
		strings.HasPrefix(s, `.\\`) ||
		strings.HasPrefix(s, `..\\`) ||
		strings.ContainsRune(s, filepath.Separator)
}

// effectiveAllowedCmds returns the caller-supplied override allowlist when set,
// falling back to the package-level allowedCommands.  The override is used only
// in tests to allow the test binary itself to be the executed program.
func (t *ExecuteBashCommandTool) effectiveAllowedCmds() map[string]bool {
	if t.allowedCmds != nil {
		return t.allowedCmds
	}
	return allowedCommands
}

// validateCommand checks that the (already-tokenized) command is permitted and
// safe.  Validation order:
//
//  1. Explicit allow-list (allowedCmds).
//  2. Blacklist defense-in-depth (blockedCmds).
//  3. Interpreter bypass flags for build/package tools.
//  4. rm recursive/directory-flag safety.
//
// Path-scope validation for rm and read-type commands is performed separately by
// validateRmPaths and validateSourcePaths after the working directory is resolved.
func validateCommand(args []string, command string, allowedCmds map[string]bool, blockedCmds map[string]bool) *tools.Result {
	// Normalize to the basename so that full-path invocations like /bin/rm or
	// /usr/bin/git are matched against the same allow/block lists as bare names.
	baseCommand := filepath.Base(args[0])

	// 1. Enforce the explicit allow-list before any other check.
	if !allowedCmds[baseCommand] {
		log.Warnf("Blocked command not in allow-list: %s", baseCommand)
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: %s", errUtils.ErrAICommandNotAllowed, baseCommand),
		}
	}

	// 2. Block dangerous subcommands for package managers, build tools, and VCS
	// binaries that can execute arbitrary code, download untrusted packages, or
	// perform write/network operations.
	if subBlocked, hasBlocked := blockedSubcommands[baseCommand]; hasBlocked && len(args) > 1 {
		sub := args[1]
		if subBlocked[sub] {
			log.Warnf("Blocked dangerous subcommand %q for command: %s", sub, command)
			return &tools.Result{
				Success: false,
				Error:   fmt.Errorf("%w: %s %s", errUtils.ErrAICommandBlacklisted, baseCommand, sub),
			}
		}
	}

	// 3. Defense-in-depth: deny explicitly blacklisted commands even if they are
	// ever added to allowedCmds by mistake.
	if blockedCmds[baseCommand] {
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

	// 4. Block inline-execution flags for tools that could otherwise be used as
	// interpreter escape hatches.
	if bypassCheckedCommands[baseCommand] {
		for _, arg := range args[1:] {
			if interpreterBypassFlags[arg] {
				log.Warnf("Blocked interpreter bypass flag %q for command: %s", arg, command)
				return &tools.Result{
					Success: false,
					Error:   fmt.Errorf("%w: %s %s", errUtils.ErrAICommandBlacklisted, baseCommand, arg),
				}
			}
		}
	}

	// For "go", also block stdin execution via "go run -".
	if baseCommand == "go" {
		hasRun := false
		for _, arg := range args[1:] {
			if arg == "run" {
				hasRun = true
			}
			if hasRun && arg == "-" {
				log.Warnf("Blocked go stdin execution: %s", command)
				return &tools.Result{
					Success: false,
					Error:   fmt.Errorf("%w: go run stdin", errUtils.ErrAICommandBlacklisted),
				}
			}
		}
	}

	// 5. Allow "rm" for single-file deletions, but block recursive and directory-removal flags.
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
// recursive flags.  Symlinks are resolved before the scope check to prevent
// symlink-based path traversal attacks.
func (t *ExecuteBashCommandTool) validateRmPaths(args []string, workingDir string) *tools.Result {
	basePath := t.atmosConfig.BasePath
	// Resolve symlinks in basePath and tmpDir so that the scope comparison works
	// correctly on systems where the temp directory contains symlinks
	// (e.g., /var → /private/var on macOS).
	if realBase, err := filepath.EvalSymlinks(basePath); err == nil {
		basePath = realBase
	}
	tmpDir := os.TempDir()
	if realTmp, err := filepath.EvalSymlinks(tmpDir); err == nil {
		tmpDir = realTmp
	}

	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "-") {
			// Handle --flag=value form: the value part may be a path (e.g., --backup=suffix
			// is not a path, but guard against future flags that embed paths).
			if eqIdx := strings.Index(arg, "="); eqIdx > 0 {
				val := arg[eqIdx+1:]
				if val != "" && isPathLike(val) {
					absPath := resolveArg(val, workingDir)
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
			}
			continue // flags are already checked by validateCommand.
		}

		absPath := resolveArg(arg, workingDir)

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

// validateSourcePaths checks that every non-flag path argument to read-type
// commands (cp, mv, cat, head, tail, diff, grep) resolves to a location within
// the tool's base path or the OS temporary directory.  This prevents the AI from
// reading sensitive system files such as /etc/passwd or /etc/shadow.
// Symlinks are resolved before the scope check to prevent path traversal attacks.
func (t *ExecuteBashCommandTool) validateSourcePaths(args []string, workingDir string) *tools.Result {
	basePath := t.atmosConfig.BasePath
	// Resolve symlinks in basePath and tmpDir so that the scope comparison works
	// correctly on systems where the temp directory contains symlinks
	// (e.g., /var → /private/var on macOS).
	if realBase, err := filepath.EvalSymlinks(basePath); err == nil {
		basePath = realBase
	}
	tmpDir := os.TempDir()
	if realTmp, err := filepath.EvalSymlinks(tmpDir); err == nil {
		tmpDir = realTmp
	}

	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "-") {
			// Handle --flag=value form: the value part may be a path that needs
			// scope validation (e.g., cp --target-directory=/etc/shadow src).
			if eqIdx := strings.Index(arg, "="); eqIdx > 0 {
				val := arg[eqIdx+1:]
				if val != "" && isPathLike(val) {
					absPath := resolveArg(val, workingDir)
					relToBase, errBase := filepath.Rel(basePath, absPath)
					relToTmp, errTmp := filepath.Rel(tmpDir, absPath)
					withinBase := errBase == nil && !strings.HasPrefix(relToBase, "..")
					withinTmp := errTmp == nil && !strings.HasPrefix(relToTmp, "..")
					if !withinBase && !withinTmp {
						log.Warnf("Blocked access to path outside allowed scope: %s (basePath=%s)", absPath, basePath)
						return &tools.Result{
							Success: false,
							Error:   errUtils.ErrAICommandPathNotAllowed,
						}
					}
				}
			}
			continue // flags are already validated by validateCommand.
		}

		// Positional arg: only apply scope check to path-like values to avoid
		// misclassifying search patterns or other non-path arguments.
		if !isPathLike(arg) {
			continue
		}

		absPath := resolveArg(arg, workingDir)

		relToBase, errBase := filepath.Rel(basePath, absPath)
		relToTmp, errTmp := filepath.Rel(tmpDir, absPath)

		withinBase := errBase == nil && !strings.HasPrefix(relToBase, "..")
		withinTmp := errTmp == nil && !strings.HasPrefix(relToTmp, "..")

		if !withinBase && !withinTmp {
			log.Warnf("Blocked access to path outside allowed scope: %s (basePath=%s)", absPath, basePath)
			return &tools.Result{
				Success: false,
				Error:   errUtils.ErrAICommandPathNotAllowed,
			}
		}
	}

	return nil
}

// resolveWorkingDir determines the working directory for command execution.
// The resolved directory must be within the tool's base path; if it is not,
// the base path is used as a fallback and a warning is logged.  Both the
// configured base path and the candidate working directory are symlink-resolved
// before the scope check so that the comparison works correctly on systems
// where paths contain symlinks (e.g., /var → /private/var on macOS).
func (t *ExecuteBashCommandTool) resolveWorkingDir(params map[string]interface{}) string {
	rawBase := t.atmosConfig.BasePath
	// Normalize the configured base path to its symlink-resolved, clean form.
	// If EvalSymlinks fails (e.g., the path does not yet exist), fall back to
	// filepath.Clean so the comparison is at least consistently canonicalized.
	normalizedBase := filepath.Clean(rawBase)
	if realBase, err := filepath.EvalSymlinks(normalizedBase); err == nil {
		normalizedBase = realBase
	}

	wd, ok := params["working_dir"].(string)
	if !ok || wd == "" {
		return rawBase
	}

	var resolved string
	if !filepath.IsAbs(wd) {
		// Relative paths are joined against normalizedBase so that subsequent
		// EvalSymlinks resolves the full canonical path correctly.
		resolved = filepath.Join(normalizedBase, wd)
	} else {
		resolved = wd
	}

	// Resolve symlinks to prevent symlink-based path traversal attacks.
	// Only resolve when the path actually exists; non-existent paths fall back
	// to rawBase via the Rel check below.
	if realPath, err := filepath.EvalSymlinks(resolved); err == nil {
		resolved = realPath
	} else {
		resolved = filepath.Clean(resolved)
	}

	// Validate that the resolved directory is within the normalized base path.
	rel, err := filepath.Rel(normalizedBase, resolved)
	if err != nil || strings.HasPrefix(rel, "..") {
		log.Warnf("Working directory %q is outside the base path %q; falling back to base path", resolved, rawBase)
		return rawBase
	}
	return resolved
}

// Execute runs the direct command and returns the output.
func (t *ExecuteBashCommandTool) Execute(ctx context.Context, params map[string]interface{}) (*tools.Result, error) {
	command, ok := params["command"].(string)
	if !ok || command == "" {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: command", errUtils.ErrAIToolParameterRequired),
		}, nil
	}

	// shell.Fields tokenizes the command with POSIX-like word splitting: full
	// quote handling (single/double quotes, backslash escapes) and rejection of
	// unquoted shell operators (;, &&, ||, |, >, <, &, $(...), backticks, etc.).
	// This eliminates shell-injection via operator smuggling while still allowing
	// operators inside quotes as literal text.
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

	// Reject environment variable references.  Since the command is executed
	// directly without a shell, $VAR would be passed literally as an argument,
	// which is almost certainly not what the caller intended.  Blocking it early
	// prevents confusion and potential information leakage via variable names.
	if containsUnquotedDollar(command) {
		log.Warnf("Rejected command with unquoted environment variable reference: %s", command)
		return &tools.Result{
			Success: false,
			Error:   errUtils.ErrAICommandVarExpansion,
		}, nil
	}

	if errResult := validateCommand(args, command, t.effectiveAllowedCmds(), blacklistedCommands); errResult != nil {
		return errResult, nil
	}

	workingDir := t.resolveWorkingDir(params)

	// Enforce path-scope for rm: targets must be within basePath or os.TempDir().
	if filepath.Base(args[0]) == "rm" {
		if errResult := t.validateRmPaths(args, workingDir); errResult != nil {
			return errResult, nil
		}
	}

	// Enforce path-scope for read-type commands: source and destination paths
	// must be within basePath or os.TempDir().
	if sourceScopedCommands[filepath.Base(args[0])] {
		if errResult := t.validateSourcePaths(args, workingDir); errResult != nil {
			return errResult, nil
		}
	}

	log.Debugf("Executing command: %s (in %s)", command, workingDir)

	// Apply a hard timeout to prevent runaway commands from consuming resources
	// indefinitely.  The parent context may impose a stricter deadline.
	execCtx, cancel := context.WithTimeout(ctx, t.commandTimeout)
	defer cancel()

	// Execute the command directly -- without a shell intermediary.
	// This eliminates shell-metacharacter interpretation (CWE-78) entirely:
	// args[0] is the binary, args[1:] are the literal arguments.
	cmd := exec.CommandContext(execCtx, args[0], args[1:]...)
	cmd.Dir = workingDir

	output, err := cmd.CombinedOutput()

	exitCode := -1 // sentinel: -1 means process never ran (launch failure)
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
	return true // All direct command execution requires user confirmation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *ExecuteBashCommandTool) IsRestricted() bool {
	return false // Permission system will handle per-command restrictions.
}
