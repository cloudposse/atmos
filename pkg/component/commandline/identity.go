package commandline

import (
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

// getExplicitIdentityFlagValue returns the normalized identity value if the identity flag
// was explicitly set on the Cobra command.
func getExplicitIdentityFlagValue(cmd *cobra.Command) (string, bool) {
	if cmd == nil {
		return "", false
	}

	flag := cmd.Flag("identity")
	if flag == nil || !flag.Changed {
		return "", false
	}

	value, err := cmd.Flags().GetString("identity")
	if err != nil {
		return "", false
	}

	// When the user explicitly passes an empty flag value (e.g., `--identity=`), Cobra reports
	// the flag as Changed with an empty string. Treat this as an explicit request for the
	// interactive-selection sentinel so the ATMOS_IDENTITY env var fallback cannot override it.
	if value == "" {
		return cfg.IdentityFlagSelectValue, true
	}

	return cfg.NormalizeIdentityValue(value), true
}

// resolveIdentityValue determines the effective identity with precedence:
// 1. The currently parsed Cobra flag state.
// 2. Any identity already parsed on the command before ProcessCommandLineArgs was called.
// 3. The legacy raw-args parser output.
//
// The raw-args value is still needed to recover the explicit value when Cobra reports the
// NoOptDefVal sentinel for space-separated syntax (`--identity value` / `-i value`).
func resolveIdentityValue(
	argsIdentity string,
	preParsedIdentityValue string,
	preParsedIdentitySet bool,
	postParsedIdentityValue string,
	postParsedIdentitySet bool,
) string {
	if postParsedIdentitySet {
		return preferArgsIdentityOverSelectValue(postParsedIdentityValue, argsIdentity)
	}
	if preParsedIdentitySet {
		return preferArgsIdentityOverSelectValue(preParsedIdentityValue, argsIdentity)
	}
	if argsIdentity == "" {
		return ""
	}
	return cfg.NormalizeIdentityValue(argsIdentity)
}

// preferArgsIdentityOverSelectValue complements resolveIdentityValue for optional-value
// identity flags. If Cobra reports cfg.IdentityFlagSelectValue but the raw CLI args
// contain a non-empty, non-sentinel identity, use the normalized raw-args value;
// otherwise keep the flag value selected by resolveIdentityValue.
func preferArgsIdentityOverSelectValue(flagValue string, argsIdentity string) string {
	if flagValue == cfg.IdentityFlagSelectValue &&
		argsIdentity != "" &&
		argsIdentity != cfg.IdentityFlagSelectValue {
		return cfg.NormalizeIdentityValue(argsIdentity)
	}
	return flagValue
}
