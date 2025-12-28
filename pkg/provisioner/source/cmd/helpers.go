package cmd

import (
	"context"
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
	"github.com/cloudposse/atmos/pkg/provisioner/source"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Package-level function variables for testing (can be replaced in tests).
var (
	initCliConfigFunc     = cfg.InitCliConfig
	mergeAuthFunc         = auth.MergeComponentAuthFromConfig
	createAuthFunc        = auth.CreateAndAuthenticateManager
	describeComponentFunc = executeDescribeComponentDefault
)

// executeDescribeComponentDefault is the default implementation for describing components.
func executeDescribeComponentDefault(component, stack string) (map[string]any, error) {
	return e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		Skip:                 nil,
		AuthManager:          nil,
	})
}

// CommonOptions contains the standard flags shared by all source commands.
type CommonOptions struct {
	global.Flags
	Stack    string
	Identity string
	Force    bool
}

// ParseCommonFlags parses common flags (stack, identity, force) using StandardParser with Viper precedence.
func ParseCommonFlags(cmd *cobra.Command, parser *flags.StandardParser) (*CommonOptions, error) {
	defer perf.Track(nil, "source.cmd.ParseCommonFlags")()

	v := viper.GetViper()
	if err := parser.BindFlagsToViper(cmd, v); err != nil {
		return nil, err
	}

	opts := &CommonOptions{
		Flags:    flags.ParseGlobalFlags(cmd, v),
		Stack:    v.GetString("stack"),
		Identity: v.GetString("identity"),
		Force:    v.GetBool("force"),
	}

	if opts.Stack == "" {
		return nil, errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
			WithExplanation("--stack flag is required").
			Err()
	}

	return opts, nil
}

// InitConfigAndAuth initializes Atmos configuration and optional authentication.
// Returns atmosConfig, authContext, and error.
// The globalFlags parameter wires CLI global flags (--base-path, --config, --config-path, --profile)
// to the configuration loader.
func InitConfigAndAuth(component, stack, identity string, globalFlags *global.Flags) (*schema.AtmosConfiguration, *schema.AuthContext, error) {
	defer perf.Track(nil, "source.cmd.InitConfigAndAuth")()

	// Build config info with global flag values.
	configInfo := schema.ConfigAndStacksInfo{
		ComponentFromArg: component,
		Stack:            stack,
	}

	// Wire global flags to config info if provided.
	if globalFlags != nil {
		configInfo.AtmosBasePath = globalFlags.BasePath
		configInfo.AtmosConfigFilesFromArg = globalFlags.Config
		configInfo.AtmosConfigDirsFromArg = globalFlags.ConfigPath
		configInfo.ProfilesFromArg = globalFlags.Profile
	}

	// Load atmos configuration.
	atmosConfig, err := initCliConfigFunc(configInfo, false)
	if err != nil {
		return nil, nil, errors.Join(errUtils.ErrFailedToInitConfig, err)
	}

	// Load component configuration to get component-level auth settings.
	componentConfig, err := describeComponentFunc(component, stack)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load component config: %w", err)
	}

	// Merge component auth with global auth (component auth takes precedence).
	mergedAuthConfig, err := mergeAuthFunc(&atmosConfig.Auth, componentConfig, &atmosConfig, cfg.AuthSectionName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to merge component auth: %w", err)
	}

	// Create AuthManager with merged config (auto-selects component's default identity if present).
	authManager, err := createAuthFunc(identity, mergedAuthConfig, cfg.IdentityFlagSelectValue)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create auth manager: %w", err)
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

// DescribeComponent returns the component configuration from stack.
func DescribeComponent(component, stack string) (map[string]any, error) {
	defer perf.Track(nil, "source.cmd.DescribeComponent")()

	return describeComponentFunc(component, stack)
}

// ProvisionSourceOptions holds parameters for provisioning a component source.
type ProvisionSourceOptions struct {
	AtmosConfig     *schema.AtmosConfiguration
	ComponentType   string // "terraform", "helmfile", "packer"
	Component       string
	Stack           string
	ComponentConfig map[string]any
	AuthContext     *schema.AuthContext
	Force           bool
}

// ProvisionSource vendors a component source based on the source configuration.
func ProvisionSource(ctx context.Context, opts *ProvisionSourceOptions) error {
	defer perf.Track(opts.AtmosConfig, "source.cmd.ProvisionSource")()

	return source.Provision(ctx, &source.ProvisionParams{
		AtmosConfig:     opts.AtmosConfig,
		ComponentType:   opts.ComponentType,
		Component:       opts.Component,
		Stack:           opts.Stack,
		ComponentConfig: opts.ComponentConfig,
		AuthContext:     opts.AuthContext,
		Force:           opts.Force,
	})
}
