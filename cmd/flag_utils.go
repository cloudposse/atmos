package cmd

import (
	"github.com/spf13/cobra"
)

// SeparatedArgs represents parsed arguments split by the -- separator.
// This follows the kubectl pattern where flags before -- are parsed by Cobra,
// and arguments after -- are passed through to a subprocess or subcommand.
//
// Example: atmos auth exec --identity admin -- terraform apply -auto-approve
//
//	BeforeDash: ["--identity", "admin"]  (parsed by Cobra into flags)
//	AfterDash:  ["terraform", "apply", "-auto-approve"]  (passed to subprocess)
type SeparatedArgs struct {
	// AfterDash contains all arguments after the -- separator.
	// These are typically passed through to a subprocess or external command.
	// Returns nil if no -- separator was found.
	AfterDash []string

	// ArgsLenAtDash is the position where -- was found in the original args.
	// Returns -1 if no -- separator was present.
	// This is useful for validation or error messages.
	ArgsLenAtDash int
}

// ParseSeparatedArgs extracts arguments after the -- separator using Cobra's built-in ArgsLenAtDash().
// This is the standard pattern used by kubectl and other tools that need to separate their own flags
// from arguments passed to a subprocess.
//
// Usage:
//
//	func myCommandRun(cmd *cobra.Command, args []string) error {
//	    separated := ParseSeparatedArgs(cmd, args)
//
//	    // Cobra has already parsed your command's flags
//	    myFlag, _ := cmd.Flags().GetString("my-flag")
//
//	    // Get arguments to pass to subprocess
//	    if len(separated.AfterDash) == 0 {
//	        return fmt.Errorf("no command specified after --")
//	    }
//
//	    // Execute subprocess with separated.AfterDash
//	    return executeCommand(separated.AfterDash)
//	}
//
// Command setup:
//
//	cmd := &cobra.Command{
//	    Use: "mycommand [flags] -- COMMAND [args...]",
//	    // NOTE: Do NOT use DisableFlagParsing
//	    RunE: myCommandRun,
//	}
//	cmd.Flags().String("my-flag", "", "My command's flag")
//
// The key insight is that Cobra's normal flag parsing works correctly with --:
//   - Flags before -- are parsed by Cobra
//   - Arguments after -- are available via ArgsLenAtDash()
//   - No custom parsing required
func ParseSeparatedArgs(cmd *cobra.Command, args []string) *SeparatedArgs {
	result := &SeparatedArgs{
		ArgsLenAtDash: cmd.ArgsLenAtDash(),
	}

	// If -- was found, extract everything after it.
	if result.ArgsLenAtDash >= 0 {
		result.AfterDash = args[result.ArgsLenAtDash:]
	}

	return result
}

// HasSeparator returns true if the -- separator was present in the arguments.
func (s *SeparatedArgs) HasSeparator() bool {
	return s.ArgsLenAtDash >= 0
}

// CommandArgs returns the arguments after --, or nil if no separator was found.
// This is a convenience method equivalent to accessing AfterDash directly.
func (s *SeparatedArgs) CommandArgs() []string {
	return s.AfterDash
}
