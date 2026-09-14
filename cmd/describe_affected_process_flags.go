package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// processTemplatesFlagName and processFunctionsFlagName are the Cobra flag names on
// `atmos describe affected`. The StandardOptionsBuilder helpers below bind them to Viper under
// these same (bare) keys, so the resolver reads them by flag name.
const (
	processTemplatesFlagName = "process-templates"
	processFunctionsFlagName = "process-functions"
)

// newDescribeAffectedProcessFlagsParser creates a minimal StandardParser wired to only the
// --process-templates and --process-functions flags on `atmos describe affected`, using the
// shared StandardOptionsBuilder helpers so the flag names, defaults, descriptions, and
// ATMOS_PROCESS_TEMPLATES / ATMOS_PROCESS_FUNCTIONS env-var bindings match the rest of Atmos
// (the `list` and `terraform` families use the same helpers/bindings).
//
// The describe family predates the unified flag-parsing migration: every other flag on this
// command is still registered via raw Cobra PersistentFlags() and read back via cmd.Flags() (in
// exec.SetDescribeAffectedFlagValueInCliArgs). Introducing this narrow, two-flag parser (rather
// than migrating the whole command to the flag handler) is the minimal way to give these flags
// real env-var overrides without calling viper.BindEnv()/viper.BindPFlag() directly (Forbidigo
// bans that outside pkg/flags/); resolveDescribeAffectedProcessFlags below bridges the resolved
// value back to the legacy reader. Fully migrating `describe affected` to the flag handler is a
// larger, separate change.
//
// These bind the bare Viper keys "process-templates" / "process-functions" — the same keys the
// `list` family binds to the same ATMOS_PROCESS_* env vars on the shared global Viper. Binding
// the same key to the same env var is idempotent (unlike --error-mode, which needs a "describe"
// prefix because it maps to a different env var per family), so no prefix is needed here.
func newDescribeAffectedProcessFlagsParser() *flags.StandardParser {
	defer perf.Track(nil, "cmd.newDescribeAffectedProcessFlagsParser")()

	return flags.NewStandardOptionsBuilder().
		WithProcessTemplates(true).
		WithProcessFunctions(true).
		Build()
}

// resolveDescribeAffectedProcessFlags writes the ATMOS_PROCESS_TEMPLATES /
// ATMOS_PROCESS_FUNCTIONS environment variable values onto cmd's own --process-templates /
// --process-functions Cobra flags when the flags were not set explicitly on the command line.
//
// This lets the command's existing legacy flag-reading code
// (exec.SetDescribeAffectedFlagValueInCliArgs, which only reads a flag when
// cmd.Flags().Changed() is true) pick up the environment-sourced value with zero changes,
// since Set() marks the flag Changed just like an explicit CLI value would.
//
// Precedence is CLI > env var > default: a flag the user passed explicitly is left untouched
// (CLI wins over the env var), and the env-sourced value is applied only when it differs from
// the flag's current (default) value.
//
// The ATMOS_PROCESS_* env vars must already be bound to v — done once in init() via the parser's
// BindToViper, and by the tests via the same call. Reading v.GetString returns the raw env-var
// value when set, otherwise the SetDefault value (equal to the flag default). An invalid non-empty
// value (e.g. `ATMOS_PROCESS_FUNCTIONS=yes`) is rejected with an error rather than silently coerced
// to false (which viper's GetBool would do), so a typo can't quietly flip processing off. An empty
// value is treated as unset by viper (AllowEmptyEnv is off by default), so it falls back to the
// default instead of the invalid path.
//
// It deliberately does NOT use v.IsSet to detect whether the env var was provided: BindToViper
// calls v.SetDefault, and viper's IsSet reports true for keys that only have a default, so it can't
// distinguish "env var set" from "default in effect". Comparing the resolved value against the
// flag's current value avoids that ambiguity — when the env var is unset, v.GetString falls back to
// the SetDefault value, so the values match and the flag is correctly left untouched.
func resolveDescribeAffectedProcessFlags(cmd *cobra.Command, v *viper.Viper) error {
	defer perf.Track(nil, "cmd.resolveDescribeAffectedProcessFlags")()

	for _, f := range []struct {
		flagName string
		envVar   string
	}{
		{processTemplatesFlagName, "ATMOS_PROCESS_TEMPLATES"},
		{processFunctionsFlagName, "ATMOS_PROCESS_FUNCTIONS"},
	} {
		flag := cmd.Flags().Lookup(f.flagName)
		// Skip when the flag isn't registered (defensive: the real describeAffectedCmd always
		// registers these, but a command constructed without them has nothing to resolve) or when
		// the user set it explicitly on the CLI (CLI wins; the legacy reader already honors it).
		if flag == nil || flag.Changed {
			continue
		}

		// An empty value means unset — viper returns "" when neither the env var (empty env is
		// treated as unset by default) nor a SetDefault is present (e.g. after viper.Reset). Keep
		// the flag's current value rather than erroring; only a non-empty, non-boolean value is an
		// error. The builder binds the env var to the bare flag-name Viper key.
		raw := v.GetString(f.flagName)
		if raw == "" {
			continue
		}
		resolved, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("%w: %s must be a boolean (e.g. `true` or `false`)", errUtils.ErrInvalidFlagValue, f.envVar)
		}
		if strconv.FormatBool(resolved) == flag.Value.String() {
			continue
		}
		// Set can't fail here: `resolved` is a valid bool and the flag was confirmed registered
		// above, so strconv.FormatBool yields a value the bool flag always accepts. Ignoring the
		// error mirrors the `_ = ...Set(...)` idiom in pkg/flags (e.g. pkg/flags/standard.go). Set
		// marks the flag Changed for the legacy reader.
		_ = cmd.Flags().Set(f.flagName, strconv.FormatBool(resolved))
	}

	return nil
}
