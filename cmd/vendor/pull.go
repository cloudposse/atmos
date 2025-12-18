package vendor

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	pkgvendor "github.com/cloudposse/atmos/pkg/vendor"
)

// PullParser handles flag parsing with Viper precedence.
var pullParser *flags.StandardParser

// PullOptions contains parsed flags for the vendor pull command.
type PullOptions struct {
	global.Flags
	Component     string
	Stack         string
	Tags          []string
	DryRun        bool
	Everything    bool
	ComponentType string
}

// pullCmd represents the vendor pull command.
var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull vendor dependencies",
	Long: `The vendor pull command downloads remote artifacts (such as Terraform modules, Helm charts, or other configurations) and stores them locally in your project.

By default, it vendors all components defined in vendor.yaml. Use flags to vendor specific components, stacks, or tags.`,
	Example: `  # Vendor all components
  atmos vendor pull

  # Vendor a specific component
  atmos vendor pull -c vpc

  # Vendor all components in a specific stack
  atmos vendor pull --stack dev-us-west-2

  # Vendor components with specific tags
  atmos vendor pull --tags networking,database

  # Dry run (show what would be vendored)
  atmos vendor pull --dry-run`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "vendor.pull.RunE")()

		// Debug log for command line args (matches legacy ProcessCommandLineArgs output).
		log.Debug("ProcessCommandLineArgs input", "componentType", "terraform", "args", args)

		// Parse flags using new options pattern.
		// Reuse global Viper to preserve env/config precedence
		v := viper.GetViper()
		if err := pullParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		// Debug log for identity flag state (matches legacy ProcessCommandLineArgs output).
		if identityFlag := cmd.Flag("identity"); identityFlag != nil {
			log.Debug("After ParseFlags", "identity.Value", identityFlag.Value.String(), "identity.Changed", identityFlag.Changed)
		}

		opts, err := parsePullOptions(cmd, v, args)
		if err != nil {
			return err
		}

		// Validate flags
		if err := pkgvendor.ValidateFlags(opts.Component, opts.Stack, opts.Tags, opts.Everything); err != nil {
			return err
		}

		// Set default for --everything if no other flags specified
		if !opts.Everything && !cmd.Flags().Changed("everything") &&
			opts.Component == "" && opts.Stack == "" && len(opts.Tags) == 0 {
			opts.Everything = true
		}

		// Initialize atmos config if not already done
		if atmosConfigPtr == nil {
			processStacks := opts.Stack != ""
			atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, processStacks)
			if err != nil {
				return fmt.Errorf("failed to initialize CLI config: %w", err)
			}
			atmosConfigPtr = &atmosConfig
		}

		// Call pkg/vendor with functional options
		return pkgvendor.Pull(atmosConfigPtr,
			pkgvendor.WithComponent(opts.Component),
			pkgvendor.WithStack(opts.Stack),
			pkgvendor.WithTags(opts.Tags),
			pkgvendor.WithDryRun(opts.DryRun),
			pkgvendor.WithComponentType(opts.ComponentType),
		)
	},
}

// parsePullOptions parses command flags into PullOptions.
//
//nolint:unparam // args parameter kept for consistency with other parse functions
func parsePullOptions(cmd *cobra.Command, v *viper.Viper, args []string) (*PullOptions, error) {
	var tags []string
	tagsCSV := v.GetString("tags")
	if tagsCSV != "" {
		tags = strings.Split(tagsCSV, ",")
	}

	return &PullOptions{
		Flags:         flags.ParseGlobalFlags(cmd, v),
		Component:     v.GetString("component"),
		Stack:         v.GetString("stack"),
		Tags:          tags,
		DryRun:        v.GetBool("dry-run"),
		Everything:    v.GetBool("everything"),
		ComponentType: v.GetString("type"),
	}, nil
}

func init() {
	// Create parser with pull-specific flags using functional options.
	pullParser = flags.NewStandardParser(
		flags.WithStringFlag("component", "c", "", "Only vendor the specified component"),
		flags.WithStringFlag("stack", "s", "", "Only vendor components for the specified stack"),
		flags.WithStringFlag("type", "t", "terraform", "Component type (terraform, helmfile, packer)"),
		flags.WithStringFlag("tags", "", "", "Only vendor components with matching tags (comma-separated)"),
		flags.WithBoolFlag("dry-run", "", false, "Simulate pulling without making changes"),
		flags.WithBoolFlag("everything", "", false, "Vendor all components from vendor.yaml"),
		flags.WithEnvVars("component", "ATMOS_VENDOR_COMPONENT"),
		flags.WithEnvVars("stack", "ATMOS_VENDOR_STACK"),
		flags.WithEnvVars("type", "ATMOS_VENDOR_TYPE"),
		flags.WithEnvVars("tags", "ATMOS_VENDOR_TAGS"),
		flags.WithEnvVars("dry-run", "ATMOS_VENDOR_DRY_RUN"),
	)

	// Register flags using the standard RegisterFlags method.
	pullParser.RegisterFlags(pullCmd)

	// Bind flags to Viper for environment variable support.
	if err := pullParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
