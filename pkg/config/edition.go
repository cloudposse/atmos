package config

import (
	"os"
	"strings"

	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/edition"
	"github.com/cloudposse/atmos/pkg/flags/osargs"
	log "github.com/cloudposse/atmos/pkg/logger"
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
// The flag is read via editionPinFromFlag, with the env var and config key read
// from the local Viper instance, where setEnv has bound ATMOS_EDITION and the
// config files have merged.
func resolveEditionPin(v *viper.Viper) string {
	if flagValue := editionPinFromFlag(); flagValue != "" {
		return flagValue
	}
	return strings.TrimSpace(v.GetString(editionKey))
}

// editionPinFromFlag returns the --edition flag's value, or "" when unset. It
// checks Viper's global singleton first (synced by syncGlobalFlagsToViper in
// cmd/root.go), falling back to a raw os.Args scan for commands that run with
// DisableFlagParsing=true (terraform, helmfile, packer, auth exec), where
// Cobra never populates the global flag binding.
func editionPinFromFlag() string {
	if flagValue := strings.TrimSpace(viper.GetViper().GetString(editionKey)); flagValue != "" {
		return flagValue
	}
	return parseEditionFromOsArgs(os.Args)
}

// EditionPinSource reports where a resolved edition pin came from: "flag",
// "env", "config", or "" when pin is empty. Callers already have the resolved
// pin (e.g. atmosConfig.Edition, produced by resolveEditionPin during config
// load) — this only determines which precedence tier produced it, reusing
// resolveEditionPin's own flag-detection (editionPinFromFlag) so the two can
// never drift out of sync. This is what `atmos describe edition` calls instead
// of re-deriving the precedence itself (see cmd/describe_edition.go).
func EditionPinSource(pin string) string {
	if pin == "" {
		return ""
	}
	if editionPinFromFlag() != "" {
		return "flag"
	}
	if envValue, set := os.LookupEnv("ATMOS_EDITION"); set && strings.TrimSpace(envValue) != "" {
		return "env"
	}
	return "config"
}

// parseEditionFromOsArgs extracts --edition from raw args for commands whose
// flags Cobra never parses (DisableFlagParsing=true). Same pattern as
// ParseProfilesFromOsArgs.
func parseEditionFromOsArgs(args []string) string {
	return strings.TrimSpace(osargs.ParseString(args, editionKey))
}
