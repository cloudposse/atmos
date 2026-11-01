// Package terraform implements the Terraform component provider and runtime.
// Shared host services are injected; this package does not import internal/exec.
package terraform

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_runtime_test.go -package=$GOPACKAGE

import (
	context "context"
	fmt "fmt"
	filepath "path/filepath"
	slices "slices"
	sort "sort"

	cobra "github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	auth "github.com/cloudposse/atmos/pkg/auth"
	component "github.com/cloudposse/atmos/pkg/component"
	deferred "github.com/cloudposse/atmos/pkg/deferred"
	degradation "github.com/cloudposse/atmos/pkg/degradation"
	"github.com/cloudposse/atmos/pkg/dependencies"
	dependency "github.com/cloudposse/atmos/pkg/dependency"
	generator "github.com/cloudposse/atmos/pkg/generator"
	providers "github.com/cloudposse/atmos/pkg/generator/providers"
	required_providers "github.com/cloudposse/atmos/pkg/generator/required_providers"
	git "github.com/cloudposse/atmos/pkg/git"
	log "github.com/cloudposse/atmos/pkg/logger"
	perf "github.com/cloudposse/atmos/pkg/perf"
	pro "github.com/cloudposse/atmos/pkg/pro"
	provisioner "github.com/cloudposse/atmos/pkg/provisioner"
	schema "github.com/cloudposse/atmos/pkg/schema"
	tf "github.com/cloudposse/atmos/pkg/terraform"
	tfbackend "github.com/cloudposse/atmos/pkg/terraform/backend"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Runtime supplies Terraform execution and artifact generation.
// Implementations must preserve execution options and error identity.
type Runtime interface {
	Execute(*component.ExecutionContext) error
	GenerateArtifacts(*component.ExecutionContext) error
}

// Provider is the built-in Terraform component implementation. Registration is
// performed by the host that supplies Runtime; importing this package has no
// registration side effects and cannot overwrite an installed provider.
type Provider struct{ runtime Runtime }

var _ component.ComponentProvider = (*Provider)(nil)

// New constructs a provider with the supplied Terraform runtime.
func New(runtime Runtime) *Provider {
	defer perf.Track(nil, "terraform.New")()

	return &Provider{runtime: runtime}
}

// GetType returns the stable component family, including OpenTofu executions.
func (*Provider) GetType() string {
	defer perf.Track(nil, "terraform.Provider.GetType")()

	return "terraform"
}

// GetGroup returns the infrastructure tooling group.
func (*Provider) GetGroup() string {
	defer perf.Track(nil, "terraform.Provider.GetGroup")()

	return "Infrastructure as Code"
}

// GetBasePath returns the configured source directory or its conventional default.
func (*Provider) GetBasePath(config *schema.AtmosConfiguration) string {
	defer perf.Track(nil, "terraform.Provider.GetBasePath")()

	if config != nil && config.Components.Terraform.BasePath != "" {
		return config.Components.Terraform.BasePath
	}
	return "components/terraform"
}

// ListComponents lists configured Terraform instances without evaluating them.
func (*Provider) ListComponents(ctx context.Context, _ string, stack map[string]any) ([]string, error) {
	defer perf.Track(nil, "terraform.Provider.ListComponents")()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	components, _ := stack["components"].(map[string]any)
	terraform, _ := components["terraform"].(map[string]any)
	names := make([]string, 0, len(terraform))
	for name := range terraform {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// ValidateComponent validates the resolved backend type's shape. Policy and
// module validation remain in the lifecycle, where stack and auth are available.
func (*Provider) ValidateComponent(config map[string]any) error {
	defer perf.Track(nil, "terraform.Provider.ValidateComponent")()

	if value, exists := config["backend_type"]; exists && value != nil {
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%w: backend_type must be a string", errUtils.ErrComponentConfigInvalid)
		}
	}
	return nil
}

// Execute runs a Terraform operation using the component runtime.
func (p *Provider) Execute(ctx *component.ExecutionContext) error {
	defer perf.Track(nil, "terraform.Provider.Execute")()

	if err := p.checkContext(ctx); err != nil {
		return err
	}
	return p.runtime.Execute(ctx)
}

// GenerateArtifacts writes files for an already resolved component. It does not
// initialize Terraform, select a workspace, or execute a plan/apply operation.
func (p *Provider) GenerateArtifacts(ctx *component.ExecutionContext) error {
	defer perf.Track(nil, "terraform.Provider.GenerateArtifacts")()

	if err := p.checkContext(ctx); err != nil {
		return err
	}
	if ctx.AtmosConfig == nil || ctx.WorkingDir == "" {
		return fmt.Errorf("%w: Terraform artifact generation requires configuration and a working directory", errUtils.ErrComponentConfigInvalid)
	}
	return p.runtime.GenerateArtifacts(ctx)
}

// checkContext rejects missing requests, cancelled callers, and providers without a host runtime.
func (p *Provider) checkContext(ctx *component.ExecutionContext) error {
	if ctx == nil {
		return fmt.Errorf("%w: missing Terraform execution context", errUtils.ErrComponentConfigInvalid)
	}
	if err := ctx.GoContext().Err(); err != nil {
		return err
	}
	if p.runtime == nil {
		return fmt.Errorf("%w: Terraform runtime is not configured", errUtils.ErrComponentExecutionFailed)
	}
	return nil
}

// GetAvailableCommands advertises tool commands. Execution deliberately remains
// open to passthrough commands supported by the selected Terraform/OpenTofu binary.
func (*Provider) GetAvailableCommands() []string {
	defer perf.Track(nil, "terraform.Provider.GetAvailableCommands")()

	return []string{"apply", "console", "deploy", "destroy", "fmt", "force-unlock", "get", "graph", "import", "init", "login", "logout", "metadata", "output", "plan", "providers", "refresh", "show", "state", "taint", "test", "untaint", "validate", "version", "workspace"}
}

var _ Runtime = (*Executor)(nil)

// Execute preserves caller options while making the provider request's context authoritative.
func (e *Executor) Execute(ctx *component.ExecutionContext) error {
	defer perf.Track(nil, "terraform.Executor.Execute")()
	opts := append(slices.Clone(ctx.Options), component.WithProcessContext(ctx.GoContext()))
	return e.execute(ctx.ConfigAndStacksInfo, opts...)
}

// GenerateArtifacts writes configuration in the prepared component working directory.
func (e *Executor) GenerateArtifacts(ctx *component.ExecutionContext) error {
	defer perf.Track(nil, "terraform.Executor.GenerateArtifacts")()
	info := ctx.ConfigAndStacksInfo
	return e.GenerateConfigFiles(ctx.AtmosConfig, &info, ctx.WorkingDir)
}

// Services supplies the shared Atmos facilities used by one Terraform executor.
type Services struct {
	OperationServices
	ExecuteDescribeComponent                      func(params *component.ExecuteDescribeComponentParams) (map[string]any, error)
	ExecuteDescribeStacksWithAuthDisabled         func(atmosConfig *schema.AtmosConfiguration, filterByStack string, components []string, componentTypes []string, sections []string, ignoreMissingFiles bool, processTemplates bool, processYamlFunctions bool, includeEmptyStacks bool, skip []string, authManager auth.AuthManager, authDisabled bool) (map[string]any, error)
	ExecuteDescribeStacksWithAuthDisabledAndMocks func(atmosConfig *schema.AtmosConfiguration, filterByStack string, components []string, componentTypes []string, sections []string, ignoreMissingFiles bool, processTemplates bool, processYamlFunctions bool, includeEmptyStacks bool, skip []string, authManager auth.AuthManager, authDisabled bool, useMocks bool) (map[string]any, error)
	ExecuteDescribeStacksWithMocks                func(atmosConfig *schema.AtmosConfiguration, filterByStack string, components []string, componentTypes []string, sections []string, ignoreMissingFiles bool, processTemplates bool, processYamlFunctions bool, includeEmptyStacks bool, skip []string, authManager auth.AuthManager, useMocks bool, tagsFilter []string, labelsFilter map[string]string, errOptions component.DescribeStacksErrorOptions) (map[string]any, error)
	ExecuteShellCommand                           func(atmosConfig schema.AtmosConfiguration, command string, args []string, dir string, env []string, dryRun bool, redirectStdError string, opts ...component.ExecutionOption) error
	ExecuteTerraform                              func(info schema.ConfigAndStacksInfo, opts ...component.ExecutionOption) error
	GenerateFilesForComponent                     func(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error
	GetAffectedComponents                         func(args *component.DescribeAffectedCmdArgs) ([]schema.Affected, error)
	GetEagerEvaluationSetting                     func(atmosConfig *schema.AtmosConfiguration) bool
	NewDependencyParserWithDelimiter              func(builder *dependency.GraphBuilder, nodeMap map[string]string, targetStates map[string]string, leftDelim string) DependencyParser
	ProcessStacks                                 func(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo schema.ConfigAndStacksInfo, checkStack bool, processTemplates bool, processYamlFunctions bool, skip []string, authManager auth.AuthManager) (schema.ConfigAndStacksInfo, error)
	ValidateComponent                             func(atmosConfig *schema.AtmosConfiguration, componentName string, componentSection map[string]any, schemaPath string, schemaType string, modulePaths []string, timeoutSeconds int) (bool, error)
	AttachComponentReporting                      func(result *error, info *schema.ConfigAndStacksInfo, componentType string, command string)
	CreateAndAuthenticateAuthManager              func(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (auth.AuthManager, error)
	DefaultAuthManagerCreator                     func(string, *schema.AuthConfig, string, *schema.AtmosConfiguration, auth.ReExecContext) (auth.AuthManager, error)
	DefaultMergedAuthConfigGetter                 func(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*schema.AuthConfig, error)
	DeferredTargetAuth                            func(ac *schema.AtmosConfiguration, component string, stack string, parents ...auth.AuthManager) (auth.AuthManager, error)
	DeferredTargetAuthAndCache                    func(ac *schema.AtmosConfiguration, component string, stack string, parents ...auth.AuthManager) (auth.AuthManager, *deferred.ValueCache, error)
	EnvVarFromList                                func(env []string, key string) string
	ExecTerraformShellCommand                     func(atmosConfig *schema.AtmosConfiguration, component string, stack string, componentEnvList []string, varFile string, workingDir string, workspaceName string, componentPath string) error
	GetMergedAuthConfig                           func(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*schema.AuthConfig, error)
	HasYqDefault                                  func(yqExpr string) bool
	IsComponentEnabled                            func(metadataSection map[string]any, componentName string) bool
	MapCIExitCode                                 func(atmosConfig *schema.AtmosConfiguration, exitCode int) int
	ResolveIdentityConfigError                    func(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, err error, wrapSentinel error) error
	ResolvedTargetAuthContext                     func(ac *schema.AtmosConfiguration, manager auth.AuthManager, fallback *schema.AuthContext, disabled bool) *schema.AuthContext
	ShouldUploadStatus                            func(info *schema.ConfigAndStacksInfo) bool
	StoreAutoDetectedIdentity                     func(authManager auth.AuthManager, info *schema.ConfigAndStacksInfo)
	UploadStatus                                  func(info *schema.ConfigAndStacksInfo, exitCode int, client pro.AtmosProAPIClientInterface, gitRepo git.GitRepoInterface) error
}

// Executor owns Terraform's execution pipeline and reference lookups.
type Executor struct {
	*Operations
	prepareAuthEnvironmentFn      func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) error
	resolveConfigurationToolsFn   func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) (*dependencies.ToolchainEnvironment, error)
	services                      Services
	executeTerraformInitForcedFn  func(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath, varFile string, reconfigure, upgrade bool, opts ...component.ExecutionOption) error
	dispatchAfterInitFn           func(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string, opts ...component.ExecutionOption)
	executeMainTerraformCommandFn func(
		atmosConfig *schema.AtmosConfiguration,
		info *schema.ConfigAndStacksInfo,
		allArgsAndFlags []string,
		componentPath string,
		uploadStatusFlag bool,
		opts ...component.ExecutionOption,
	) error
	shellInitFn      func(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath, varFile string, opts ...component.ExecutionOption) error
	shellWorkspaceFn func(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string, opts ...component.ExecutionOption) error
	shellExecFn      func(*schema.AtmosConfiguration, string, string, []string, string, string, string, string) error
}

// NewExecutor binds shared host services to a Terraform executor.
//
//nolint:gocritic // Copy service bindings so later changes to the caller's struct cannot change this executor.
func NewExecutor(services Services) *Executor {
	defer perf.Track(nil, "terraform.NewExecutor")()

	e := &Executor{Operations: NewOperations(services.OperationServices), services: services}
	e.prepareAuthEnvironmentFn = auth.TerraformPreHook
	e.resolveConfigurationToolsFn = resolveAndInstallToolchainDeps
	e.executeTerraformInitForcedFn = e.executeTerraformInitForced
	e.dispatchAfterInitFn = e.dispatchAfterInit
	e.executeMainTerraformCommandFn = e.executeMainTerraformCommand
	e.shellInitFn = e.executeTerraformInitCommand
	e.shellWorkspaceFn = e.runWorkspaceSetup
	e.shellExecFn = services.ExecTerraformShellCommand
	return e
}

// DependencyParser is the shared graph parser used by Terraform dependency discovery.
type DependencyParser interface {
	ParseComponentDependencies(stackName, componentName string, componentSection map[string]any) error
}

// ComponentAuthManagerResolver constructs component-scoped authentication.
type ComponentAuthManagerResolver = func(*schema.AtmosConfiguration, map[string]any, string, string, auth.AuthManager) (auth.AuthManager, error)

// ComponentAuthResolver resolves authentication for a nested component lookup.
type ComponentAuthResolver = func(*schema.AtmosConfiguration, string, string, auth.AuthManager) (auth.AuthManager, error)

// executionOutputWriters forwards configured process streams to provisioning and generation.
// Without streams, provisioners retain their default output handling.
func executionOutputWriters(opts ...component.ExecutionOption) provisioner.OutputWriters {
	var options component.ExecutionOptions
	for _, opt := range opts {
		opt(&options)
	}
	if options.Streams == nil {
		return provisioner.OutputWriters{}
	}
	return provisioner.OutputWriters{Stdout: options.Streams.Stdout, Stderr: options.Streams.Stderr}
}

// OperationServices supplies shared Atmos configuration and execution services.
// Implementations are injected per Operations instance; no process-global callbacks are installed.
type OperationServices interface {
	GenerateFilesForComponent(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error
	ProcessStacks(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo schema.ConfigAndStacksInfo, checkStack bool, processTemplates bool, processYamlFunctions bool, skip []string, authManager auth.AuthManager) (schema.ConfigAndStacksInfo, error)
	FindStacksMap(atmosConfig *schema.AtmosConfiguration, ignoreMissingFiles bool) (map[string]any, map[string]map[string]any, component.AllStacksDeferredContexts, error)
	ProcessTmpl(atmosConfig *schema.AtmosConfiguration, tmplName string, tmplValue string, tmplData any, ignoreMissingTemplateValues bool) (string, error)
	ProcessTmplWithDatasources(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo, settingsSection schema.Settings, tmplName string, tmplValue string, tmplData any, ignoreMissingTemplateValues bool) (string, error)
	ProcessCustomYamlTags(atmosConfig *schema.AtmosConfiguration, input schema.AtmosSectionMapType, currentStack string, skip []string, stackInfo *schema.ConfigAndStacksInfo) (schema.AtmosSectionMapType, error)
	ProcessCommandLineArgs(componentType string, cmd *cobra.Command, args []string, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error)
	ResolveDeferredYamlFunctions(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo, settingsSectionStruct *schema.Settings, componentTemplateContext map[string]any, skip []string, evalSections []string, warnings ...func(degradation.Warning)) error
	PrepareConfigurationValues(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, skip []string, sections []string) ([]string, func() error)
	StackLocalsForComponent(rawStackConfigs map[string]map[string]any, stackFile string, componentType string) map[string]any
	CloneComponentSectionWithOverrides(raw map[string]any, overrides map[string]any) map[string]any
	ContainsStarlark(value any) bool
	BuildTerraformWorkspace(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo schema.ConfigAndStacksInfo) (string, error)
	ExecuteTerraform(info schema.ConfigAndStacksInfo, opts ...component.ExecutionOption) error
}

// Operations owns Terraform generation, plan comparison, and verification.
type Operations struct{ host OperationServices }

// NewOperations binds shared services to a Terraform operations instance.
func NewOperations(host OperationServices) *Operations {
	defer perf.Track(nil, "terraform.NewOperations")()
	return &Operations{host: host}
}

type (
	GenerateBackendOptions = tf.GenerateBackendOptions
	VarfileOptions         = tf.VarfileOptions
	ShellCommandOption     = component.ExecutionOption
)

const (
	filePermissions = 0o644
	planFileMode    = 0o600
)

// generateComponentBackendConfig builds backend settings for the selected Terraform workspace.
func generateComponentBackendConfig(backendType string, backendConfig map[string]any, workspace string, _ *schema.AuthContext) (map[string]any, error) {
	return tfbackend.Config(backendType, backendConfig, workspace)
}

// GenerateConfigFiles generates backend, custom, provider, and version configuration.
func (s *Operations) GenerateConfigFiles(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	defer perf.Track(atmosConfig, "terraform.Operations.GenerateConfigFiles")()

	if err := GenerateBackendConfig(atmosConfig, info, workingDir); err != nil {
		return err
	}
	if err := s.host.GenerateFilesForComponent(atmosConfig, info, workingDir); err != nil {
		return err
	}
	if err := GenerateProviderOverrides(atmosConfig, info, workingDir); err != nil {
		return err
	}
	// Generate required_providers (terraform_override.tf.json) for version pinning (DEV-3124).
	return GenerateRequiredProviders(atmosConfig, info, workingDir)
}

// GenerateBackendConfig delegates backend-file generation using the resolved configuration and working directory.
func GenerateBackendConfig(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	defer perf.Track(atmosConfig, "terraform.GenerateBackendConfig")()

	return tfbackend.GenerateFile(atmosConfig, info, workingDir)
}

// GenerateProviderOverrides writes `providers_override.tf.json` from the component's `providers`
// section, and removes a previously generated file when that section is no longer configured.
func GenerateProviderOverrides(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	defer perf.Track(atmosConfig, "terraform.GenerateProviderOverrides")()

	// Let registered provider-config contributors (e.g. an emulator binding) deep-merge
	// provider fragments UNDER the explicit `providers:` section before generation.
	genCtx := generator.NewGeneratorContext(atmosConfig, info, workingDir)
	if merged, err := generator.ApplyProviderContributors(context.Background(), genCtx); err != nil {
		return err
	} else if merged != nil {
		info.ComponentProvidersSection = merged
	}

	// Generate `providers_override.tf.json` file if the `providers` section is configured.
	if len(info.ComponentProvidersSection) > 0 {
		providerOverrideFileName := filepath.Join(workingDir, providers.DefaultFilenameConst)

		log.Debug("Writing the provider overrides to file.", "file", providerOverrideFileName)

		if !info.DryRun {
			providerOverrides := map[string]any{"provider": tfoutput.ProcessProviderAliases(info.ComponentProvidersSection)}
			err := u.WriteToFileAsJSON(providerOverrideFileName, providerOverrides, planFileMode)
			return err
		}
	} else if !info.DryRun {
		// Remove a stale file, otherwise an earlier stack's providers apply to this one.
		return generator.RemoveGenerated(workingDir, providers.DefaultFilenameConst)
	}
	return nil
}

// GenerateRequiredProviders generates the terraform_override.tf.json file with required_version
// and required_providers blocks from stack configuration (DEV-3124).
func GenerateRequiredProviders(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	defer perf.Track(atmosConfig, "exec.GenerateRequiredProviders")()

	// Skip if no required_version or required_providers configured, removing a stale file
	// so an earlier stack's version pins don't apply to this one.
	if info.RequiredVersion == "" && len(info.RequiredProviders) == 0 {
		if info.DryRun {
			return nil
		}
		return generator.RemoveGenerated(workingDir, required_providers.DefaultFilenameConst)
	}

	requiredProvidersFileName := filepath.Join(workingDir, required_providers.DefaultFilenameConst)

	log.Debug("Writing the required_providers to file.", "file", requiredProvidersFileName)

	if info.DryRun {
		return nil
	}

	// Create generator context.
	genCtx := generator.NewGeneratorContext(atmosConfig, info, workingDir)

	// Generate and write using the generator package.
	return generator.Generate(context.Background(), required_providers.Name, genCtx, generator.NewFileWriter())
}
