package vendor

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	vendor "github.com/cloudposse/atmos/pkg/vendoring"
)

// DiffOptions contains all options for vendor diff command.
type DiffOptions struct {
	global.Flags                             // Embedded global flags (chdir, logs-level, etc.).
	AtmosConfig   *schema.AtmosConfiguration // Populated after config init.
	Component     string                     // --component, -c (required).
	ComponentType string                     // --type, -t (default: "terraform").
	From          string                     // --from (source version/ref).
	To            string                     // --to (target version/ref).
	File          string                     // --file (specific file to diff).
	Context       int                        // --context (lines of context, default: 3).
	Unified       bool                       // --unified (unified diff format, default: true).
}

// SetAtmosConfig implements AtmosConfigSetter for DiffOptions.
func (o *DiffOptions) SetAtmosConfig(cfg *schema.AtmosConfiguration) {
	o.AtmosConfig = cfg
}

// Validate checks that diff options are consistent.
func (o *DiffOptions) Validate() error {
	defer perf.Track(nil, "vendor.DiffOptions.Validate")()

	if o.Component == "" {
		return errUtils.ErrComponentFlagRequired
	}
	return nil
}

var diffParser *flags.StandardParser

var diffCmd = &cobra.Command{
	Use:                "diff",
	Short:              "Show differences between vendor component versions",
	Long:               "Compare vendor component versions and display the differences between the current version and a target version or between two specified versions.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	RunE:               runDiff,
}

func init() {
	// Create parser with all diff-specific flags.
	diffParser = flags.NewStandardParser(
		// String flags.
		flags.WithStringFlag("component", "c", "", "Component to diff (required)"),
		flags.WithStringFlag("type", "t", "terraform", "Component type (terraform or helmfile)"),
		flags.WithStringFlag("from", "", "", "Source version/ref to compare from (default: current version)"),
		flags.WithStringFlag("to", "", "", "Target version/ref to compare to (default: latest)"),
		flags.WithStringFlag("file", "", "", "Specific file to diff within the component"),

		// Int flags.
		flags.WithIntFlag("context", "", 3, "Number of context lines to show in diff"),

		// Bool flags.
		flags.WithBoolFlag("unified", "", true, "Use unified diff format"),

		// Environment variable bindings.
		flags.WithEnvVars("component", "ATMOS_VENDOR_COMPONENT"),
		flags.WithEnvVars("type", "ATMOS_VENDOR_TYPE"),
		flags.WithEnvVars("from", "ATMOS_VENDOR_FROM"),
		flags.WithEnvVars("to", "ATMOS_VENDOR_TO"),
		flags.WithEnvVars("context", "ATMOS_VENDOR_CONTEXT"),
	)

	// Register flags with cobra command.
	diffParser.RegisterFlags(diffCmd)

	// Shell completions.
	_ = diffCmd.RegisterFlagCompletionFunc("component", componentsArgCompletion)
}

func runDiff(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "vendor.runDiff")()

	// Parse options with Viper precedence (CLI > ENV > config).
	opts, err := parseDiffOptions(cmd)
	if err != nil {
		return err
	}

	// Validate options.
	if err := opts.Validate(); err != nil {
		return err
	}

	// Initialize Atmos config.
	// Vendor diff doesn't use stack flag, so skip stack validation.
	if err := initAtmosConfig(opts, true); err != nil {
		return err
	}

	// Execute via pkg/vendoring.
	return vendor.Diff(opts.AtmosConfig, &vendor.DiffParams{
		Component:     opts.Component,
		ComponentType: opts.ComponentType,
		From:          opts.From,
		To:            opts.To,
		File:          opts.File,
		Context:       opts.Context,
		Unified:       opts.Unified,
		NoColor:       false, // This would come from global flags if needed.
	})
}

func parseDiffOptions(cmd *cobra.Command) (*DiffOptions, error) {
	defer perf.Track(nil, "vendor.parseDiffOptions")()

	v := viper.GetViper()
	if err := diffParser.BindFlagsToViper(cmd, v); err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrParseFlag, err)
	}

	return &DiffOptions{
		// global.Flags embedded - would be populated from root persistent flags.
		Component:     v.GetString(flagComponent),
		ComponentType: v.GetString(flagType),
		From:          v.GetString("from"),
		To:            v.GetString("to"),
		File:          v.GetString("file"),
		Context:       v.GetInt("context"),
		Unified:       v.GetBool("unified"),
	}, nil
}
