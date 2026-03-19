package atmos

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/syntax"

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

// validateCommand parses the command string into a shell AST and validates that
// it contains exactly one top-level statement with no compound operators (&&, ||, ;),
// no substitutions ($(), backticks, process substitution), no I/O redirections,
// and no blacklisted binaries. Pipes (|, |&) are allowed with per-segment validation.
func validateCommand(command string) *tools.Result {
	parser := syntax.NewParser()
	file, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		log.Warnf("Failed to parse shell command: %s", command)
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: %s", errUtils.ErrAICommandParseFailed, err.Error()),
		}
	}

	if len(file.Stmts) == 0 {
		return &tools.Result{
			Success: false,
			Error:   errUtils.ErrAICommandEmpty,
		}
	}

	// Reject multiple statements (semicolons, newlines separating commands).
	if len(file.Stmts) > 1 {
		log.Warnf("Blocked compound command (multiple statements): %s", command)
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: multiple statements are not permitted", errUtils.ErrAICommandCompoundNotAllowed),
		}
	}

	return validateStmt(file.Stmts[0], command)
}

// validateStmt validates a single statement, rejecting I/O redirections,
// background execution, negation, and coprocesses.
func validateStmt(stmt *syntax.Stmt, command string) *tools.Result {
	if len(stmt.Redirs) > 0 {
		log.Warnf("Blocked command with I/O redirection: %s", command)
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: I/O redirection (>, >>, <, heredoc) is not permitted", errUtils.ErrAICommandCompoundNotAllowed),
		}
	}

	if stmt.Negated {
		log.Warnf("Blocked negated command: %s", command)
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: command negation (!) is not permitted", errUtils.ErrAICommandCompoundNotAllowed),
		}
	}

	if stmt.Background {
		log.Warnf("Blocked background command: %s", command)
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: background execution (&) is not permitted", errUtils.ErrAICommandCompoundNotAllowed),
		}
	}

	if stmt.Coprocess {
		log.Warnf("Blocked coprocess command: %s", command)
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: coprocess execution is not permitted", errUtils.ErrAICommandCompoundNotAllowed),
		}
	}

	return validateCommandNode(stmt.Cmd, command)
}

// validateCommandNode dispatches validation based on the AST command type.
// Only simple commands (CallExpr) and pipe chains (BinaryCmd with Pipe/PipeAll)
// are allowed. All other command structures (if/while/for/case clauses, blocks,
// function declarations, arithmetic commands, etc.) are rejected by default.
func validateCommandNode(cmd syntax.Command, command string) *tools.Result {
	switch node := cmd.(type) {
	case *syntax.CallExpr:
		return validateCallExpr(node, command)
	case *syntax.BinaryCmd:
		return validateBinaryCmd(node, command)
	case *syntax.Subshell:
		log.Warnf("Blocked subshell: %s", command)
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: subshells are not permitted", errUtils.ErrAICommandCompoundNotAllowed),
		}
	default:
		log.Warnf("Blocked unsupported command type: %s", command)
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: unsupported command structure", errUtils.ErrAICommandCompoundNotAllowed),
		}
	}
}

// validateBinaryCmd validates binary command operators. Pipes (|, |&) are allowed
// with per-side validation; control operators (&&, ||) are blocked because
// they enable chaining arbitrary commands after an allowed one.
func validateBinaryCmd(node *syntax.BinaryCmd, command string) *tools.Result {
	if node.Op != syntax.Pipe && node.Op != syntax.PipeAll {
		log.Warnf("Blocked binary command operator (%s): %s", node.Op, command)
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: operator %s is not permitted", errUtils.ErrAICommandCompoundNotAllowed, node.Op),
		}
	}

	// For pipes, recursively validate both sides as full statements
	// to catch redirections, background, etc. on pipe segments.
	if result := validateStmt(node.X, command); result != nil {
		return result
	}
	return validateStmt(node.Y, command)
}

// validateCallExpr validates a simple command (CallExpr), checking the command
// name against the blacklist and scanning arguments for substitutions.
func validateCallExpr(call *syntax.CallExpr, command string) *tools.Result {
	// Reject environment variable assignments prefixed to commands (e.g., PATH=/evil ls)
	// because they can manipulate PATH, LD_PRELOAD, etc.
	if len(call.Assigns) > 0 && len(call.Args) > 0 {
		log.Warnf("Blocked command with environment variable override: %s", command)
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: environment variable assignments before commands are not permitted", errUtils.ErrAICommandCompoundNotAllowed),
		}
	}

	if len(call.Args) == 0 {
		// No arguments means an empty or assignment-only command (e.g., VAR=value).
		return nil
	}

	cmdName, isStatic := extractCommandName(call.Args[0])
	if !isStatic {
		log.Warnf("Blocked command with dynamic name: %s", command)
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: dynamic command names are not permitted", errUtils.ErrAICommandCompoundNotAllowed),
		}
	}

	// Check the base name against the blacklist (catches /usr/bin/rm -> rm).
	baseName := filepath.Base(cmdName)
	if blacklistedCommands[baseName] {
		log.Warnf("Blocked blacklisted command: %s", baseName)
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: %s", errUtils.ErrAICommandBlacklisted, baseName),
		}
	}

	// Check for mkfs.* variants.
	if strings.HasPrefix(baseName, "mkfs.") {
		log.Warnf("Blocked mkfs variant command: %s", baseName)
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: %s", errUtils.ErrAICommandBlacklisted, baseName),
		}
	}

	// Scan all arguments for command/process substitution.
	for _, word := range call.Args {
		if result := validateWordParts(word, command); result != nil {
			return result
		}
	}

	return nil
}

// extractCommandName extracts the command name string from a Word AST node,
// concatenating only literal parts. Returns false if any non-literal parts
// (variable expansions, parameter expansions) are found, since dynamically
// constructed command names could bypass the blacklist.
func extractCommandName(word *syntax.Word) (string, bool) {
	var parts []string
	for _, part := range word.Parts {
		lit, ok := part.(*syntax.Lit)
		if !ok {
			return "", false
		}
		parts = append(parts, lit.Value)
	}
	return strings.Join(parts, ""), true
}

// validateWordParts inspects all parts of a Word for dangerous substitutions
// including command substitution ($(), backticks) and process substitution (<(), >()).
func validateWordParts(word *syntax.Word, command string) *tools.Result {
	for _, part := range word.Parts {
		if result := checkPart(part, command); result != nil {
			return result
		}
	}
	return nil
}

// checkPart recursively checks a single word part for dangerous substitutions.
// Safe types (Lit, SglQuoted, ExtGlob, ArithmExp) do not contain nested command
// execution and are allowed through.
func checkPart(part syntax.WordPart, command string) *tools.Result {
	switch node := part.(type) {
	case *syntax.CmdSubst:
		log.Warnf("Blocked command substitution: %s", command)
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: command substitution ($() or backticks) is not permitted", errUtils.ErrAICommandCompoundNotAllowed),
		}
	case *syntax.ProcSubst:
		log.Warnf("Blocked process substitution: %s", command)
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: process substitution is not permitted", errUtils.ErrAICommandCompoundNotAllowed),
		}
	case *syntax.DblQuoted:
		// Check inside double-quoted strings for embedded substitutions.
		for _, inner := range node.Parts {
			if result := checkPart(inner, command); result != nil {
				return result
			}
		}
	case *syntax.ParamExp:
		// Check for nested command substitution inside parameter expansions.
		if node.NestedParam != nil {
			if result := checkPart(node.NestedParam, command); result != nil {
				return result
			}
		}
	}
	return nil
}

// resolveWorkingDir determines the working directory for command execution.
func (t *ExecuteBashCommandTool) resolveWorkingDir(params map[string]any) string {
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
func (t *ExecuteBashCommandTool) Execute(ctx context.Context, params map[string]any) (*tools.Result, error) {
	command, ok := params["command"].(string)
	if !ok || command == "" {
		return &tools.Result{
			Success: false,
			Error:   fmt.Errorf("%w: command", errUtils.ErrAIToolParameterRequired),
		}, nil
	}

	if errResult := validateCommand(command); errResult != nil {
		return errResult, nil
	}

	workingDir := t.resolveWorkingDir(params)

	log.Debugf("Executing shell command: %s (in %s)", command, workingDir)

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = workingDir

	output, err := cmd.CombinedOutput()

	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}

	result := &tools.Result{
		Success: err == nil,
		Output:  string(output),
		Data: map[string]any{
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
