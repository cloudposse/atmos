package planfile

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/planfile"
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
	Key   string
	Force bool
}

var deleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Delete a Terraform plan file from storage",
	Long:  `Delete a Terraform plan file from the configured storage backend.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runDelete,
}

func init() {
	// Create parser with delete-specific flags using functional options.
	deleteParser = flags.NewStandardParser(
		flags.WithStringFlag("store", "", "", "Storage backend to use (default from config)"),
		flags.WithBoolFlag("force", "f", false, "Skip confirmation prompt"),
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
	return &DeleteOptions{
		BaseOptions: parseBaseOptions(cmd, v),
		Key:         args[0],
		Force:       v.GetBool("force"),
	}
}

func runDelete(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "planfile.runDelete")()

	// Bind flags to Viper for proper precedence.
	v := viper.GetViper()
	if err := deleteParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	// Parse options.
	opts := parseDeleteOptions(cmd, v, args)

	// Initialize configuration and store.
	store, err := initDeleteStore(opts)
	if err != nil {
		return err
	}

	// Check if planfile exists.
	ctx := context.Background()
	exists, err := store.Exists(ctx, opts.Key)
	if err != nil {
		return err
	}
	if !exists {
		_ = ui.Warning(fmt.Sprintf("Planfile does not exist: %s", opts.Key))
		return nil
	}

	// Require --force flag for deletion.
	if !opts.Force {
		return errUtils.Build(errUtils.ErrPlanfileDeleteRequireForce).
			WithContext("key", opts.Key).
			WithHint("Use --force to confirm deletion").
			Err()
	}

	// Delete.
	if err := store.Delete(ctx, opts.Key); err != nil {
		return err
	}

	_ = ui.Success(fmt.Sprintf("Deleted planfile from %s: %s", store.Name(), opts.Key))
	return nil
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

	storeOpts, err := getStoreOptions(&atmosConfig, opts.Store)
	if err != nil {
		return nil, err
	}

	return planfile.NewStore(storeOpts)
}
