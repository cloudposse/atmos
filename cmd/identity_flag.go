package cmd

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

// GetIdentityFromFlags retrieves the identity value from command-line flags and environment variables.
// This function handles the Cobra NoOptDefVal quirk where --identity <value> with positional args
// can be misinterpreted as --identity (without value).
//
// Returns:
//   - identity value if explicitly provided
//   - cfg.IdentityFlagSelectValue if --identity was used without a value (interactive selection)
//   - value from ATMOS_IDENTITY env var if flag not provided
//   - empty string if no identity specified anywhere
//
// Usage:
//
//	identity := GetIdentityFromFlags(cmd, os.Args)
//	if identity == cfg.IdentityFlagSelectValue {
//	    // Show interactive selector
//	} else if identity != "" {
//	    // Use explicit identity
//	}
func GetIdentityFromFlags(cmd *cobra.Command, osArgs []string) string {
	// First, try to get identity from manual parsing of os.Args.
	// This bypasses Cobra's NoOptDefVal issue with positional arguments.
	identity := extractIdentityFromArgs(osArgs)
	if identity != "" {
		return identity
	}

	// If not found in os.Args, check if flag was set via Cobra.
	// This handles cases where the command doesn't have positional args
	// or where the flag appears before positional args.
	if cmd.Flags().Changed(IdentityFlagName) {
		value, _ := cmd.Flags().GetString(IdentityFlagName)
		return value
	}

	// Fall back to environment variable.
	return viper.GetString(IdentityFlagName)
}

// extractIdentityFromArgs manually parses os.Args to find --identity flag and its value.
// This is necessary because Cobra's NoOptDefVal behavior causes it to misinterpret
// "--identity value" as "--identity" (without value) when positional args are present.
//
// Handles three cases:
//   - --identity value (space-separated) -> returns "value"
//   - --identity=value (equals sign) -> returns "value"
//   - --identity (no value) -> returns cfg.IdentityFlagSelectValue
//
// Returns empty string if --identity flag is not present in args.
func extractIdentityFromArgs(args []string) string {
	for i, arg := range args {
		// Handle --identity=value format.
		if strings.HasPrefix(arg, cfg.IdentityFlag+"=") {
			value := strings.TrimPrefix(arg, cfg.IdentityFlag+"=")
			if value == "" {
				// --identity= (empty value) -> interactive selection.
				return cfg.IdentityFlagSelectValue
			}
			return value
		}

		// Handle -i=value format (short flag).
		if strings.HasPrefix(arg, "-i=") {
			value := strings.TrimPrefix(arg, "-i=")
			if value == "" {
				// -i= (empty value) -> interactive selection.
				return cfg.IdentityFlagSelectValue
			}
			return value
		}

		// Handle --identity value format (space-separated).
		if arg == cfg.IdentityFlag || arg == "-i" {
			// Check if next arg exists and is not another flag.
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				// Has value: --identity <value>.
				return args[i+1]
			}
			// No value: --identity (interactive selection).
			return cfg.IdentityFlagSelectValue
		}
	}

	// Flag not found in args.
	return ""
}
