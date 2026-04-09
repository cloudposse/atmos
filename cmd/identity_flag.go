package cmd

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// GetIdentityFromFlags retrieves the identity value from command-line flags and environment variables.
// This function handles the Cobra NoOptDefVal quirk where --identity <value> with positional args
// can be misinterpreted as --identity (without value).
//
// Returns:
//   - identity value if explicitly provided
//   - cfg.IdentityFlagSelectValue if --identity was used without a value (interactive selection)
//   - cfg.IdentityFlagDisabledValue if --identity=false (authentication disabled)
//   - value from ATMOS_IDENTITY env var if flag not provided
//   - empty string if no identity specified anywhere
//
// Usage:
//
//	identity := GetIdentityFromFlags(cmd, os.Args)
//	if identity == cfg.IdentityFlagSelectValue {
//	    // Show interactive selector
//	} else if identity == cfg.IdentityFlagDisabledValue {
//	    // Skip authentication
//	} else if identity != "" {
//	    // Use explicit identity
//	}
func GetIdentityFromFlags(cmd *cobra.Command, osArgs []string) string {
	// Check if flag was set via Cobra first (handles both SetArgs in tests and real CLI).
	// For commands without positional args, this is reliable.
	// For commands with positional args, we'll use os.Args parsing as a fallback.
	if cmd.Flags().Changed(IdentityFlagName) {
		value, _ := cmd.Flags().GetString(IdentityFlagName)
		// Only trust this value if it's not the NoOptDefVal issue.
		// If we got "__SELECT__" but there might be a real value in os.Args, check os.Args.
		if value != cfg.IdentityFlagSelectValue {
			return normalizeIdentityValue(value)
		}
		// Got __SELECT__ - check if os.Args has an actual value.
		identity := extractIdentityFromArgs(osArgs)
		if identity != "" && identity != cfg.IdentityFlagSelectValue {
			// Found actual value in os.Args - use that instead.
			return normalizeIdentityValue(identity)
		}
		// No value in os.Args either - return __SELECT__.
		return value
	}

	// Flag not changed - fall back to environment variable.
	envValue := viper.GetString(IdentityFlagName)
	return normalizeIdentityValue(envValue)
}

// normalizeIdentityValue converts boolean false representations to the disabled sentinel value.
// Recognizes: false, False, FALSE, 0, no, No, NO, off, Off, OFF.
// All other values are returned unchanged.
//
// Deprecated: Use cfg.NormalizeIdentityValue() instead. This wrapper exists for backward compatibility.
func normalizeIdentityValue(value string) string {
	return cfg.NormalizeIdentityValue(value)
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

// CreateAuthManagerFromIdentity creates and authenticates an AuthManager from an identity name.
// Returns nil if identityName is empty (no authentication requested).
// Returns error if identityName is provided but auth is not configured in atmos.yaml.
// This helper reduces nested complexity in describe commands.
//
// This function delegates to auth.CreateAndAuthenticateManager to ensure consistent
// authentication behavior across CLI commands and internal execution logic.
//
// Note: This function does not load stack configs for default identities.
// Use CreateAuthManagerFromIdentityWithAtmosConfig if you need stack-level default identity resolution.
func CreateAuthManagerFromIdentity(
	identityName string,
	authConfig *schema.AuthConfig,
) (auth.AuthManager, error) {
	return auth.CreateAndAuthenticateManager(identityName, authConfig, IdentityFlagSelectValue)
}

// CreateAuthManagerFromIdentityWithAtmosConfig creates and authenticates an AuthManager from an
// identity name using a pre-merged auth config.
//
// **This is the NO-SCAN wrapper** — it delegates to `auth.CreateAndAuthenticateManagerWithAtmosConfig`
// and never runs the global stack-file pre-scanner. Use this for Category A callers that have
// already merged the target stack's auth section via `ExecuteDescribeComponent` /
// `MergeComponentAuthFromConfig` (e.g. `atmos describe component`).
//
// For Category B callers (multi-stack commands with no specific target), use
// `CreateAuthManagerFromIdentityWithStackScan` instead. Using the scan variant on a Category A
// caller would reintroduce the Discussion #122 cross-stack leak. See
// `docs/fixes/2026-04-08-atmos-auth-identity-resolution-fixes.md` for the full design rationale.
func CreateAuthManagerFromIdentityWithAtmosConfig(
	identityName string,
	authConfig *schema.AuthConfig,
	atmosConfig *schema.AtmosConfiguration,
) (auth.AuthManager, error) {
	return auth.CreateAndAuthenticateManagerWithAtmosConfig(identityName, authConfig, IdentityFlagSelectValue, atmosConfig)
}

// CreateAuthManagerFromIdentityWithStackScan creates and authenticates an AuthManager, first
// running the global stack-file pre-scanner (Approach 2) to discover stack-level default
// identities declared in `auth.identities.<name>.default: true`.
//
// **This is the SCAN wrapper** — it delegates to `auth.CreateAndAuthenticateManagerWithStackScan`.
// Use it for Category B commands that legitimately have no target `(component, stack)` pair and
// therefore cannot use the exec-layer merge path: `atmos describe stacks`,
// `atmos describe affected`, `atmos describe dependents`, `atmos list affected`,
// `atmos list instances`, `atmos aws security`, `atmos aws compliance`, and workflow execution.
//
// The scanner now follows `import:` chains, so stack-level defaults declared inside an imported
// `_defaults.yaml` are visible even when that file is listed under `excluded_paths` — fixing
// Issue #2293 for multi-stack commands. Conflict handling is unchanged: when two stacks declare
// different defaults, both are discarded (matches Issue #2072's `allAgree` behavior).
//
// Category A callers (terraform/helmfile/describe component/nested auth) must NOT use this
// variant. Running the scanner on top of a stack-scoped merged config reintroduces Discussion
// #122's cross-stack leak.
func CreateAuthManagerFromIdentityWithStackScan(
	identityName string,
	authConfig *schema.AuthConfig,
	atmosConfig *schema.AtmosConfiguration,
) (auth.AuthManager, error) {
	return auth.CreateAndAuthenticateManagerWithStackScan(identityName, authConfig, IdentityFlagSelectValue, atmosConfig)
}
