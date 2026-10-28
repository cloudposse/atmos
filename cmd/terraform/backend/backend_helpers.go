package backend

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=backend_helpers.go -destination=mock_backend_helpers_test.go -package=backend

import (
	"errors"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
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
		AuthDisabled:         true,
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
	return createDescribeComponentFunc(nil, authManager)
}

// createDescribeComponentFunc resolves only backend environment references after authentication.
// Other functions remain deferred: provisioning a backend must not require its own state.
func createDescribeComponentFunc(atmosConfig *schema.AtmosConfiguration, authManager auth.AuthManager) func(string, string) (map[string]any, error) {
	return func(component, stack string) (map[string]any, error) {
		effectiveConfig := atmosConfig
		config, err := e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
			Component:            component,
			Stack:                stack,
			ProcessTemplates:     false,
			ProcessYamlFunctions: false,
			Skip:                 nil,
			AuthManager:          authManager,
			AuthDisabled:         authManager == nil,
		})
		if err != nil {
			return nil, err
		}
		if effectiveConfig == nil {
			loaded, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{ComponentFromArg: component, Stack: stack}, false)
			if err != nil {
				return nil, err
			}
			effectiveConfig = &loaded
		}
		return resolveBackendEnvironment(effectiveConfig, config, component, stack)
	}
}

func resolveBackendEnvironment(atmosConfig *schema.AtmosConfiguration, config map[string]any, component, stack string) (map[string]any, error) {
	// Use the normal evaluator so explicit-tag provenance, edition policy, defaults,
	// and component-env precedence remain the same as other stack consumers.
	skip := make([]string, 0, len(u.AtmosYamlTags))
	for _, tag := range u.AtmosYamlTags {
		if tag != u.AtmosYamlFuncEnv {
			skip = append(skip, strings.TrimPrefix(tag, "!"))
		}
	}
	env, _ := config[cfg.EnvSectionName].(map[string]any)
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: component, Stack: stack, ComponentEnvSection: env}
	backend, err := e.ProcessCustomYamlTags(atmosConfig, schema.AtmosSectionMapType{
		"backend": config["backend"], "backend_type": config["backend_type"],
	}, stack, skip, info)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve backend environment: %w", err)
	}
	config["backend"], config["backend_type"] = backend["backend"], backend["backend_type"]
	return config, nil
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

	// Authentication is already complete. Only environment references are evaluated;
	// the selected auth context is passed directly to the backend provisioner below.
	describeFunc := createDescribeComponentFunc(atmosConfig, nil)

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
	describeFunc := createDescribeComponentFunc(atmosConfig, nil)

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
// Parsed identity and prompt provenance are retained in the call contract, but unsupported
// inspection does not initialize credentials or load component configuration.
func executeDescribeCommandWithValues(component, stack, identity, format string, prompted promptedFlags) error {
	// Validate required values.
	if stack == "" {
		return errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
			WithExplanation("--stack flag is required").
			WithHint("Specify a stack with --stack or -s flag").
			Err()
	}

	// Inspection is not implemented. Do not load a stack or authenticate merely
	// to report that fact, especially when credentials would prompt or re-exec.
	return prov.DescribeBackend(nil, component, map[string]string{"format": format})
}

// executeListCommandWithValues is the internal implementation for the list command.
// Keep the parser's identity and prompt-provenance arguments for compatibility; listing
// remains unsupported and must not initialize credentials or describe an empty component.
func executeListCommandWithValues(stack, identity, format string, stackPrompted bool) error {
	// Validate required values.
	if stack == "" {
		return errUtils.Build(errUtils.ErrRequiredFlagNotProvided).
			WithExplanation("--stack flag is required").
			WithHint("Specify a stack with --stack or -s flag").
			Err()
	}

	// Listing is not implemented and must not attempt to describe an empty component.
	return prov.ListBackends(nil, map[string]string{"format": format})
}
