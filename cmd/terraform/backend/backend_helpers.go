package backend

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/global"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner"
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
// Returns atmosConfig, authContext, and error.
// It loads component configuration, merges component-level auth with global auth,
// and creates an AuthContext that respects component's default identity settings.
func InitConfigAndAuth(component, stack, identity string) (*schema.AtmosConfiguration, *schema.AuthContext, error) {
	// Load atmos configuration.
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{
		ComponentFromArg: component,
		Stack:            stack,
	}, false)
	if err != nil {
		return nil, nil, errors.Join(errUtils.ErrFailedToInitConfig, err)
	}

	// Load component configuration to get component-level auth settings.
	componentConfig, err := e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 nil,
		AuthManager:          nil, // Don't need auth to describe component
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load component config: %w", err)
	}

	// Merge component auth with global auth (component auth takes precedence).
	mergedAuthConfig, err := auth.MergeComponentAuthFromConfig(&atmosConfig.Auth, componentConfig, &atmosConfig, cfg.AuthSectionName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to merge component auth: %w", err)
	}

	// Create AuthManager with merged config (auto-selects component's default identity if present).
	authManager, err := auth.CreateAndAuthenticateManager(identity, mergedAuthConfig, cfg.IdentityFlagSelectValue)
	if err != nil {
		return nil, nil, err
	}

	// Get AuthContext from AuthManager.
	var authContext *schema.AuthContext
	if authManager != nil {
		stackInfo := authManager.GetStackInfo()
		if stackInfo != nil {
			authContext = stackInfo.AuthContext
		}
	}

	return &atmosConfig, authContext, nil
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

	// Initialize config and auth (now returns AuthContext instead of AuthManager).
	atmosConfig, authContext, err := InitConfigAndAuth(component, opts.Stack, opts.Identity)
	if err != nil {
		return err
	}

	// Create describe component callback.
	// Note: We don't need to pass authContext to the describe function for backend provisioning
	// since we already loaded the component config in InitConfigAndAuth.
	describeFunc := func(component, stack string) (map[string]any, error) {
		return e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
			Component:            component,
			Stack:                stack,
			ProcessTemplates:     false,
			ProcessYamlFunctions: false,
			Skip:                 nil,
			AuthManager:          nil, // Auth already handled
		})
	}

	// Execute provision command using pkg/provisioner.
	return provisioner.Provision(atmosConfig, "backend", component, opts.Stack, describeFunc, authContext)
}
