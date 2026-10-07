package cmd

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
)

// describeAffectedTagsFlagName and describeAffectedLabelsFlagName are the Cobra flag names on
// `atmos describe affected`; the *ViperKey constants are the namespaced Viper keys they are bound to.
const (
	describeAffectedTagsFlagName   = "tags"
	describeAffectedLabelsFlagName = "labels"
	describeAffectedTagsViperKey   = "describe.affected." + describeAffectedTagsFlagName
	describeAffectedLabelsViperKey = "describe.affected." + describeAffectedLabelsFlagName
)

// newDescribeAffectedSelectorFlagsParser creates a minimal StandardParser wired to only the
// --tags and --labels component selectors on `atmos describe affected`.
//
// Like the sibling --error-mode and --process-* parsers, it exists so these two flags get real
// ATMOS_TAGS / ATMOS_LABELS environment variable bindings without calling viper.BindEnv() or
// viper.BindPFlag() directly (Forbidigo bans that outside pkg/flags/), while the rest of the
// command keeps its legacy raw-Cobra flag reading in exec.SetDescribeAffectedFlagValueInCliArgs.
// The "describe.affected" Viper prefix keeps the bindings from colliding with the bare "tags" and
// "labels" keys that the terraform and list families bind to other environment variables.
func newDescribeAffectedSelectorFlagsParser() *flags.StandardParser {
	defer perf.Track(nil, "cmd.newDescribeAffectedSelectorFlagsParser")()

	return flags.NewStandardParser(
		flags.WithStringSliceFlag(describeAffectedTagsFlagName, "", nil,
			"Keep only components whose `metadata.tags` contain any of these tags (comma-separated, matches any): --tags=production,tier-1"),
		flags.WithEnvVars(describeAffectedTagsFlagName, "ATMOS_TAGS"),
		flags.WithStringFlag(describeAffectedLabelsFlagName, "", "",
			"Keep only components whose `metadata.labels` contain all of these labels (comma-separated key=value or key:value pairs, matches all): --labels=ci=auto"),
		flags.WithEnvVars(describeAffectedLabelsFlagName, "ATMOS_LABELS"),
		flags.WithViperPrefix("describe.affected"),
	)
}

// resolveDescribeAffectedSelectorFlags writes the ATMOS_TAGS / ATMOS_LABELS environment variable
// values onto cmd's own --tags / --labels Cobra flags when the flags were not set explicitly on
// the command line, so the legacy flag reader (which only reads a flag when it is Changed) picks
// them up. Precedence is CLI > env var > default.
func resolveDescribeAffectedSelectorFlags(cmd *cobra.Command, v *viper.Viper, parser *flags.StandardParser) error {
	defer perf.Track(nil, "cmd.resolveDescribeAffectedSelectorFlags")()

	if err := parser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	if f := cmd.Flags().Lookup(describeAffectedTagsFlagName); f != nil && !f.Changed {
		// Viper splits a slice-typed env var on whitespace, so re-join before handing it back to
		// the comma-aware slice flag.
		if envTags := v.GetStringSlice(describeAffectedTagsViperKey); len(envTags) > 0 {
			if err := cmd.Flags().Set(describeAffectedTagsFlagName, strings.Join(envTags, ",")); err != nil {
				return err
			}
		}
	}

	if f := cmd.Flags().Lookup(describeAffectedLabelsFlagName); f != nil && !f.Changed {
		if envLabels := v.GetString(describeAffectedLabelsViperKey); envLabels != "" {
			if err := cmd.Flags().Set(describeAffectedLabelsFlagName, envLabels); err != nil {
				return err
			}
		}
	}

	return nil
}
