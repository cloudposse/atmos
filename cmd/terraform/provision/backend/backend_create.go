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

var createParser *flags.StandardParser

// CreateOptions contains parsed flags for the create command.
type CreateOptions struct {
	global.Flags
	Stack    string
	Identity string
}

var createCmd = &cobra.Command{
	Use:     "<component>",
	Short:   "Provision backend infrastructure",
	Long:    `Create or update S3 backend with secure defaults (versioning, encryption, public access blocking). This operation is idempotent.`,
	Example: `  atmos terraform provision backend vpc --stack dev`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "backend.create.RunE")()

		component := args[0]

		// Parse flags using StandardParser with Viper precedence.
		v := viper.GetViper()
		if err := createParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		opts := &CreateOptions{
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

		// Execute provision command using pkg/provision.
		return provision.Provision(&atmosConfig, "backend", component, opts.Stack, describeComponent, authManager)
	},
}

func init() {
	createCmd.DisableFlagParsing = false

	// Create parser with functional options using existing flag builders.
	createParser = flags.NewStandardParser(
		flags.WithStackFlag(),    // Adds stack with env binding
		flags.WithIdentityFlag(), // Adds identity with NoOptDefVal + env binding
	)

	// Register flags with the command.
	createParser.RegisterFlags(createCmd)

	// Bind flags to Viper for environment variable support and precedence handling.
	if err := createParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
