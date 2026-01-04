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

// UpdateOptions contains all options for vendor update command.
type UpdateOptions struct {
	global.Flags                             // Embedded global flags (chdir, logs-level, etc.).
	AtmosConfig   *schema.AtmosConfiguration // Populated after config init.
	Component     string                     // --component, -c.
	ComponentType string                     // --type, -t (default: "terraform").
	Tags          string                     // --tags (comma-separated).
	Check         bool                       // --check (check only, don't update).
	Pull          bool                       // --pull (pull after updating).
	Outdated      bool                       // --outdated (show only outdated components).
}

// SetAtmosConfig implements AtmosConfigSetter for UpdateOptions.
func (o *UpdateOptions) SetAtmosConfig(cfg *schema.AtmosConfiguration) {
	o.AtmosConfig = cfg
}

// Validate checks that update options are consistent.
func (o *UpdateOptions) Validate() error {
	defer perf.Track(nil, "vendor.UpdateOptions.Validate")()

	if o.Check && o.Pull {
		return fmt.Errorf("%w: --check and --pull cannot be used together", errUtils.ErrMutuallyExclusiveFlags)
	}
	return nil
}

var updateParser *flags.StandardParser

var updateCmd = &cobra.Command{
	Use:                "update",
	Short:              "Check for and apply vendor component updates",
	Long:               "Check for available updates to vendored components and optionally update the vendor configuration to use newer versions.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: false},
	Args:               cobra.NoArgs,
	RunE:               runUpdate,
}

func init() {
	// Create parser with all update-specific flags.
	updateParser = flags.NewStandardParser(
		// String flags.
		flags.WithStringFlag("component", "c", "", "Only check/update the specified component"),
		flags.WithStringFlag("type", "t", "terraform", "Component type (terraform or helmfile)"),
		flags.WithStringFlag("tags", "", "", "Only check/update components with specified tags (comma-separated)"),

		// Bool flags.
		flags.WithBoolFlag("check", "", false, "Check for updates without modifying files"),
		flags.WithBoolFlag("pull", "", false, "Pull components after updating vendor config"),
		flags.WithBoolFlag("outdated", "", false, "Show only outdated components"),

		// Environment variable bindings.
		flags.WithEnvVars("component", "ATMOS_VENDOR_COMPONENT"),
		flags.WithEnvVars("type", "ATMOS_VENDOR_TYPE"),
		flags.WithEnvVars("tags", "ATMOS_VENDOR_TAGS"),
		flags.WithEnvVars("check", "ATMOS_VENDOR_CHECK"),
		flags.WithEnvVars("pull", "ATMOS_VENDOR_PULL"),
		flags.WithEnvVars("outdated", "ATMOS_VENDOR_OUTDATED"),
	)

	// Register flags with cobra command.
	updateParser.RegisterFlags(updateCmd)

	// Shell completions.
	_ = updateCmd.RegisterFlagCompletionFunc("component", componentsArgCompletion)
}

func runUpdate(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "vendor.runUpdate")()

	// Parse options with Viper precedence (CLI > ENV > config).
	opts, err := parseUpdateOptions(cmd)
	if err != nil {
		return err
	}

	// Validate options.
	if err := opts.Validate(); err != nil {
		return err
	}

	// Initialize Atmos config.
	// Vendor update doesn't use stack flag, so skip stack validation.
	if err := initAtmosConfig(opts, true); err != nil {
		return err
	}

	// Execute via pkg/vendoring.
	return vendor.Update(opts.AtmosConfig, &vendor.UpdateParams{
		Component:     opts.Component,
		ComponentType: opts.ComponentType,
		Tags:          opts.Tags,
		Check:         opts.Check,
		Pull:          opts.Pull,
		Outdated:      opts.Outdated,
	})
}

func parseUpdateOptions(cmd *cobra.Command) (*UpdateOptions, error) {
	defer perf.Track(nil, "vendor.parseUpdateOptions")()

	v := viper.GetViper()
	if err := updateParser.BindFlagsToViper(cmd, v); err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrParseFlag, err)
	}

	return &UpdateOptions{
		// global.Flags embedded - would be populated from root persistent flags.
		Component:     v.GetString(flagComponent),
		ComponentType: v.GetString(flagType),
		Tags:          v.GetString("tags"),
		Check:         v.GetBool("check"),
		Pull:          v.GetBool("pull"),
		Outdated:      v.GetBool("outdated"),
	}, nil
}
