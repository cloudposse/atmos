package cmd

import (
	"errors"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// executeAtmosUI allows command tests to verify routing without opening a terminal.
var executeAtmosUI = e.ExecuteAtmosCmdWithConfig

// runRootCommand opens the stack picker when stacks are available. Projects that
// only use workflows or custom commands can display help without configuring stacks.
func runRootCommand(cmd *cobra.Command, _ []string) error {
	showHelp := func() error {
		// Explicit help avoids the shared renderer's missing-subcommand error path.
		cmd.HelpFunc()(cmd, []string{helpFlagLong})
		return nil
	}

	info := schema.ConfigAndStacksInfo{}
	info.AtmosBasePath, _ = cmd.Flags().GetString("base-path")
	info.AtmosConfigFilesFromArg, _ = cmd.Flags().GetStringSlice("config")
	info.AtmosConfigDirsFromArg, _ = cmd.Flags().GetStringSlice("config-path")

	// Load CLI configuration without requiring stack settings first.
	config, err := cfg.InitCliConfig(info, false)
	if errors.Is(err, cfg.NotFound) {
		return showHelp()
	}
	if err != nil {
		return err
	}
	if config.Stacks.BasePath == "" || len(config.Stacks.IncludedPaths) == 0 {
		return showHelp()
	}

	// Use normal discovery, including exclusions and environment/CLI overrides.
	config, err = cfg.InitCliConfig(info, true)
	if errors.Is(err, errUtils.ErrNoStackManifestsFound) {
		return showHelp()
	}
	if err != nil {
		return err
	}
	// Errors parsing manifests, resolving imports, or starting the UI remain errors.
	return executeAtmosUI(&config)
}
