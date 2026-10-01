package backend

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=backend_helpers.go -destination=mock_backend_helpers_test.go -package=backend

import (
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ConfigInitializer abstracts configuration and auth initialization for testability.
type ConfigInitializer interface {
	// InitConfigAndAuth initializes Atmos configuration and optional authentication.
	// The componentPrompted and stackPrompted parameters record whether component/stack
	// were resolved via an interactive prompt (rather than supplied on the command line,
	// via config, or via an environment variable) so a profile-fallback re-exec can
	// re-inject prompted values into the child's argv without duplicating CLI-supplied ones.
	InitConfigAndAuth(component, stack, identity string, componentPrompted, stackPrompted bool) (*schema.AtmosConfiguration, *schema.AuthContext, error)
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

// promptedFlags records which of a backend command's component and stack values were
// resolved via an interactive prompt (see flags.StandardOptions.ComponentPrompted /
// StackPrompted), rather than supplied via CLI flag, positional argument, environment
// variable, or config file. Bundled into a struct (instead of two bool parameters) so the
// execute*CommandWithValues helpers below stay within the linter's function argument limit.
type promptedFlags struct {
	Component bool
	Stack     bool
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

func (d *defaultConfigInitializer) InitConfigAndAuth(component, stack, identity string, componentPrompted, stackPrompted bool) (*schema.AtmosConfiguration, *schema.AuthContext, error) {
	return InitConfigAndAuth(component, stack, identity, componentPrompted, stackPrompted)
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

// InitConfigAndAuth initializes Atmos configuration and optional authentication.
// Returns atmosConfig, authContext, and error.
// It loads component configuration, merges component-level auth with global auth,
// and creates an AuthContext that respects component's default identity settings.
// The componentPrompted and stackPrompted parameters record whether component/stack
// were resolved via an interactive prompt; they flow into auth.ReExecContext so a
// profile-fallback re-exec re-injects prompted values into the child's argv instead
// of losing them (or re-prompting for them) after the re-exec.
func InitConfigAndAuth(component, stack, identity string, componentPrompted, stackPrompted bool) (*schema.AtmosConfiguration, *schema.AuthContext, error) {
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

	// Create AuthManager with merged config (auto-selects component's default identity if
	// present). Use the stack-aware variant: stack-scoped identities (e.g. kind: aws/emulator)
	// need SetStack called before authentication so their PostAuthenticate hook can resolve a
	// live endpoint (e.g. the emulator container started for this specific stack) into
	// AuthContext.AWS. Without the stack, that resolution silently no-ops and callers fall back
	// to the standard AWS SDK credential chain instead of the emulator/local sandbox.
	authManager, err := auth.CreateAndAuthenticateManagerWithReExecContext(identity, mergedAuthConfig, cfg.IdentityFlagSelectValue, &atmosConfig, auth.ReExecContext{
		Component:         component,
		ComponentPrompted: componentPrompted,
		Stack:             stack,
		StackPrompted:     stackPrompted,
	})
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

// executeProvisionCommandWithValues is the internal implementation that accepts already-parsed values.
// Used by commands that use StandardParser's prompting infrastructure. See promptedFlags for
// what prompted records and why it's threaded through to InitConfigAndAuth.
func executeProvisionCommandWithValues(component, stack, identity string, prompted promptedFlags) error {
	// Validate required values.
	if stack == "" {
		return errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
			WithExplanation("--stack flag is required").
			WithHint("Specify a stack with --stack or -s flag").
			Err()
	}

	// Initialize config and auth using injected dependency.
	atmosConfig, authContext, err := configInit.InitConfigAndAuth(component, stack, identity, prompted.Component, prompted.Stack)
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
		Stack:        stack,
		DescribeFunc: describeFunc,
		AuthContext:  authContext,
	})
}

// executeDeleteCommandWithValues is the internal implementation for the delete command.
// Used by commands that use StandardParser's prompting infrastructure. See promptedFlags for
// what prompted records and why it's threaded through to InitConfigAndAuth.
func executeDeleteCommandWithValues(component, stack, identity string, force bool, prompted promptedFlags) error {
	// Validate required values.
	if stack == "" {
		return errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
			WithExplanation("--stack flag is required").
			WithHint("Specify a stack with --stack or -s flag").
			Err()
	}

	// Initialize config and auth using injected dependency.
	atmosConfig, authContext, err := configInit.InitConfigAndAuth(component, stack, identity, prompted.Component, prompted.Stack)
	if err != nil {
		return err
	}

	// Create describe component callback.
	describeFunc := CreateDescribeComponentFunc(nil) // Auth already handled in InitConfigAndAuth.

	// Execute delete command using injected provisioner.
	return prov.DeleteBackend(&DeleteBackendParams{
		AtmosConfig:  atmosConfig,
		Component:    component,
		Stack:        stack,
		Force:        force,
		DescribeFunc: describeFunc,
		AuthContext:  authContext,
	})
}

// executeDescribeCommandWithValues is the internal implementation for the describe command.
// Used by commands that use StandardParser's prompting infrastructure. See promptedFlags for
// what prompted records and why it's threaded through to InitConfigAndAuth.
func executeDescribeCommandWithValues(component, stack, identity, format string, prompted promptedFlags) error {
	// Validate required values.
	if stack == "" {
		return errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
			WithExplanation("--stack flag is required").
			WithHint("Specify a stack with --stack or -s flag").
			Err()
	}

	// Initialize config using injected dependency.
	atmosConfig, _, err := configInit.InitConfigAndAuth(component, stack, identity, prompted.Component, prompted.Stack)
	if err != nil {
		return err
	}

	// Execute describe command using injected provisioner.
	return prov.DescribeBackend(atmosConfig, component, map[string]string{"format": format})
}

// executeListCommandWithValues is the internal implementation for the list command.
// Used by commands that use StandardParser's prompting infrastructure. The stackPrompted
// parameter records whether stack was filled in via an interactive prompt (see
// flags.StandardOptions.StackPrompted), so it can be threaded into auth.ReExecContext for
// profile-fallback re-exec. There's no component parameter here (list operates across all
// components in the stack), so componentPrompted is always false when calling InitConfigAndAuth.
func executeListCommandWithValues(stack, identity, format string, stackPrompted bool) error {
	// Validate required values.
	if stack == "" {
		return errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
			WithExplanation("--stack flag is required").
			WithHint("Specify a stack with --stack or -s flag").
			Err()
	}

	// Initialize config using injected dependency (no component needed for list).
	atmosConfig, _, err := configInit.InitConfigAndAuth("", stack, identity, false, stackPrompted)
	if err != nil {
		return err
	}

	// Execute list command using injected provisioner.
	return prov.ListBackends(atmosConfig, map[string]string{"format": format})
}
