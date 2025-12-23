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

var deleteCmd = &cobra.Command{
	Use:   "delete <key>",
	Short: "Delete a Terraform plan file from storage",
	Long:  `Delete a Terraform plan file from the configured storage backend.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runDelete,
}

var (
	deleteStore string
	deleteForce bool
)

func init() {
	deleteCmd.Flags().StringVar(&deleteStore, "store", "", "Storage backend to use (default from config)")
	deleteCmd.Flags().BoolVar(&deleteForce, "force", false, "Skip confirmation prompt")
}

func runDelete(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "planfile.runDelete")()

	key := args[0]

	// Get global flags from Viper (includes base-path, config, config-path, profile).
	v := viper.GetViper()
	globalFlags := flags.ParseGlobalFlags(cmd, v)

	// Build ConfigAndStacksInfo from global flags to honor config selection flags.
	configAndStacksInfo := schema.ConfigAndStacksInfo{
		AtmosBasePath:           globalFlags.BasePath,
		AtmosConfigFilesFromArg: globalFlags.Config,
		AtmosConfigDirsFromArg:  globalFlags.ConfigPath,
		ProfilesFromArg:         globalFlags.Profile,
	}

	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(configAndStacksInfo, true)
	if err != nil {
		return err
	}

	// Get the storage configuration.
	storeOpts, err := getStoreOptions(&atmosConfig, deleteStore)
	if err != nil {
		return err
	}

	// Create the store.
	store, err := planfile.NewStore(storeOpts)
	if err != nil {
		return err
	}

	// Check if planfile exists.
	ctx := context.Background()
	exists, err := store.Exists(ctx, key)
	if err != nil {
		return err
	}

	if !exists {
		_ = ui.Warning(fmt.Sprintf("Planfile does not exist: %s", key))
		return nil
	}

	// Require --force flag for deletion.
	if !deleteForce {
		return fmt.Errorf("%w: %s", errUtils.ErrPlanfileDeleteRequireForce, key)
	}

	// Delete.
	if err := store.Delete(ctx, key); err != nil {
		return err
	}

	_ = ui.Success(fmt.Sprintf("Deleted planfile from %s: %s", store.Name(), key))
	return nil
}
