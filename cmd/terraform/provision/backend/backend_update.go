//nolint:dupl // CRUD commands share similar structure intentionally - standard command pattern.
package backend

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provision"
	"github.com/cloudposse/atmos/pkg/schema"
)

//nolint:dupl // Update shares logic with create intentionally - both provision backends (idempotent operation).
var updateParser *flags.StandardParser

// UpdateOptions contains parsed flags for the update command.
type UpdateOptions struct {
	global.Flags
	Stack    string
	Identity string
}

var updateCmd = &cobra.Command{
	Use:   "update <component>",
	Short: "Update backend configuration",
	Long: `Apply configuration changes to existing backend.

This operation is idempotent and will update backend settings like
versioning, encryption, and public access blocking to match secure defaults.`,
	Example: `  atmos terraform provision backend update vpc --stack dev`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "backend.update.RunE")()

		component := args[0]

		// Parse flags using StandardParser with Viper precedence.
		v := viper.GetViper()
		if err := updateParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &UpdateOptions{
			Flags:    flags.ParseGlobalFlags(cmd, v),
			Stack:    v.GetString("stack"),
			Identity: v.GetString("identity"),
		}

		if opts.Stack == "" {
			return errUtils.ErrRequiredFlagNotProvided
		}

		// Load atmos configuration.
		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{
			ComponentFromArg: component,
			Stack:            opts.Stack,
		}, false)
		if err != nil {
			return errors.Join(errUtils.ErrFailedToInitConfig, err)
		}

		// Create AuthManager from identity flag if provided.
		var authManager auth.AuthManager
		if opts.Identity != "" {
			authManager, err = auth.CreateAndAuthenticateManager(opts.Identity, &atmosConfig.Auth, cfg.IdentityFlagSelectValue)
			if err != nil {
				return err
			}
		}

		// Create describe component function that calls internal/exec.
		describeComponent := func(component, stack string) (map[string]any, error) {
			return e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
				Component:            component,
				Stack:                stack,
				ProcessTemplates:     false,
				ProcessYamlFunctions: false,
				Skip:                 nil,
				AuthManager:          authManager,
			})
		}

		// Execute update command using pkg/provision (reuses provision logic which is idempotent).
		return provision.Provision(&atmosConfig, "backend", component, opts.Stack, describeComponent, authManager)
	},
}

func init() {
	updateCmd.DisableFlagParsing = false

	// Create parser with functional options.
	updateParser = flags.NewStandardParser(
		flags.WithStackFlag(),
		flags.WithIdentityFlag(),
	)

	// Register flags with the command.
	updateParser.RegisterFlags(updateCmd)

	// Bind flags to Viper.
	if err := updateParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
