package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// describeAffectedTagsFlagName, describeAffectedLabelsFlagName, and describeAffectedFlattenFlagName are
// the Cobra flag names on `atmos describe affected`; the *ViperKey constants are the namespaced Viper
// keys they are bound to, and the *EnvVar constants are the environment variables that feed them.
const (
	describeAffectedTagsFlagName    = "tags"
	describeAffectedLabelsFlagName  = "labels"
	describeAffectedFlattenFlagName = "flatten"
	describeAffectedViperPrefix     = "describe.affected"
	describeAffectedTagsViperKey    = describeAffectedViperPrefix + "." + describeAffectedTagsFlagName
	describeAffectedLabelsViperKey  = describeAffectedViperPrefix + "." + describeAffectedLabelsFlagName
	describeAffectedFlattenViperKey = describeAffectedViperPrefix + "." + describeAffectedFlattenFlagName
	describeAffectedTagsEnvVar      = "ATMOS_TAGS"
	describeAffectedLabelsEnvVar    = "ATMOS_LABELS"
	describeAffectedFlattenEnvVar   = "ATMOS_DESCRIBE_AFFECTED_FLATTEN"
)

// newDescribeAffectedSelectorFlagsParser creates a minimal StandardParser wired to the
// --tags and --labels component selectors and the --flatten result-shaping flag on
// `atmos describe affected`.
//
// Like the sibling --error-mode and --process-* parsers, it exists so these two flags get real
// ATMOS_TAGS / ATMOS_LABELS / ATMOS_DESCRIBE_AFFECTED_FLATTEN environment variable bindings without
// calling viper.BindEnv() or viper.BindPFlag() directly (Forbidigo bans that outside pkg/flags/), while the rest of the
// command keeps its legacy raw-Cobra flag reading in exec.SetDescribeAffectedFlagValueInCliArgs.
// The "describe.affected" Viper prefix keeps the bindings from colliding with the bare "tags" and
// "labels" keys that the terraform and list families bind to other environment variables.
func newDescribeAffectedSelectorFlagsParser() *flags.StandardParser {
	defer perf.Track(nil, "cmd.newDescribeAffectedSelectorFlagsParser")()

	return flags.NewStandardParser(
		flags.WithStringSliceFlag(describeAffectedTagsFlagName, "", nil,
			"Keep only components whose `metadata.tags` contain any of these tags (comma-separated, matches any): --tags=production,tier-1"),
		flags.WithEnvVars(describeAffectedTagsFlagName, describeAffectedTagsEnvVar),
		flags.WithStringFlag(describeAffectedLabelsFlagName, "", "",
			"Keep only components whose `metadata.labels` contain all of these labels (comma-separated key=value or key:value pairs, matches all): --labels=ci=auto"),
		flags.WithEnvVars(describeAffectedLabelsFlagName, describeAffectedLabelsEnvVar),
		flags.WithBoolFlag(describeAffectedFlattenFlagName, "", false,
			"Lift every dependent (after --tags/--labels pruning) into the top-level list as `affected: dependent`; requires --include-dependents"),
		flags.WithEnvVars(describeAffectedFlattenFlagName, describeAffectedFlattenEnvVar),
		flags.WithViperPrefix(describeAffectedViperPrefix),
	)
}

// resolveDescribeAffectedSelectorFlags writes the ATMOS_TAGS / ATMOS_LABELS /
// ATMOS_DESCRIBE_AFFECTED_FLATTEN environment variable values onto cmd's own --tags / --labels /
// --flatten Cobra flags when the flags were not set explicitly on the command line, so the legacy
// flag reader (which only reads a flag when it is Changed) picks them up. Precedence is
// CLI > env var > default.
//
// Every value copied from the environment is marked with flags.MarkFlagValueFromEnv so the
// validation downstream can tell an ambient ATMOS_* value from an explicit flag.
func resolveDescribeAffectedSelectorFlags(cmd *cobra.Command, v *viper.Viper, parser *flags.StandardParser) error {
	defer perf.Track(nil, "cmd.resolveDescribeAffectedSelectorFlags")()

	if err := parser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Drop any environment mark left by an earlier run on the same long-lived flag object.
	for _, name := range []string{describeAffectedTagsFlagName, describeAffectedLabelsFlagName, describeAffectedFlattenFlagName} {
		flags.MarkFlagValueFromEnv(cmd.Flags().Lookup(name), "")
	}

	if err := resolveDescribeAffectedTagsFlag(cmd, v); err != nil {
		return err
	}
	if err := resolveDescribeAffectedLabelsFlag(cmd, v); err != nil {
		return err
	}
	return resolveDescribeAffectedFlattenFlag(cmd, v)
}

// resolveDescribeAffectedTagsFlag copies ATMOS_TAGS onto the --tags flag when the flag was not set
// on the command line.
func resolveDescribeAffectedTagsFlag(cmd *cobra.Command, v *viper.Viper) error {
	f := cmd.Flags().Lookup(describeAffectedTagsFlagName)
	if f == nil || f.Changed {
		return nil
	}

	// Viper splits a slice-typed env var on whitespace, so re-join before handing it back to
	// the comma-aware slice flag.
	envTags := v.GetStringSlice(describeAffectedTagsViperKey)
	if len(envTags) == 0 {
		return nil
	}
	if err := cmd.Flags().Set(describeAffectedTagsFlagName, strings.Join(envTags, ",")); err != nil {
		return err
	}
	flags.MarkFlagValueFromEnv(f, describeAffectedTagsEnvVar)
	return nil
}

// resolveDescribeAffectedLabelsFlag copies ATMOS_LABELS onto the --labels flag when the flag was not
// set on the command line.
func resolveDescribeAffectedLabelsFlag(cmd *cobra.Command, v *viper.Viper) error {
	f := cmd.Flags().Lookup(describeAffectedLabelsFlagName)
	if f == nil || f.Changed {
		return nil
	}

	envLabels := v.GetString(describeAffectedLabelsViperKey)
	if envLabels == "" {
		return nil
	}
	if err := cmd.Flags().Set(describeAffectedLabelsFlagName, envLabels); err != nil {
		return err
	}
	flags.MarkFlagValueFromEnv(f, describeAffectedLabelsEnvVar)
	return nil
}

// resolveDescribeAffectedFlattenFlag copies ATMOS_DESCRIBE_AFFECTED_FLATTEN onto the --flatten flag
// when the flag was not set on the command line. A non-boolean value is rejected rather than
// silently read as false, so a typo cannot quietly turn flattening off.
func resolveDescribeAffectedFlattenFlag(cmd *cobra.Command, v *viper.Viper) error {
	f := cmd.Flags().Lookup(describeAffectedFlattenFlagName)
	if f == nil || f.Changed {
		return nil
	}

	raw := v.GetString(describeAffectedFlattenViperKey)
	if raw == "" {
		return nil
	}
	enabled, err := strconv.ParseBool(raw)
	if err != nil {
		return fmt.Errorf("%w: %s must be a boolean (e.g. `true` or `false`)", errUtils.ErrInvalidFlagValue, describeAffectedFlattenEnvVar)
	}
	// The flag defaults to false, so a false value changes nothing.
	if !enabled {
		return nil
	}
	if err := cmd.Flags().Set(describeAffectedFlattenFlagName, "true"); err != nil {
		return err
	}
	flags.MarkFlagValueFromEnv(f, describeAffectedFlattenEnvVar)
	return nil
}
