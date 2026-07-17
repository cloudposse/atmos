package config

import (
	"io"
	"os"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/edition"
)

const editionKey = "edition"

// applyEditionDefaults applies the edition pin, if any, as a rollback overlay on
// the Viper defaults layer: for every default that changed after the pinned date,
// the pre-change value is re-installed via SetDefault. It must run after all
// config sources (files, atmos.d, profiles, env bindings) have merged and
// immediately before the final Unmarshal — SetDefault only touches the defaults
// layer, so a user-set value, env var, or flag always still wins.
func applyEditionDefaults(v *viper.Viper) error {
	raw := resolveEditionPin(v)
	if raw == "" {
		return nil
	}

	anchor, err := edition.ParseAnchor(raw)
	if err != nil {
		return err
	}

	overrides := edition.Overrides(anchor)
	for key, oldValue := range overrides {
		v.SetDefault(key, oldValue)
	}
	// Record the winning pin so atmosConfig.Edition reflects it even when it
	// came from the --edition flag (which only the global Viper sees).
	v.Set(editionKey, raw)
	log.Debug("Edition pin active",
		editionKey, raw,
		"resolved", anchor.Date.Format("2006-01-02"),
		"overrides", len(overrides))
	return nil
}

// resolveEditionPin returns the raw edition string, or "" when no pin is set.
//
// Precedence: --edition flag > ATMOS_EDITION env var > `edition:` in atmos.yaml.
// The flag is read from Viper's global singleton (synced by syncGlobalFlagsToViper
// in cmd/root.go), with an os.Args fallback for commands that run with
// DisableFlagParsing=true (terraform, helmfile, packer, auth exec). The env var
// and config key are read from the local Viper instance, where setEnv has bound
// ATMOS_EDITION and the config files have merged.
func resolveEditionPin(v *viper.Viper) string {
	if flagValue := strings.TrimSpace(viper.GetViper().GetString(editionKey)); flagValue != "" {
		return flagValue
	}
	if argValue := parseEditionFromOsArgs(os.Args); argValue != "" {
		return argValue
	}
	return strings.TrimSpace(v.GetString(editionKey))
}

// parseEditionFromOsArgs extracts --edition from raw args for commands whose
// flags Cobra never parses (DisableFlagParsing=true). Same pattern as
// ParseProfilesFromOsArgs.
func parseEditionFromOsArgs(args []string) string {
	fs := pflag.NewFlagSet("edition-parser", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.ParseErrorsAllowlist.UnknownFlags = true
	// Suppress pflag's implicit usage printout on --help (see ParseProfilesFromOsArgs).
	fs.Usage = func() {}

	value := fs.String(editionKey, "", "Edition pin")
	_ = fs.Parse(args) // Ignore errors from unknown flags.

	return strings.TrimSpace(*value)
}
