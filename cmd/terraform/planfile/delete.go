package planfile

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// deleteParser handles flag parsing with Viper precedence for the delete command.
var deleteParser *flags.StandardParser

// DeleteOptions contains parsed flags for the delete command.
type DeleteOptions struct {
	BaseOptions
	Component string
	Force     bool
	All       bool
}

var deleteCmd = &cobra.Command{
	Use:   "delete [component]",
	Short: "Delete Terraform plan files from storage",
	Long: `Delete Terraform plan files from the configured storage backend.

The component is an optional positional argument and the stack can be specified via -s/--stack.
By default, only planfiles for the current SHA are deleted. Use --all to delete all SHAs.
Use --force to skip the interactive confirmation prompt.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDelete,
}

func init() {
	// Create parser with delete-specific flags using functional options.
	deleteParser = flags.NewStandardParser(
		flags.WithStringFlag("store", "", "", "Storage backend to use (default from config)"),
		flags.WithBoolFlag("force", "f", false, "Skip confirmation prompt"),
		flags.WithBoolFlag("all", "", false, "Delete planfiles for all SHAs (bypass SHA filter)"),
		flags.WithEnvVars("store", "ATMOS_PLANFILE_STORE"),
		flags.WithEnvVars("force", "ATMOS_PLANFILE_DELETE_FORCE"),
	)

	// Register flags with the command.
	deleteParser.RegisterFlags(deleteCmd)

	// Bind to Viper for environment variable support.
	if err := deleteParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	// Add to parent command.
	PlanfileCmd.AddCommand(deleteCmd)
}

// parseDeleteOptions parses command flags into DeleteOptions.
func parseDeleteOptions(cmd *cobra.Command, v *viper.Viper, args []string) *DeleteOptions {
	component := ""
	if len(args) > 0 {
		component = args[0]
	}

	return &DeleteOptions{
		BaseOptions: parseBaseOptions(cmd, v),
		Component:   component,
		Force:       v.GetBool("force"),
		All:         v.GetBool("all"),
	}
}

func runDelete(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "planfile.runDelete")()

	// Bind flags to Viper for proper precedence.
	v := viper.GetViper()
	if err := deleteParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Bind persistent parent flags too.
	if err := planfileParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Parse options.
	opts := parseDeleteOptions(cmd, v, args)

	// Initialize configuration and store.
	store, err := initDeleteStore(opts)
	if err != nil {
		return err
	}

	// Resolve SHA context.
	resolved, err := resolveContext(opts.All)
	if err != nil {
		return err
	}

	// Build query from component, stack, and SHA.
	query := buildQuery(opts.Component, opts.Stack, resolved.SHA)

	// List matching planfiles.
	ctx := context.Background()
	files, err := store.List(ctx, query)
	if err != nil {
		return err
	}

	if len(files) == 0 {
		ui.Warning("No matching planfiles found")
		return nil
	}

	// Print affected planfiles.
	ui.Warning(fmt.Sprintf("The following %d planfile(s) will be deleted:", len(files)))
	for _, f := range files {
		if f.Metadata != nil {
			ui.Writeln(fmt.Sprintf("  %s/%s (SHA: %s)", f.Metadata.Stack, f.Metadata.Component, f.Metadata.SHA))
		} else {
			ui.Writeln(fmt.Sprintf("  %s", f.Key))
		}
	}

	// Require --force flag or interactive confirmation.
	if !opts.Force {
		if !confirmDeletion() {
			ui.Info("Deletion cancelled")
			return nil
		}
	}

	// Delete each matching planfile.
	deleted := 0
	for _, f := range files {
		if err := store.Delete(ctx, f.Key); err != nil {
			ui.Error(fmt.Sprintf("Failed to delete %s: %v", f.Key, err))
			continue
		}
		deleted++
	}

	ui.Success(fmt.Sprintf("Deleted %d planfile(s) from %s", deleted, store.Name()))
	return nil
}

// confirmDeletion prompts the user for interactive confirmation.
func confirmDeletion() bool {
	defer perf.Track(nil, "planfile.confirmDeletion")()

	ui.Writeln("")                                                        //nolint:errcheck // Best-effort UI output.
	ui.Writeln("Are you sure you want to delete these planfiles? [y/N] ") //nolint:errcheck // Best-effort UI output.

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	input = strings.TrimSpace(strings.ToLower(input))
	return input == "y" || input == "yes"
}

// initDeleteStore initializes the planfile store from options.
func initDeleteStore(opts *DeleteOptions) (planfile.Store, error) {
	configAndStacksInfo := schema.ConfigAndStacksInfo{
		AtmosBasePath:           opts.BasePath,
		AtmosConfigFilesFromArg: opts.Config,
		AtmosConfigDirsFromArg:  opts.ConfigPath,
		ProfilesFromArg:         opts.Profile,
	}

	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return nil, err
	}

	return createStore(&atmosConfig, opts.Store)
}
