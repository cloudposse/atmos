package exec

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	cobra "github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	auth "github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	terraformcomponent "github.com/cloudposse/atmos/pkg/component/terraform"
	"github.com/cloudposse/atmos/pkg/dependencies"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/perf"
	schema "github.com/cloudposse/atmos/pkg/schema"
)

// The host wires shared Atmos services into the Terraform component and registers it.
// Terraform owns its lifecycle; this adapter preserves existing host entry points.
func init() {
	terraformExecutor = NewTerraformExecutor()

	stateGetter = terraformExecutor.StateGetter()
	outputGetter = terraformExecutor.OutputGetter()

	if err := component.Register(terraformcomponent.New(terraformExecutor)); err != nil {
		panic(err)
	}
}

// NewTerraformExecutor binds a new Terraform runtime to the shared Atmos host services.
// Each executor owns its service bindings; creating one does not change the registry.
func NewTerraformExecutor() *terraformcomponent.Executor {
	defer perf.Track(nil, "exec.NewTerraformExecutor")()
	return terraformcomponent.NewExecutor(terraformcomponent.Services{
		OperationServices:                             terraformOperationServices{},
		ExecuteDescribeComponent:                      ExecuteDescribeComponent,
		ExecuteDescribeStacksWithAuthDisabled:         ExecuteDescribeStacksWithAuthDisabled,
		ExecuteDescribeStacksWithAuthDisabledAndMocks: ExecuteDescribeStacksWithAuthDisabledAndMocks,
		ExecuteDescribeStacksWithMocks:                ExecuteDescribeStacksWithMocks,
		ExecuteShellCommand:                           ExecuteShellCommand,
		ExecuteTerraform:                              ExecuteTerraform,
		GenerateFilesForComponent:                     GenerateFilesForComponent,
		GetAffectedComponents: func(args *component.DescribeAffectedCmdArgs) ([]schema.Affected, error) {
			return GetAffectedComponents((*DescribeAffectedCmdArgs)(args))
		},
		GetEagerEvaluationSetting: GetEagerEvaluationSetting,
		NewDependencyParserWithDelimiter: func(builder *dependency.GraphBuilder, nodeMap, targetStates map[string]string, leftDelim string) terraformcomponent.DependencyParser {
			return NewDependencyParserWithDelimiter(builder, nodeMap, targetStates, leftDelim)
		},
		ProcessStacks:                    ProcessStacks,
		ValidateComponent:                ValidateComponent,
		AttachComponentReporting:         attachComponentReporting,
		CreateAndAuthenticateAuthManager: createAndAuthenticateAuthManager,
		DefaultAuthManagerCreator: func(identity string, config *schema.AuthConfig, selectValue string, atmosConfig *schema.AtmosConfiguration, context auth.ReExecContext) (auth.AuthManager, error) {
			return defaultAuthManagerCreator(identity, config, selectValue, atmosConfig, context)
		},
		DefaultMergedAuthConfigGetter: func(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*schema.AuthConfig, error) {
			return defaultMergedAuthConfigGetter(config, info)
		},
		DeferredTargetAuth:         deferredTargetAuth,
		DeferredTargetAuthAndCache: deferredTargetAuthAndCache,
		EnvVarFromList:             envVarFromList,
		ExecTerraformShellCommand:  execTerraformShellCommand,
		GetMergedAuthConfig:        getMergedAuthConfig,
		HasYqDefault:               hasYqDefault,
		IsComponentEnabled:         isComponentEnabled,
		MapCIExitCode:              mapCIExitCode,
		ResolveIdentityConfigError: resolveIdentityConfigError,
		ResolvedTargetAuthContext:  resolvedTargetAuthContext,
		ShouldUploadStatus:         shouldUploadStatus,
		StoreAutoDetectedIdentity:  storeAutoDetectedIdentity,
		UploadStatus:               uploadStatus,
	})
}

// ExecuteTerraform is a compatibility entry point for existing CLI, scheduler,
// plan-diff, and migration callers. All of them now select the registered provider.
//
//nolint:gocritic // Preserve the public value-based API while routing existing callers through the registry.
func ExecuteTerraform(info schema.ConfigAndStacksInfo, opts ...ShellCommandOption) error {
	defer perf.Track(nil, "exec.DispatchTerraform")()

	provider, ok := component.GetProvider("terraform")
	if !ok {
		return fmt.Errorf("%w: terraform", errUtils.ErrComponentProviderNotFound)
	}
	return provider.Execute(&component.ExecutionContext{
		Context: shellCommandContext(opts...), Options: opts,
		ComponentType: "terraform", Component: info.ComponentFromArg,
		Stack: info.Stack, Command: info.Command, SubCommand: info.SubCommand,
		ComponentConfig: info.ComponentSection, ConfigAndStacksInfo: info,
		Args: info.AdditionalArgsAndFlags,
	})
}

var terraformExecutor *terraformcomponent.Executor

var defaultMergedAuthConfigGetter = getMergedAuthConfig

var stateGetter terraformcomponent.TerraformStateGetter

var outputGetter terraformcomponent.TerraformOutputGetter

type terraformOperationServices struct{}

// ProcessStacks resolves the selected component through the host stack-processing pipeline.
//
//nolint:revive,gocritic // Match the existing shared stack-processing contract.
func (terraformOperationServices) ProcessStacks(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo schema.ConfigAndStacksInfo, checkStack bool, processTemplates bool, processYamlFunctions bool, skip []string, authManager auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
	defer perf.Track(atmosConfig, "exec.terraformOperationServices.ProcessStacks")()

	return ProcessStacks(atmosConfig, configAndStacksInfo, checkStack, processTemplates, processYamlFunctions, skip, authManager)
}

// FindStacksMap returns processed stacks, raw manifests, and their deferred evaluation contexts.
//
//nolint:revive // Preserve raw stacks and deferred contexts in the shared contract.
func (terraformOperationServices) FindStacksMap(atmosConfig *schema.AtmosConfiguration, ignoreMissingFiles bool) (map[string]any, map[string]map[string]any, AllStacksDeferredContexts, error) {
	defer perf.Track(atmosConfig, "exec.terraformOperationServices.FindStacksMap")()

	return FindStacksMap(atmosConfig, ignoreMissingFiles)
}

// ProcessTmpl renders a template with the host template engine and missing-value policy.
func (terraformOperationServices) ProcessTmpl(atmosConfig *schema.AtmosConfiguration, tmplName string, tmplValue string, tmplData any, ignoreMissingTemplateValues bool) (string, error) {
	defer perf.Track(atmosConfig, "exec.terraformOperationServices.ProcessTmpl")()

	return ProcessTmpl(atmosConfig, tmplName, tmplValue, tmplData, ignoreMissingTemplateValues)
}

// ProcessTmplWithDatasources renders a template with component-scoped data sources and settings.
//
//nolint:revive,gocritic // Match the shared template service contract.
func (terraformOperationServices) ProcessTmplWithDatasources(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo, settingsSection schema.Settings, tmplName string, tmplValue string, tmplData any, ignoreMissingTemplateValues bool) (string, error) {
	defer perf.Track(atmosConfig, "exec.terraformOperationServices.ProcessTmplWithDatasources")()

	return ProcessTmplWithDatasources(atmosConfig, configAndStacksInfo, settingsSection, tmplName, tmplValue, tmplData, ignoreMissingTemplateValues)
}

// ProcessCustomYamlTags evaluates Atmos YAML functions using the selected stack and component context.
func (terraformOperationServices) ProcessCustomYamlTags(atmosConfig *schema.AtmosConfiguration, input schema.AtmosSectionMapType, currentStack string, skip []string, stackInfo *schema.ConfigAndStacksInfo) (schema.AtmosSectionMapType, error) {
	defer perf.Track(atmosConfig, "exec.terraformOperationServices.ProcessCustomYamlTags")()

	return ProcessCustomYamlTags(atmosConfig, input, currentStack, skip, stackInfo)
}

// ProcessCommandLineArgs resolves command flags and passthrough arguments into component execution metadata.
func (terraformOperationServices) ProcessCommandLineArgs(componentType string, cmd *cobra.Command, args []string, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
	defer perf.Track(nil, "exec.terraformOperationServices.ProcessCommandLineArgs")()

	return ProcessCommandLineArgs(componentType, cmd, args, additionalArgsAndFlags)
}

// ResolveDeferredYamlFunctions evaluates deferred values in the requested component sections.
//
//nolint:revive // Match the shared deferred-resolution service contract.
func (terraformOperationServices) ResolveDeferredYamlFunctions(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo, settingsSectionStruct *schema.Settings, componentTemplateContext map[string]any, skip []string, evalSections []string, warnings ...func(DegradationWarning)) error {
	defer perf.Track(atmosConfig, "exec.terraformOperationServices.ResolveDeferredYamlFunctions")()

	return resolveDeferredYamlFunctions(atmosConfig, configAndStacksInfo, settingsSectionStruct, componentTemplateContext, skip, evalSections, warnings...)
}

// PrepareConfigurationValues defers Starlark evaluation and returns the callback that completes resolution.
func (terraformOperationServices) PrepareConfigurationValues(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, skip []string, sections []string) ([]string, func() error) {
	defer perf.Track(nil, "exec.terraformOperationServices.PrepareConfigurationValues")()

	return prepareConfigurationValues(config, info, skip, sections)
}

// StackLocalsForComponent retrieves manifest locals with component-type locals applied over root locals.
func (terraformOperationServices) StackLocalsForComponent(rawStackConfigs map[string]map[string]any, stackFile string, componentType string) map[string]any {
	defer perf.Track(nil, "exec.terraformOperationServices.StackLocalsForComponent")()

	return stackLocalsForComponent(rawStackConfigs, stackFile, componentType)
}

// CloneComponentSectionWithOverrides copies component configuration before applying resolved overrides.
func (terraformOperationServices) CloneComponentSectionWithOverrides(raw map[string]any, overrides map[string]any) map[string]any {
	defer perf.Track(nil, "exec.terraformOperationServices.CloneComponentSectionWithOverrides")()

	return cloneComponentSectionWithOverrides(raw, overrides)
}

// ContainsStarlark reports whether a configuration value contains Starlark expressions.
func (terraformOperationServices) ContainsStarlark(value any) bool {
	defer perf.Track(nil, "exec.terraformOperationServices.ContainsStarlark")()

	return containsStarlark(value)
}

// GenerateFilesForComponent invokes automatic generation for the prepared component working directory.
func (terraformOperationServices) GenerateFilesForComponent(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	defer perf.Track(nil, "exec.terraformOperationServices.GenerateFilesForComponent")()

	return GenerateFilesForComponent(config, info, workingDir)
}

// BuildTerraformWorkspace resolves the workspace from component metadata and Atmos naming conventions.
//
//nolint:gocritic // Preserve the existing value-based naming contract.
func (terraformOperationServices) BuildTerraformWorkspace(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo schema.ConfigAndStacksInfo) (string, error) {
	defer perf.Track(atmosConfig, "exec.terraformOperationServices.BuildTerraformWorkspace")()

	return BuildTerraformWorkspace(atmosConfig, configAndStacksInfo)
}

// ExecuteTerraform routes operation-service executions through the registered Terraform provider.
//
//nolint:gocritic // Preserve the value-based registry dispatch API.
func (terraformOperationServices) ExecuteTerraform(info schema.ConfigAndStacksInfo, opts ...ShellCommandOption) error {
	defer perf.Track(nil, "exec.terraformOperationServices.ExecuteTerraform")()

	return ExecuteTerraform(info, opts...)
}

var terraformOperations = terraformcomponent.NewOperations(terraformOperationServices{})

// ExecuteGenerateBackend delegates to the Terraform component implementation.
func ExecuteGenerateBackend(opts *GenerateBackendOptions, atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "exec.ExecuteGenerateBackend")()

	return terraformOperations.ExecuteGenerateBackend(opts, atmosConfig)
}

// ExecuteGeneratePlanfile delegates to the Terraform component implementation.
func ExecuteGeneratePlanfile(opts *PlanfileOptions, atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "exec.ExecuteGeneratePlanfile")()

	return terraformOperations.ExecuteGeneratePlanfile(opts, atmosConfig)
}

// ExecuteGenerateVarfile delegates to the Terraform component implementation.
func ExecuteGenerateVarfile(opts *VarfileOptions, atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "exec.ExecuteGenerateVarfile")()

	return terraformOperations.ExecuteGenerateVarfile(opts, atmosConfig)
}

// ExecuteTerraformGenerateBackendCmd delegates to the Terraform component implementation.
func ExecuteTerraformGenerateBackendCmd(cmd interface{}, args []string) error {
	defer perf.Track(nil, "exec.ExecuteTerraformGenerateBackendCmd")()

	return terraformcomponent.ExecuteTerraformGenerateBackendCmd(cmd, args)
}

// ExecuteTerraformGenerateBackends delegates to the Terraform component implementation.
func ExecuteTerraformGenerateBackends(atmosConfig *schema.AtmosConfiguration, fileTemplate string, format string, stacks []string, components []string) error {
	defer perf.Track(atmosConfig, "exec.ExecuteTerraformGenerateBackends")()

	return terraformOperations.ExecuteTerraformGenerateBackends(atmosConfig, fileTemplate, format, stacks, components)
}

// ExecuteTerraformGeneratePlanfile delegates to the Terraform component implementation.
func ExecuteTerraformGeneratePlanfile(options *PlanfileOptions, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "exec.ExecuteTerraformGeneratePlanfile")()

	return terraformOperations.ExecuteTerraformGeneratePlanfile(options, info)
}

// ExecuteTerraformGeneratePlanfileOld delegates to the Terraform component implementation.
func ExecuteTerraformGeneratePlanfileOld(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "exec.ExecuteTerraformGeneratePlanfileOld")()

	return terraformOperations.ExecuteTerraformGeneratePlanfileOld(cmd, args)
}

// ExecuteTerraformGenerateRequiredProvidersCmd delegates to the Terraform component implementation.
func ExecuteTerraformGenerateRequiredProvidersCmd(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "exec.ExecuteTerraformGenerateRequiredProvidersCmd")()

	return terraformOperations.ExecuteTerraformGenerateRequiredProvidersCmd(cmd, args)
}

// ExecuteTerraformGenerateVarfileCmd delegates to the Terraform component implementation.
func ExecuteTerraformGenerateVarfileCmd(cmd interface{}, args []string) error {
	defer perf.Track(nil, "exec.ExecuteTerraformGenerateVarfileCmd")()

	return terraformcomponent.ExecuteTerraformGenerateVarfileCmd(cmd, args)
}

// ExecuteTerraformGenerateVarfiles delegates to the Terraform component implementation.
func ExecuteTerraformGenerateVarfiles(atmosConfig *schema.AtmosConfiguration, fileTemplate string, format string, stacks []string, components []string) error {
	defer perf.Track(atmosConfig, "exec.ExecuteTerraformGenerateVarfiles")()

	return terraformOperations.ExecuteTerraformGenerateVarfiles(atmosConfig, fileTemplate, format, stacks, components)
}

// TerraformPlanDiff delegates to the Terraform component implementation.
func TerraformPlanDiff(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(atmosConfig, "exec.TerraformPlanDiff")()

	return terraformOperations.TerraformPlanDiff(atmosConfig, info)
}

// VerifyPlanfile delegates to the Terraform component implementation.
func VerifyPlanfile(info *schema.ConfigAndStacksInfo, storedPlanFile string, mode schema.PlanfileVerifyMode) error {
	defer perf.Track(nil, "exec.VerifyPlanfile")()

	return terraformOperations.VerifyPlanfile(info, storedPlanFile, mode)
}

type (
	backendConfig = terraformcomponent.BackendConfig

	terraformPlanDiffExecutor     = terraformcomponent.TerraformPlanDiffExecutor
	terraformPlanDiffExecutorFunc = terraformcomponent.TerraformPlanDiffExecutorFunc
	PlanFileOptions               = terraformcomponent.PlanFileOptions
	PlanfileOptions               = terraformcomponent.PlanfileOptions
)

var (
	ErrInvalidFormat                      = terraformcomponent.ErrInvalidFormat
	ErrCreatingTempDirectory              = terraformcomponent.ErrCreatingTempDirectory
	ErrCreatingIntermediateSubdirectories = terraformcomponent.ErrCreatingIntermediateSubdirectories
	ErrGettingJsonForPlanfile             = terraformcomponent.ErrGettingJsonForPlanfile
	ErrConvertingJsonToGoType             = terraformcomponent.ErrConvertingJsonToGoType
	ErrNoComponent                        = terraformcomponent.ErrNoComponent
	ErrNoJSONOutput                       = terraformcomponent.ErrNoJSONOutput
)

// IsOpenTofu is the compatibility entry point for Terraform tool detection.
func IsOpenTofu(config *schema.AtmosConfiguration, tools *dependencies.ToolchainEnvironment) bool {
	defer perf.Track(nil, "exec.IsOpenTofu")()

	return terraformcomponent.IsOpenTofu(config, tools)
}

// GetTerraformEnvCliArgs parses Terraform's CLI environment arguments.
func GetTerraformEnvCliArgs() []string {
	defer perf.Track(nil, "exec.GetTerraformEnvCliArgs")()

	return terraformcomponent.GetTerraformEnvCliArgs()
}

// GetTerraformEnvCliVars resolves variable assignments from Terraform's CLI environment arguments.
func GetTerraformEnvCliVars() (map[string]any, error) {
	defer perf.Track(nil, "exec.GetTerraformEnvCliVars")()

	return terraformcomponent.GetTerraformEnvCliVars()
}

// SetLastAuthContext records credentials for post-execution hooks.
func SetLastAuthContext(ctx *schema.AuthContext, manager any) {
	defer perf.Track(nil, "exec.SetLastAuthContext")()

	terraformcomponent.SetLastAuthContext(ctx, manager)
}

// GetLastAuthContext retrieves credentials recorded for post-execution hooks.
func GetLastAuthContext() (*schema.AuthContext, any) {
	defer perf.Track(nil, "exec.GetLastAuthContext")()

	return terraformcomponent.GetLastAuthContext()
}

// ClearLastAuthContext clears post-execution credentials.
func ClearLastAuthContext() {
	defer perf.Track(nil, "exec.ClearLastAuthContext")()

	terraformcomponent.ClearLastAuthContext()
}

// TerraformDeclaredVarType returns a variable's declared HCL type.
func TerraformDeclaredVarType(section map[string]any, name string) (string, bool) {
	defer perf.Track(nil, "exec.TerraformDeclaredVarType")()

	return terraformcomponent.TerraformDeclaredVarType(section, name)
}

// GetComponentRemoteStateBackendStaticType returns configured static state outputs.
func GetComponentRemoteStateBackendStaticType(sections *map[string]any) map[string]any {
	defer perf.Track(nil, "exec.GetComponentRemoteStateBackendStaticType")()

	return terraformcomponent.GetComponentRemoteStateBackendStaticType(sections)
}

// ExecuteShellCommandWithRetry delegates to the Terraform component implementation.
func ExecuteShellCommandWithRetry(
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	phase string,
	invoke func(opts ...ShellCommandOption) error,
	baseOpts ...ShellCommandOption,
) error {
	defer perf.Track(nil, "exec.ExecuteShellCommandWithRetry")()

	return terraformcomponent.ExecuteShellCommandWithRetry(atmosConfig, info, phase, invoke, baseOpts...)
}

type AuthManagerQueryFactory = terraformcomponent.AuthManagerQueryFactory

// ExecuteTerraformQuery delegates to the Terraform component implementation.
func ExecuteTerraformQuery(info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "exec.ExecuteTerraformQuery")()

	return terraformExecutor.ExecuteTerraformQuery(info)
}

// ExecuteTerraformQueryWithContext delegates to the Terraform component implementation.
func ExecuteTerraformQueryWithContext(ctx context.Context, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "exec.ExecuteTerraformQueryWithContext")()

	return terraformExecutor.ExecuteTerraformQueryWithContext(ctx, info)
}

// ExecuteTerraformShell delegates to the Terraform component implementation.
func ExecuteTerraformShell(opts *ShellOptions, atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(nil, "exec.ExecuteTerraformShell")()

	return terraformExecutor.ExecuteTerraformShell(opts, atmosConfig)
}

// FilterTerraformAffected delegates to the Terraform component implementation.
func FilterTerraformAffected(affectedList []schema.Affected) []schema.Affected {
	defer perf.Track(nil, "exec.FilterTerraformAffected")()

	return terraformcomponent.FilterTerraformAffected(affectedList)
}

// ExecuteTerraformAffected delegates to the Terraform component implementation.
func ExecuteTerraformAffected(args *DescribeAffectedCmdArgs, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "exec.ExecuteTerraformAffected")()

	return terraformExecutor.ExecuteTerraformAffected((*component.DescribeAffectedCmdArgs)(args), info)
}

// ExecuteTerraformAffectedWithContext delegates to the Terraform component implementation.
func ExecuteTerraformAffectedWithContext(ctx context.Context, args *DescribeAffectedCmdArgs, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "exec.ExecuteTerraformAffectedWithContext")()

	return terraformExecutor.ExecuteTerraformAffectedWithContext(ctx, (*component.DescribeAffectedCmdArgs)(args), info)
}

// HookStoreDefaultIdentity delegates to the Terraform component implementation.
func HookStoreDefaultIdentity(authManager auth.AuthManager, info *schema.ConfigAndStacksInfo) string {
	defer perf.Track(nil, "exec.HookStoreDefaultIdentity")()

	return terraformExecutor.HookStoreDefaultIdentity(authManager, info)
}

// SetupTerraformAuthForCLI delegates to the Terraform component implementation.
func SetupTerraformAuthForCLI(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (any, error) {
	defer perf.Track(nil, "exec.SetupTerraformAuthForCLI")()

	return terraformExecutor.SetupTerraformAuthForCLI(atmosConfig, info)
}

// SetupComponentAuthForCLI delegates to the Terraform component implementation.
func SetupComponentAuthForCLI(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
	defer perf.Track(nil, "exec.SetupComponentAuthForCLI")()

	return terraformExecutor.SetupComponentAuthForCLI(atmosConfig, info)
}

// ResetStateCache delegates to the Terraform component implementation.
func ResetStateCache() {
	defer perf.Track(nil, "exec.ResetStateCache")()
	terraformcomponent.ResetStateCache()
}

// GetTerraformState delegates to the Terraform component implementation.
//
//nolint:revive // Preserve the state lookup compatibility signature.
func GetTerraformState(
	atmosConfig *schema.AtmosConfiguration,
	yamlFunc string,
	stack string,
	component string,
	output string,
	skipCache bool,
	authContext *schema.AuthContext,
	authManager any,
	options ...TerraformLookupOptions,
) (any, error) {
	defer perf.Track(nil, "exec.GetTerraformState")()

	return terraformExecutor.GetTerraformState(atmosConfig, yamlFunc, stack, component, output, skipCache, authContext, authManager, options...)
}

type TerraformLookupOptions = terraformcomponent.TerraformLookupOptions

type TerraformOutputGetter = terraformcomponent.TerraformOutputGetter

// GetTerraformOutput delegates to the Terraform component implementation.
//
//nolint:revive // Preserve the output lookup compatibility signature.
func GetTerraformOutput(
	atmosConfig *schema.AtmosConfiguration,
	stack string,
	component string,
	output string,
	skipCache bool,
	authContext *schema.AuthContext,
	authManager any,
) (any, bool, error) {
	defer perf.Track(nil, "exec.GetTerraformOutput")()

	return terraformcomponent.GetTerraformOutput(atmosConfig, stack, component, output, skipCache, authContext, authManager)
}

// GetAllTerraformOutputs delegates to the Terraform component implementation.
//
//nolint:revive // Preserve the output lookup compatibility signature.
func GetAllTerraformOutputs(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	skipInit bool,
	authContext *schema.AuthContext,
	authManager any,
) (map[string]any, error) {
	defer perf.Track(nil, "exec.GetAllTerraformOutputs")()

	return terraformcomponent.GetAllTerraformOutputs(atmosConfig, component, stack, skipInit, authContext, authManager)
}

// GetStaticRemoteStateOutput delegates to the Terraform component implementation.
func GetStaticRemoteStateOutput(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	remoteStateSection map[string]any,
	output string,
) (any, bool, error) {
	defer perf.Track(nil, "exec.GetStaticRemoteStateOutput")()

	return terraformcomponent.GetStaticRemoteStateOutput(atmosConfig, component, stack, remoteStateSection, output)
}

type TerraformStateGetter = terraformcomponent.TerraformStateGetter

// ExecuteTerraformAll delegates to the Terraform component implementation.
func ExecuteTerraformAll(info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "exec.ExecuteTerraformAll")()

	return terraformExecutor.ExecuteTerraformAll(info)
}

// ExecuteTerraformAllWithContext delegates to the Terraform component implementation.
func ExecuteTerraformAllWithContext(ctx context.Context, info *schema.ConfigAndStacksInfo) error {
	defer perf.Track(nil, "exec.ExecuteTerraformAllWithContext")()

	return terraformExecutor.ExecuteTerraformAllWithContext(ctx, info)
}

// ResetNestedAuthManagerCache delegates to the Terraform component implementation.
func ResetNestedAuthManagerCache() {
	defer perf.Track(nil, "exec.ResetNestedAuthManagerCache")()
	terraformcomponent.ResetNestedAuthManagerCache()
}

type ComponentStack = terraformcomponent.ComponentStack

// ListTerraformComponentTargets delegates to the Terraform component implementation.
func ListTerraformComponentTargets(atmosConfig *schema.AtmosConfiguration, filterStack string, components []string, query string) ([]ComponentStack, error) {
	defer perf.Track(nil, "exec.ListTerraformComponentTargets")()

	return terraformExecutor.ListTerraformComponentTargets(atmosConfig, filterStack, components, query)
}

// allDiagnosticsAreModuleSourceInterpolation identifies module-source interpolation diagnostics that Atmos can tolerate.
func allDiagnosticsAreModuleSourceInterpolation(diagnostics tfconfig.Diagnostics) bool {
	return terraformcomponent.AllDiagnosticsAreModuleSourceInterpolation(diagnostics)
}
