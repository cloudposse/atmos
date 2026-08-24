package ui

import (
	"strings"
)

// hasJSONFlag checks if the -json flag is present in arguments.
func hasJSONFlag(args []string) bool {
	for _, arg := range args {
		if arg == flagJSON || strings.HasPrefix(arg, flagJSON+"=") {
			return true
		}
	}
	return false
}

// hasCompactWarningsFlag checks if the -compact-warnings flag is present.
func hasCompactWarningsFlag(args []string) bool {
	for _, arg := range args {
		if arg == flagCompactWarnings {
			return true
		}
	}
	return false
}

// isJSONSubCommand returns true if the argument is a terraform subcommand that supports -json.
func isJSONSubCommand(arg string) bool {
	return arg == subCommandPlan || arg == subCommandApply || arg == subCommandInit || arg == "refresh"
}

// buildRequiredFlags builds the list of flags that need to be added.
func buildRequiredFlags(hasJSON, hasCompactWarnings bool) []string {
	var flags []string
	if !hasJSON {
		flags = append(flags, flagJSON)
	}
	if !hasCompactWarnings {
		flags = append(flags, flagCompactWarnings)
	}
	return flags
}

// buildArgsWithJSON adds the -json and -compact-warnings flags to the arguments.
// -json enables structured JSON output for parsing.
// -compact-warnings suppresses verbose human-readable warnings since we route
// diagnostics through the Atmos logger via parsed JSON.
func buildArgsWithJSON(args []string) []string {
	hasJSON := hasJSONFlag(args)
	hasCompactWarnings := hasCompactWarningsFlag(args)

	// If both flags are already present, return as-is.
	if hasJSON && hasCompactWarnings {
		return args
	}

	requiredFlags := buildRequiredFlags(hasJSON, hasCompactWarnings)

	// Find the position to insert flags (after the subcommand).
	var result []string
	inserted := false

	for i, arg := range args {
		result = append(result, arg)
		// Insert flags after the subcommand (plan, apply, init, refresh).
		if i == 0 && isJSONSubCommand(arg) {
			result = append(result, requiredFlags...)
			inserted = true
		}
	}

	// If no subcommand was found at position 0, just prepend flags.
	if !inserted {
		return append(requiredFlags, args...)
	}

	return result
}

// extractOutFlag checks if -out flag is present and returns its value.
func extractOutFlag(args []string) string {
	for i, arg := range args {
		if arg == "-out" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(arg, "-out=") {
			return strings.TrimPrefix(arg, "-out=")
		}
	}
	return ""
}

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func extractPlanFile(args []string) string {
	// Look for a planfile argument (positional argument that's a file path).
	// Also check for --from-plan or --planfile flags.
	for i, arg := range args {
		if arg == "--from-plan" || arg == "--planfile" {
			if i+1 < len(args) {
				return args[i+1]
			}
		}
		if strings.HasPrefix(arg, "--from-plan=") {
			return strings.TrimPrefix(arg, "--from-plan=")
		}
		if strings.HasPrefix(arg, "--planfile=") {
			return strings.TrimPrefix(arg, "--planfile=")
		}
	}

	// Check for a positional planfile. Terraform's apply accepts any filename for a
	// saved plan (the ".tfplan" convention is not required), so any non-flag final
	// argument qualifies — except one that looks like Terraform configuration or
	// variables source (.tf, .tf.json, .tfvars, .tfvars.json), which Terraform
	// itself warns against using as a plan-file name because it would otherwise be
	// parsed as config source rather than a saved plan.
	if len(args) > 1 {
		lastArg := args[len(args)-1]
		if !strings.HasPrefix(lastArg, "-") && !looksLikeTerraformConfigFile(lastArg) {
			return lastArg
		}
	}

	return ""
}

// looksLikeTerraformConfigFile reports whether name has an extension Terraform
// treats as configuration or variables source, rather than a saved plan file.
func looksLikeTerraformConfigFile(name string) bool {
	for _, ext := range []string{".tf", ".tf.json", ".tfvars", ".tfvars.json"} {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

func buildPlanArgs(args []string, planFile string) []string {
	// Convert apply args to plan args.
	// Let append handle capacity growth to avoid integer overflow concerns.
	var result []string

	// Replace "apply" with "plan".
	for i, arg := range args {
		switch {
		case i == 0 && arg == subCommandApply:
			result = append(result, subCommandPlan)
		case arg == flagAutoApprove:
			// Skip -auto-approve for plan.
			continue
		default:
			result = append(result, arg)
		}
	}

	// Add -out flag.
	result = append(result, "-out="+planFile)

	return result
}

func buildApplyArgs(planFile string) []string {
	// Simple apply with planfile - no -auto-approve needed since planfile is provided.
	return []string{subCommandApply, planFile}
}

func buildDestroyPlanArgs(args []string, planFile string) []string {
	// Convert destroy args to plan -destroy args.
	// Let append handle capacity growth to avoid integer overflow concerns.
	var result []string

	// Replace "destroy" with "plan" and add -destroy flag.
	for i, arg := range args {
		switch {
		case i == 0 && arg == subCommandDestroy:
			result = append(result, subCommandPlan, "-destroy")
		case arg == flagAutoApprove:
			// Skip -auto-approve for plan.
			continue
		default:
			result = append(result, arg)
		}
	}

	// Add -out flag.
	result = append(result, "-out="+planFile)

	return result
}
