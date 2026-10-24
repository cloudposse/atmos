package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// describeErrorModeFlagName is the Cobra flag name shared by all three describe
// commands; describeErrorModeViperKey is the namespaced Viper key it's bound to
// (see the "describe" Viper prefix comment on newDescribeErrorModeParser below).
const (
	describeErrorModeFlagName = "error-mode"
	describeErrorModeViperKey = "describe." + describeErrorModeFlagName
)

// newDescribeErrorModeParser creates a minimal StandardParser wired to only the
// --error-mode flag shared by `describe component`, `describe stacks`,
// `describe affected`, and `describe dependents`.
//
// The describe family predates the unified flag-parsing migration: every other flag
// on these three commands is still registered via raw Cobra PersistentFlags() and
// read back via cmd.Flags(). Introducing this narrow, single-flag parser (rather than
// migrating the whole command to flags.NewStandardParser) is the minimal way to give
// --error-mode a real ATMOS_DESCRIBE_ERROR_MODE environment variable override without
// calling viper.BindEnv() or viper.BindPFlag() directly, which Forbidigo bans outside
// pkg/flags/. It mirrors how cmd/list/flag_wrappers.go's WithErrorModeFlag isolates
// --error-mode as one composable flags.Option among many otherwise-raw list flags.
//
// A "describe" Viper key prefix is used (yielding the Viper key "describe.error-mode")
// so this parser's environment variable binding does not collide with the "list"
// family's bare "error-mode" Viper key (bound to ATMOS_LIST_ERROR_MODE by
// cmd/list/flag_wrappers.go's WithErrorModeFlag). Both parsers register against the
// same global viper.GetViper() instance at init() time, and Viper's BindEnv only
// keeps the most recently bound env var names for a given key, so sharing the bare
// "error-mode" key across both families would make one of the two env vars silently
// stop working depending on package init order.
//
// The Go-level default is intentionally "" (not "warn"): callers resolve the final
// value with exec.ResolveErrorMode(value, atmosConfig.Describe.ErrorMode) after
// loading atmosConfig, exactly as before this change.
func newDescribeErrorModeParser() *flags.StandardParser {
	defer perf.Track(nil, "cmd.newDescribeErrorModeParser")()

	return flags.NewStandardParser(
		flags.WithStringFlag(describeErrorModeFlagName, "", "",
			"How to handle recoverable errors (e.g. a Terraform backend not yet provisioned): `warn` (degrade + summary, default), "+
				"`silent` (degrade, no summary), or `strict` (fail immediately). Defaults to atmos.yaml's `describe.error_mode`, or `warn`. "+
				"Can also be set via the `ATMOS_DESCRIBE_ERROR_MODE` environment variable"),
		flags.WithEnvVars(describeErrorModeFlagName, "ATMOS_DESCRIBE_ERROR_MODE"),
		flags.WithValidValues(describeErrorModeFlagName, "strict", "warn", "silent"),
		flags.WithViperPrefix("describe"),
	)
}

// resolveDescribeErrorModeFlag binds the describe --error-mode flag to Viper (so CLI >
// ATMOS_DESCRIBE_ERROR_MODE > default precedence applies) and, if an effective value
// was resolved, writes it back onto cmd's own --error-mode Cobra flag.
//
// This lets each describe command's existing legacy flag-reading code
// (cmd.Flags().Changed + cmd.Flags().GetString, some of which lives in
// internal/exec) pick up the environment-variable-sourced value with zero changes,
// since pflag's Set() marks the flag Changed just like an explicit CLI value would.
func resolveDescribeErrorModeFlag(cmd *cobra.Command, v *viper.Viper, parser *flags.StandardParser) error {
	defer perf.Track(nil, "cmd.resolveDescribeErrorModeFlag")()

	if err := parser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	if val := v.GetString(describeErrorModeViperKey); val != "" {
		if err := cmd.Flags().Set(describeErrorModeFlagName, val); err != nil {
			return err
		}
	}

	return nil
}
