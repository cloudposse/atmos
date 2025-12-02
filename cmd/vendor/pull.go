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

// PullOptions contains all options for vendor pull command.
type PullOptions struct {
	global.Flags                             // Embedded global flags (chdir, logs-level, etc.).
	AtmosConfig   *schema.AtmosConfiguration // Populated after config init.
	Component     string                     // --component, -c.
	Stack         string                     // --stack, -s.
	ComponentType string                     // --type, -t (default: "terraform").
	Tags          string                     // --tags (comma-separated).
	DryRun        bool                       // --dry-run.
	Everything    bool                       // --everything.
}

// SetAtmosConfig implements AtmosConfigSetter for PullOptions.
func (o *PullOptions) SetAtmosConfig(cfg *schema.AtmosConfiguration) {
	o.AtmosConfig = cfg
}

// Validate checks that pull options are consistent.
func (o *PullOptions) Validate() error {
	defer perf.Track(nil, "vendor.PullOptions.Validate")()

	if o.Component != "" && o.Stack != "" {
		return fmt.Errorf("%w: --component and --stack cannot be used together", errUtils.ErrMutuallyExclusiveFlags)
	}
	if o.Component != "" && o.Tags != "" {
		return fmt.Errorf("%w: --component and --tags cannot be used together", errUtils.ErrMutuallyExclusiveFlags)
	}
	if o.Everything && (o.Component != "" || o.Stack != "" || o.Tags != "") {
		return fmt.Errorf("%w: --everything cannot be combined with --component, --stack, or --tags", errUtils.ErrMutuallyExclusiveFlags)
	}
	return nil
}

var pullParser *flags.StandardParser

var pullCmd = &cobra.Command{
	Use:                "pull",
	Short:              "Pull the latest vendor configurations or dependencies",
	Long:               "Pull and update vendor-specific configurations or dependencies to ensure the project has the latest required resources.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	RunE:               runPull,
}

func init() {
	// Create parser with all pull-specific flags.
	pullParser = flags.NewStandardParser(
		// String flags.
		flags.WithStringFlag("component", "c", "", "Only vendor the specified component"),
		flags.WithStringFlag("stack", "s", "", "Only vendor the specified stack"),
		flags.WithStringFlag("type", "t", "terraform", "Component type (terraform or helmfile)"),
		flags.WithStringFlag("tags", "", "", "Only vendor components with specified tags (comma-separated)"),

		// Bool flags.
		flags.WithBoolFlag("dry-run", "", false, "Simulate without making changes"),
		flags.WithBoolFlag("everything", "", false, "Vendor all components"),

		// Environment variable bindings.
		flags.WithEnvVars("component", "ATMOS_VENDOR_COMPONENT"),
		flags.WithEnvVars("stack", "ATMOS_VENDOR_STACK", "ATMOS_STACK"),
		flags.WithEnvVars("type", "ATMOS_VENDOR_TYPE"),
		flags.WithEnvVars("tags", "ATMOS_VENDOR_TAGS"),
		flags.WithEnvVars("dry-run", "ATMOS_VENDOR_DRY_RUN"),
	)

	// Register flags with cobra command.
	pullParser.RegisterFlags(pullCmd)

	// Shell completions.
	_ = pullCmd.RegisterFlagCompletionFunc("component", componentsArgCompletion)
	addStackCompletion(pullCmd)
}

func runPull(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "vendor.runPull")()

	// Parse options with Viper precedence (CLI > ENV > config).
	opts, err := parsePullOptions(cmd)
	if err != nil {
		return err
	}

	// Validate options.
	if err := opts.Validate(); err != nil {
		return err
	}

	// Initialize Atmos config.
	skipStackValidation := opts.Stack == ""
	if err := initAtmosConfig(opts, skipStackValidation); err != nil {
		return err
	}

	// Execute via pkg/vendoring.
	return vendor.Pull(opts.AtmosConfig, &vendor.PullParams{
		Component:     opts.Component,
		Stack:         opts.Stack,
		ComponentType: opts.ComponentType,
		DryRun:        opts.DryRun,
		Tags:          opts.Tags,
		Everything:    opts.Everything,
	})
}

func parsePullOptions(cmd *cobra.Command) (*PullOptions, error) {
	defer perf.Track(nil, "vendor.parsePullOptions")()

	v := viper.GetViper()
	if err := pullParser.BindFlagsToViper(cmd, v); err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrParseFlag, err)
	}

	return &PullOptions{
		// global.Flags embedded - would be populated from root persistent flags.
		Component:     v.GetString(flagComponent),
		Stack:         v.GetString("stack"),
		ComponentType: v.GetString(flagType),
		Tags:          v.GetString("tags"),
		DryRun:        v.GetBool("dry-run"),
		Everything:    v.GetBool("everything"),
	}, nil
}
