package commandline

import (
	"fmt"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	comp "github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/schema"
)

// parseCompoundSubcommand checks if the arguments represent a terraform compound subcommand
// (a command with its own subcommand, e.g., "providers lock", "state list").
// It handles both quoted forms (e.g., "providers lock") and separate forms (e.g., "providers", "lock").
// Returns nil if no compound subcommand is detected.
func parseCompoundSubcommand(args []string) *compoundSubcommandResult {
	if len(args) == 0 {
		return nil
	}

	// First, check if the first argument is a quoted compound subcommand (e.g., "providers lock").
	firstArg := args[0]
	if strings.Contains(firstArg, " ") {
		if result := parseQuotedCompoundSubcommand(firstArg); result != nil {
			return result
		}
	}

	// If not a quoted command, check for separate word forms (e.g., "providers", "lock").
	if len(args) > 1 {
		return parseSeparateCompoundSubcommand(args[0], args[1])
	}

	return nil
}

// parseQuotedCompoundSubcommand parses a quoted compound subcommand like "providers lock".
// It is only called from parseCompoundSubcommand when strings.Contains(arg, " ") is true,
// which guarantees SplitN returns exactly 2 parts.  The defensive guard below protects
// against future refactoring that could call this function with a no-space string.
func parseQuotedCompoundSubcommand(arg string) *compoundSubcommandResult {
	parts := strings.SplitN(arg, " ", 2)
	// Defensive guard: this function is only called when arg contains a space
	// (see parseCompoundSubcommand), but this check protects against callers
	// that may not enforce that invariant in the future.
	if len(parts) != 2 {
		return nil
	}
	first, second := parts[0], parts[1]

	switch first {
	case cmdWrite:
		if second == cmdVarfile {
			return &compoundSubcommandResult{subCommand: cmdWrite, subCommand2: cmdVarfile, argCount: 1}
		}
	case cmdWorkspace:
		if slices.Contains(workspaceSubcommands, second) {
			return &compoundSubcommandResult{subCommand: cmdWorkspace, subCommand2: second, argCount: 1}
		}
	case cmdState:
		if slices.Contains(stateSubcommands, second) {
			return &compoundSubcommandResult{subCommand: fmt.Sprintf(cmdFmtSpaced, cmdState, second), argCount: 1}
		}
	case cmdProviders:
		if slices.Contains(providersSubcommands, second) {
			return &compoundSubcommandResult{subCommand: fmt.Sprintf(cmdFmtSpaced, cmdProviders, second), argCount: 1}
		}
	}

	return nil
}

// processTerraformCompoundSubcommand handles terraform compound subcommands (e.g., "providers lock").
// Returns true if a compound subcommand was processed, false otherwise.
func processTerraformCompoundSubcommand(info *schema.ArgsAndFlagsInfo, args []string) (bool, error) {
	result := parseCompoundSubcommand(args)
	if result == nil {
		return false, nil
	}

	info.SubCommand = result.subCommand
	info.SubCommand2 = result.subCommand2

	// The component argument is at index argCount.
	// For quoted commands like "providers lock", argCount=1, so component is at [1].
	// For separate commands like "providers" "lock", argCount=2, so component is at [2].
	componentArgIndex := result.argCount
	if len(args) <= componentArgIndex {
		return true, fmt.Errorf("%w: command %q requires an argument", errUtils.ErrMissingComponent, info.SubCommand)
	}

	info.ComponentFromArg = args[componentArgIndex]
	if comp.IsExplicitComponentPath(info.ComponentFromArg) {
		info.NeedsPathResolution = true
	}
	if len(args) > componentArgIndex+1 {
		info.AdditionalArgsAndFlags = args[componentArgIndex+1:]
	}

	return true, nil
}

// processSingleCommand handles parsing for single-word commands (non-two-word commands).
// It sets the subcommand, component, and additional args on the info struct.
func processSingleCommand(info *schema.ArgsAndFlagsInfo, args []string) error {
	info.SubCommand = args[0]
	if len(args) <= 1 {
		return nil
	}

	secondArg := args[1]
	if len(secondArg) == 0 {
		return fmt.Errorf("%w: invalid empty argument provided", errUtils.ErrInvalidArguments)
	}

	if strings.HasPrefix(secondArg, "--") {
		if len(secondArg) <= 2 {
			return fmt.Errorf("%w: invalid option format: %s", errUtils.ErrInvalidArguments, secondArg)
		}
		info.AdditionalArgsAndFlags = args[1:]
		return nil
	}

	info.ComponentFromArg = secondArg
	// Check if argument is an explicit path that needs resolution.
	// Only resolve as a filesystem path if the argument explicitly indicates a path:
	// - "." (current directory).
	// - Starts with "./" or "../" (relative path).
	// - Starts with "/" (absolute path).
	// Otherwise, treat it as a component name (even if it contains slashes).
	if comp.IsExplicitComponentPath(secondArg) {
		info.NeedsPathResolution = true
	}

	if len(args) > 2 {
		info.AdditionalArgsAndFlags = args[2:]
	}

	return nil
}

// parseSeparateCompoundSubcommand parses a compound subcommand passed as separate arguments.
func parseSeparateCompoundSubcommand(first, second string) *compoundSubcommandResult {
	switch first {
	case cmdWrite:
		if second == cmdVarfile {
			return &compoundSubcommandResult{subCommand: cmdWrite, subCommand2: cmdVarfile, argCount: 2}
		}
	case cmdWorkspace:
		if slices.Contains(workspaceSubcommands, second) {
			return &compoundSubcommandResult{subCommand: cmdWorkspace, subCommand2: second, argCount: 2}
		}
	case cmdState:
		if slices.Contains(stateSubcommands, second) {
			return &compoundSubcommandResult{subCommand: fmt.Sprintf(cmdFmtSpaced, cmdState, second), argCount: 2}
		}
	case cmdProviders:
		if slices.Contains(providersSubcommands, second) {
			return &compoundSubcommandResult{subCommand: fmt.Sprintf(cmdFmtSpaced, cmdProviders, second), argCount: 2}
		}
	}

	return nil
}
