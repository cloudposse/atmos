package cmd

import (
	"fmt"
	"strings"

	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"mvdan.cc/sh/v3/syntax"
)

// SeparatedCommandArgs represents arguments split by the -- separator.
// This pattern is used across multiple Atmos commands (terraform, helmfile, packer,
// custom commands, auth exec/shell) to separate Atmos-specific flags from native
// command flags.
//
// Pattern:
//   atmos <command> <atmos-flags> -- <native-flags>
//
// Examples:
//   atmos terraform plan myapp -s dev -- -var foo=bar
//   atmos auth exec --identity admin -- terraform apply -auto-approve
//   atmos helmfile apply -s prod -- --set image.tag=v1.0
type SeparatedCommandArgs struct {
	// BeforeSeparator contains arguments before the -- marker.
	// These are processed by Atmos (stack, component, identity, etc.)
	BeforeSeparator []string

	// AfterSeparator contains arguments after the -- marker.
	// These are passed through to the native command (terraform, helmfile, subprocess, etc.)
	AfterSeparator []string

	// SeparatorIndex is the position of -- in os.Args, or -1 if not found.
	SeparatorIndex int

	// HasSeparator returns true if -- was found in the arguments.
	HasSeparator bool
}

// ExtractSeparatedArgs extracts arguments before and after the -- separator.
// This is the unified implementation used by all Atmos commands that support
// the -- end-of-args pattern.
//
// Why we need os.Args:
//   When DisableFlagParsing=true, Cobra consumes the -- separator and we lose
//   information about where it was. We need os.Args to find the original position
//   and extract trailing args correctly.
//
// Parameters:
//   - cmd: The Cobra command (may have DisableFlagParsing=true)
//   - args: The args passed to RunE (after Cobra processing)
//   - osArgs: Usually os.Args - the raw command line arguments
//
// Usage:
//
//	func myCommandRun(cmd *cobra.Command, args []string) error {
//	    separated := ExtractSeparatedArgs(cmd, args, os.Args)
//
//	    if separated.HasSeparator {
//	        // Process Atmos flags from separated.BeforeSeparator
//	        // Pass native flags from separated.AfterSeparator
//	    }
//	}
func ExtractSeparatedArgs(cmd *cobra.Command, args []string, osArgs []string) *SeparatedCommandArgs {
	// Find the position of -- in os.Args.
	separatorIndex := lo.IndexOf(osArgs, "--")

	result := &SeparatedCommandArgs{
		SeparatorIndex: separatorIndex,
		HasSeparator:   separatorIndex >= 0,
	}

	if !result.HasSeparator {
		// No separator found - all args are "before separator".
		result.BeforeSeparator = args
		result.AfterSeparator = nil
		return result
	}

	// Split os.Args at the separator.
	// Everything before -- (excluding the -- itself).
	argsBeforeSep := lo.Slice(osArgs, 0, separatorIndex)

	// Everything after -- (excluding the -- itself).
	result.AfterSeparator = lo.Slice(osArgs, separatorIndex+1, len(osArgs))

	// Now we need to filter the "args" slice to only include items that appear
	// in argsBeforeSep. This handles the case where Cobra has already processed
	// some args and removed recognized flags.
	//
	// Build a lookup map for quick existence check.
	lookup := make(map[string]bool)
	for _, val := range argsBeforeSep {
		lookup[val] = true
	}

	// Preserve order from args, but only include items that were before --.
	result.BeforeSeparator = make([]string, 0, len(args))
	for _, val := range args {
		if lookup[val] {
			result.BeforeSeparator = append(result.BeforeSeparator, val)
		}
	}

	return result
}

// GetAfterSeparator returns arguments after --, or nil if no separator.
// This is a convenience method for commands that only care about trailing args.
func (s *SeparatedCommandArgs) GetAfterSeparator() []string {
	return s.AfterSeparator
}

// GetBeforeSeparator returns arguments before --, or all args if no separator.
func (s *SeparatedCommandArgs) GetBeforeSeparator() []string {
	return s.BeforeSeparator
}

// GetAfterSeparatorAsString returns arguments after -- joined as a space-separated string.
// Returns empty string if no separator or no trailing args.
//
// WARNING: This method uses plain strings.Join() which does NOT preserve argument boundaries
// or whitespace when the result is later parsed by a shell. For shell-safe quoting that
// preserves whitespace and special characters, use GetAfterSeparatorAsQuotedString() instead.
//
// This method exists for backwards compatibility and non-shell use cases.
func (s *SeparatedCommandArgs) GetAfterSeparatorAsString() string {
	if !s.HasSeparator || len(s.AfterSeparator) == 0 {
		return ""
	}
	return strings.Join(s.AfterSeparator, " ")
}

// GetAfterSeparatorAsQuotedString returns arguments after -- as a shell-quoted string.
// Each argument is properly quoted for shell safety, preserving whitespace and special characters.
// Returns empty string if no separator or no trailing args.
//
// This is the CORRECT method to use for custom commands that pass trailing args to shell execution,
// as it ensures argument boundaries and whitespace are preserved when the shell re-parses the string.
//
// Example:
//   args after --: ["echo", "hello  world"]  (note: two spaces)
//   GetAfterSeparatorAsString():       "echo hello  world"  (when shell re-parses: loses boundary!)
//   GetAfterSeparatorAsQuotedString(): "echo 'hello  world'" (shell correctly preserves!)
func (s *SeparatedCommandArgs) GetAfterSeparatorAsQuotedString() (string, error) {
	if !s.HasSeparator || len(s.AfterSeparator) == 0 {
		return "", nil
	}

	var quotedArgs []string
	for _, arg := range s.AfterSeparator {
		quoted, err := syntax.Quote(arg, syntax.LangBash)
		if err != nil {
			return "", fmt.Errorf("failed to quote argument %q: %w", arg, err)
		}
		quotedArgs = append(quotedArgs, quoted)
	}

	return strings.Join(quotedArgs, " "), nil
}
