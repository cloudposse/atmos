package cmd

import (
	"strconv"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// processTemplatesFlagName and processFunctionsFlagName are the Cobra flag names on
// `atmos describe affected`; the *ViperKey constants are the namespaced Viper keys they
// bind to (see the "describe" Viper prefix comment on newDescribeAffectedProcessFlagsParser).
const (
	processTemplatesFlagName = "process-templates"
	processFunctionsFlagName = "process-functions"
	processTemplatesViperKey = "describe." + processTemplatesFlagName
	processFunctionsViperKey = "describe." + processFunctionsFlagName
)

// newDescribeAffectedProcessFlagsParser creates a minimal StandardParser wired to only the
// --process-templates and --process-functions flags on `atmos describe affected`.
//
// It mirrors newDescribeErrorModeParser: the describe family predates the unified
// flag-parsing migration, so every other flag on this command is registered via raw Cobra
// PersistentFlags() and read back via cmd.Flags() (in exec.SetDescribeAffectedFlagValueInCliArgs).
// Introducing this narrow, two-flag parser (rather than migrating the whole command to
// flags.NewStandardParser) is the minimal way to give these two flags real
// ATMOS_PROCESS_TEMPLATES / ATMOS_PROCESS_FUNCTIONS environment variable overrides without
// calling viper.BindEnv() or viper.BindPFlag() directly, which Forbidigo bans outside
// pkg/flags/. The `list` and `terraform` command families already bind these same env vars
// this way (see cmd/list/flag_wrappers.go); this closes the gap for `describe affected`.
//
// A "describe" Viper key prefix is used (yielding "describe.process-templates" /
// "describe.process-functions") so this parser's bindings do not collide with the "list"
// family's bare "process-templates" / "process-functions" Viper keys, which are bound to the
// same env vars against the same global viper.GetViper() instance at init() time. The keys
// differ, so both families' bindings coexist; the shared "describe" prefix with the
// error-mode parser is likewise safe because the flag names differ.
func newDescribeAffectedProcessFlagsParser() *flags.StandardParser {
	defer perf.Track(nil, "cmd.newDescribeAffectedProcessFlagsParser")()

	return flags.NewStandardParser(
		flags.WithBoolFlag(processTemplatesFlagName, "", true,
			"Enable/disable Go template processing in Atmos stack manifests when executing the command"),
		flags.WithBoolFlag(processFunctionsFlagName, "", true,
			"Enable/disable YAML functions processing in Atmos stack manifests when executing the command"),
		flags.WithEnvVars(processTemplatesFlagName, "ATMOS_PROCESS_TEMPLATES"),
		flags.WithEnvVars(processFunctionsFlagName, "ATMOS_PROCESS_FUNCTIONS"),
		flags.WithViperPrefix("describe"),
	)
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
// BindToViper, and by the tests via the same call. Reading v.GetBool returns the env-var value
// when set, otherwise the SetDefault value (equal to the flag default). It deliberately does NOT
// use v.IsSet to detect whether the env var was provided: BindToViper calls v.SetDefault, and
// viper's IsSet reports true for keys that only have a default, so it can't distinguish "env var
// set" from "default in effect". Comparing the resolved value against the flag's current value
// avoids that ambiguity — when the env var is unset, v.GetBool falls back to the SetDefault value,
// so the values match and the flag is correctly left untouched.
func resolveDescribeAffectedProcessFlags(cmd *cobra.Command, v *viper.Viper) {
	defer perf.Track(nil, "cmd.resolveDescribeAffectedProcessFlags")()

	for _, f := range []struct {
		flagName string
		viperKey string
	}{
		{processTemplatesFlagName, processTemplatesViperKey},
		{processFunctionsFlagName, processFunctionsViperKey},
	} {
		flag := cmd.Flags().Lookup(f.flagName)
		// Skip when the flag isn't registered (defensive: the real describeAffectedCmd always
		// registers these, but a command constructed without them has nothing to resolve) or when
		// the user set it explicitly on the CLI (CLI wins; the legacy reader already honors it).
		if flag == nil || flag.Changed {
			continue
		}

		resolved := strconv.FormatBool(v.GetBool(f.viperKey))
		if resolved == flag.Value.String() {
			continue
		}
		// Set can't fail here: `resolved` is always the literal "true"/"false" and the flag was
		// confirmed registered above. Ignoring the error mirrors the `_ = ...Set(...)` idiom in
		// pkg/flags (e.g. pkg/flags/standard.go). Set marks the flag Changed for the legacy reader.
		_ = cmd.Flags().Set(f.flagName, resolved)
	}
}
