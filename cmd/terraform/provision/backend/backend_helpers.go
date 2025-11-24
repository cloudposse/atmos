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

// CommonOptions contains the standard flags shared by all backend commands.
type CommonOptions struct {
	global.Flags
	Stack    string
	Identity string
}

// ParseCommonFlags parses common flags (stack, identity) using StandardParser with Viper precedence.
func ParseCommonFlags(cmd *cobra.Command, parser *flags.StandardParser) (*CommonOptions, error) {
	v := viper.GetViper()
	if err := parser.BindFlagsToViper(cmd, v); err != nil {
		return nil, err
	}

	opts := &CommonOptions{
		Flags:    flags.ParseGlobalFlags(cmd, v),
		Stack:    v.GetString("stack"),
		Identity: v.GetString("identity"),
	}

	if opts.Stack == "" {
		return nil, errUtils.ErrRequiredFlagNotProvided
	}

	return opts, nil
}

// InitConfigAndAuth initializes Atmos configuration and optional authentication.
// Returns atmosConfig, authManager, and error.
func InitConfigAndAuth(component, stack, identity string) (*schema.AtmosConfiguration, auth.AuthManager, error) {
	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{
		ComponentFromArg: component,
		Stack:            stack,
	}, false)
	if err != nil {
		return nil, nil, errors.Join(errUtils.ErrFailedToInitConfig, err)
	}

	// Create AuthManager from identity flag if provided.
	var authManager auth.AuthManager
	if identity != "" {
		authManager, err = auth.CreateAndAuthenticateManager(identity, &atmosConfig.Auth, cfg.IdentityFlagSelectValue)
		if err != nil {
			return nil, nil, err
		}
	}

	return &atmosConfig, authManager, nil
}

// CreateDescribeComponentFunc creates a describe component function with the given authManager.
func CreateDescribeComponentFunc(authManager auth.AuthManager) func(string, string) (map[string]any, error) {
	return func(component, stack string) (map[string]any, error) {
		return e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
			Component:            component,
			Stack:                stack,
			ProcessTemplates:     false,
			ProcessYamlFunctions: false,
			Skip:                 nil,
			AuthManager:          authManager,
		})
	}
}

// ExecuteProvisionCommand is the shared RunE implementation for create and update commands.
// Both operations are idempotent - they provision or update the backend to match the desired state.
func ExecuteProvisionCommand(cmd *cobra.Command, args []string, parser *flags.StandardParser, perfLabel string) error {
	defer perf.Track(atmosConfigPtr, perfLabel)()

	component := args[0]

	// Parse common flags.
	opts, err := ParseCommonFlags(cmd, parser)
	if err != nil {
		return err
	}

	// Initialize config and auth.
	atmosConfig, authManager, err := InitConfigAndAuth(component, opts.Stack, opts.Identity)
	if err != nil {
		return err
	}

	// Execute provision command using pkg/provision.
	return provision.Provision(atmosConfig, "backend", component, opts.Stack, CreateDescribeComponentFunc(authManager), authManager)
}
