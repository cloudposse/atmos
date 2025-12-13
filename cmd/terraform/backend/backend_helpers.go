package backend

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=backend_helpers.go -destination=mock_backend_helpers_test.go -package=backend

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

// ConfigInitializer abstracts configuration and auth initialization for testability.
type ConfigInitializer interface {
	InitConfigAndAuth(component, stack, identity string) (*schema.AtmosConfiguration, *schema.AuthContext, error)
}

// CreateBackendParams contains parameters for CreateBackend operation.
type CreateBackendParams struct {
	AtmosConfig  *schema.AtmosConfiguration
	Component    string
	Stack        string
	DescribeFunc func(string, string) (map[string]any, error)
	AuthContext  *schema.AuthContext
}

// DeleteBackendParams contains parameters for DeleteBackend operation.
type DeleteBackendParams struct {
	AtmosConfig  *schema.AtmosConfiguration
	Component    string
	Stack        string
	Force        bool
	DescribeFunc func(string, string) (map[string]any, error)
	AuthContext  *schema.AuthContext
}

// Provisioner abstracts provisioning operations for testability.
type Provisioner interface {
	CreateBackend(params *CreateBackendParams) error
	DeleteBackend(params *DeleteBackendParams) error
	DescribeBackend(atmosConfig *schema.AtmosConfiguration, component string, opts interface{}) error
	ListBackends(atmosConfig *schema.AtmosConfiguration, opts interface{}) error
}

// defaultConfigInitializer implements ConfigInitializer using production code.
type defaultConfigInitializer struct{}

func (d *defaultConfigInitializer) InitConfigAndAuth(component, stack, identity string) (*schema.AtmosConfiguration, *schema.AuthContext, error) {
	return InitConfigAndAuth(component, stack, identity)
}

// defaultProvisioner implements Provisioner using production code.
type defaultProvisioner struct{}

func (d *defaultProvisioner) CreateBackend(params *CreateBackendParams) error {
	return provisioner.ProvisionWithParams(&provisioner.ProvisionParams{
		AtmosConfig:       params.AtmosConfig,
		ProvisionerType:   "backend",
		Component:         params.Component,
		Stack:             params.Stack,
		DescribeComponent: params.DescribeFunc,
		AuthContext:       params.AuthContext,
	})
}

func (d *defaultProvisioner) DeleteBackend(params *DeleteBackendParams) error {
	return provisioner.DeleteBackendWithParams(&provisioner.DeleteBackendParams{
		AtmosConfig:       params.AtmosConfig,
		Component:         params.Component,
		Stack:             params.Stack,
		Force:             params.Force,
		DescribeComponent: params.DescribeFunc,
		AuthContext:       params.AuthContext,
	})
}

func (d *defaultProvisioner) DescribeBackend(atmosConfig *schema.AtmosConfiguration, component string, opts interface{}) error {
	return provisioner.DescribeBackend(atmosConfig, component, opts)
}

func (d *defaultProvisioner) ListBackends(atmosConfig *schema.AtmosConfiguration, opts interface{}) error {
	return provisioner.ListBackends(atmosConfig, opts)
}

// Package-level dependencies for production use. These can be overridden in tests.
var (
	configInit ConfigInitializer = &defaultConfigInitializer{}
	prov       Provisioner       = &defaultProvisioner{}
)

// SetConfigInitializer sets the config initializer (for testing).
// If nil is passed, resets to default implementation.
func SetConfigInitializer(ci ConfigInitializer) {
	if ci == nil {
		configInit = &defaultConfigInitializer{}
		return
	}
	configInit = ci
}

// SetProvisioner sets the provisioner (for testing).
// If nil is passed, resets to default implementation.
func SetProvisioner(p Provisioner) {
	if p == nil {
		prov = &defaultProvisioner{}
		return
	}
	prov = p
}

// ResetDependencies resets dependencies to production defaults (for test cleanup).
func ResetDependencies() {
	configInit = &defaultConfigInitializer{}
	prov = &defaultProvisioner{}
}

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

	// Initialize config and auth using injected dependency.
	atmosConfig, authContext, err := configInit.InitConfigAndAuth(component, opts.Stack, opts.Identity)
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
			AuthManager:          nil, // Auth already handled.
		})
	}

	// Execute provision command using injected provisioner.
	return prov.CreateBackend(&CreateBackendParams{
		AtmosConfig:  atmosConfig,
		Component:    component,
		Stack:        opts.Stack,
		DescribeFunc: describeFunc,
		AuthContext:  authContext,
	})
}
