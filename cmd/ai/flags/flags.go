package flags

import (
	"os"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// HasAIFlag checks if --ai flag is present in os.Args.
// This is needed before Cobra parses flags because we set up output capture
// in Execute() before internal.Execute() runs the command.
func HasAIFlag() bool {
	defer perf.Track(nil, "flags.HasAIFlag")()

	return HasAIFlagInternal(os.Args)
}

// HasAIFlagInternal checks if --ai flag is present in the provided args.
// Respects CLI precedence: if --ai or --ai=<value> is explicitly provided, that value wins.
// Only falls back to ATMOS_AI environment variable when no CLI flag is present.
func HasAIFlagInternal(args []string) bool {
	for _, arg := range args {
		// Stop scanning after bare "--" (end-of-flags delimiter).
		if arg == "--" {
			break
		}
		// Bare --ai is equivalent to --ai=true.
		if arg == "--ai" {
			return true
		}
		// Explicit --ai=<value>: parse the boolean to respect --ai=false.
		if strings.HasPrefix(arg, "--ai=") {
			val, err := strconv.ParseBool(strings.TrimPrefix(arg, "--ai="))
			if err != nil {
				return false
			}
			return val
		}
	}
	// Fall back to ATMOS_AI environment variable for CI/CD env-only usage.
	//nolint:forbidigo // Must use os.Getenv: AI flag is processed before Viper configuration loads.
	val, err := strconv.ParseBool(os.Getenv("ATMOS_AI"))
	return err == nil && val
}

// ParseSkillFlag extracts all --skill flag values from os.Args.
// This is needed before Cobra parses flags because we validate and load skills
// in Execute() before internal.Execute() runs the command.
func ParseSkillFlag() []string {
	defer perf.Track(nil, "flags.ParseSkillFlag")()

	return ParseSkillFlagInternal(os.Args)
}

// ParseSkillFlagInternal extracts all --skill flag values from the provided args.
// Supports repeated flags (--skill a --skill b) and comma-separated values (--skill a,b).
// Respects CLI precedence: if --skill is explicitly provided (even as empty), the env var is not consulted.
// Only falls back to ATMOS_SKILL environment variable when no --skill flag is present.
func ParseSkillFlagInternal(args []string) []string {
	var result []string
	flagSeen := false
	for i, arg := range args {
		// Stop scanning after bare "--" (end-of-flags delimiter).
		if arg == "--" {
			break
		}

		var value string
		if arg == "--skill" && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			value = args[i+1]
			flagSeen = true
		} else if strings.HasPrefix(arg, "--skill=") {
			value = strings.TrimPrefix(arg, "--skill=")
			flagSeen = true
		}

		if value != "" {
			result = append(result, SplitCSV(value)...)
		}
	}

	// Fall back to ATMOS_SKILL environment variable only when no --skill CLI flag was provided.
	if !flagSeen {
		//nolint:forbidigo // Must use os.Getenv: skill flag is processed before Viper configuration loads.
		result = SplitCSV(os.Getenv("ATMOS_SKILL"))
	}

	return result
}

// SplitCSV splits a comma-separated string into trimmed, non-empty values.
func SplitCSV(value string) []string {
	var result []string
	for _, v := range strings.Split(value, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			result = append(result, v)
		}
	}
	return result
}
