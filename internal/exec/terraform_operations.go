package exec

import (
	cobra "github.com/spf13/cobra"

	auth "github.com/cloudposse/atmos/pkg/auth"
	terraformcomponent "github.com/cloudposse/atmos/pkg/component/terraform"
	"github.com/cloudposse/atmos/pkg/perf"
	schema "github.com/cloudposse/atmos/pkg/schema"
)

type terraformOperationServices struct{}

//nolint:revive,gocritic // Match the existing shared stack-processing contract.
func (terraformOperationServices) ProcessStacks(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo schema.ConfigAndStacksInfo, checkStack bool, processTemplates bool, processYamlFunctions bool, skip []string, authManager auth.AuthManager) (schema.ConfigAndStacksInfo, error) {
	defer perf.Track(atmosConfig, "exec.terraformOperationServices.ProcessStacks")()

	return ProcessStacks(atmosConfig, configAndStacksInfo, checkStack, processTemplates, processYamlFunctions, skip, authManager)
}

//nolint:revive // Preserve raw stacks and deferred contexts in the shared contract.
func (terraformOperationServices) FindStacksMap(atmosConfig *schema.AtmosConfiguration, ignoreMissingFiles bool) (map[string]any, map[string]map[string]any, AllStacksDeferredContexts, error) {
	defer perf.Track(atmosConfig, "exec.terraformOperationServices.FindStacksMap")()

	return FindStacksMap(atmosConfig, ignoreMissingFiles)
}

func (terraformOperationServices) ProcessTmpl(atmosConfig *schema.AtmosConfiguration, tmplName string, tmplValue string, tmplData any, ignoreMissingTemplateValues bool) (string, error) {
	defer perf.Track(atmosConfig, "exec.terraformOperationServices.ProcessTmpl")()

	return ProcessTmpl(atmosConfig, tmplName, tmplValue, tmplData, ignoreMissingTemplateValues)
}

//nolint:revive,gocritic // Match the shared template service contract.
func (terraformOperationServices) ProcessTmplWithDatasources(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo, settingsSection schema.Settings, tmplName string, tmplValue string, tmplData any, ignoreMissingTemplateValues bool) (string, error) {
	defer perf.Track(atmosConfig, "exec.terraformOperationServices.ProcessTmplWithDatasources")()

	return ProcessTmplWithDatasources(atmosConfig, configAndStacksInfo, settingsSection, tmplName, tmplValue, tmplData, ignoreMissingTemplateValues)
}

func (terraformOperationServices) ProcessCustomYamlTags(atmosConfig *schema.AtmosConfiguration, input schema.AtmosSectionMapType, currentStack string, skip []string, stackInfo *schema.ConfigAndStacksInfo) (schema.AtmosSectionMapType, error) {
	defer perf.Track(atmosConfig, "exec.terraformOperationServices.ProcessCustomYamlTags")()

	return ProcessCustomYamlTags(atmosConfig, input, currentStack, skip, stackInfo)
}

func (terraformOperationServices) ProcessCommandLineArgs(componentType string, cmd *cobra.Command, args []string, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
	defer perf.Track(nil, "exec.terraformOperationServices.ProcessCommandLineArgs")()

	return ProcessCommandLineArgs(componentType, cmd, args, additionalArgsAndFlags)
}

//nolint:revive // Match the shared deferred-resolution service contract.
func (terraformOperationServices) ResolveDeferredYamlFunctions(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo *schema.ConfigAndStacksInfo, settingsSectionStruct *schema.Settings, componentTemplateContext map[string]any, skip []string, evalSections []string, warnings ...func(DegradationWarning)) error {
	defer perf.Track(atmosConfig, "exec.terraformOperationServices.ResolveDeferredYamlFunctions")()

	return resolveDeferredYamlFunctions(atmosConfig, configAndStacksInfo, settingsSectionStruct, componentTemplateContext, skip, evalSections, warnings...)
}

func (terraformOperationServices) PrepareConfigurationValues(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, skip []string, sections []string) ([]string, func() error) {
	defer perf.Track(nil, "exec.terraformOperationServices.PrepareConfigurationValues")()

	return prepareConfigurationValues(config, info, skip, sections)
}

func (terraformOperationServices) StackLocalsForComponent(rawStackConfigs map[string]map[string]any, stackFile string, componentType string) map[string]any {
	defer perf.Track(nil, "exec.terraformOperationServices.StackLocalsForComponent")()

	return stackLocalsForComponent(rawStackConfigs, stackFile, componentType)
}

func (terraformOperationServices) CloneComponentSectionWithOverrides(raw map[string]any, overrides map[string]any) map[string]any {
	defer perf.Track(nil, "exec.terraformOperationServices.CloneComponentSectionWithOverrides")()

	return cloneComponentSectionWithOverrides(raw, overrides)
}

func (terraformOperationServices) ContainsStarlark(value any) bool {
	defer perf.Track(nil, "exec.terraformOperationServices.ContainsStarlark")()

	return containsStarlark(value)
}

func (terraformOperationServices) GenerateFilesForComponent(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, workingDir string) error {
	defer perf.Track(nil, "exec.terraformOperationServices.GenerateFilesForComponent")()

	return GenerateFilesForComponent(config, info, workingDir)
}

//nolint:gocritic // Preserve the existing value-based naming contract.
func (terraformOperationServices) BuildTerraformWorkspace(atmosConfig *schema.AtmosConfiguration, configAndStacksInfo schema.ConfigAndStacksInfo) (string, error) {
	defer perf.Track(atmosConfig, "exec.terraformOperationServices.BuildTerraformWorkspace")()

	return BuildTerraformWorkspace(atmosConfig, configAndStacksInfo)
}

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

// checkBackendTypeAfterProcessing delegates to the Terraform component implementation.
func checkBackendTypeAfterProcessing(backendType string) error {
	return terraformcomponent.CheckBackendTypeAfterProcessing(backendType)
}

// comparePlansAndGenerateDiff delegates to the Terraform component implementation.
func comparePlansAndGenerateDiff(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, componentPath string, origPlanFile string, newPlanFile string) error {
	return terraformOperations.ComparePlansAndGenerateDiff(atmosConfig, info, componentPath, origPlanFile, newPlanFile)
}

// copyPlanFileIfNeeded delegates to the Terraform component implementation.
func copyPlanFileIfNeeded(planFile string, componentPath string) (string, func(), error) {
	return terraformcomponent.CopyPlanFileIfNeeded(planFile, componentPath)
}

// ensureTerraformComponentExists delegates to the Terraform component implementation.
func ensureTerraformComponentExists(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) error {
	return terraformcomponent.EnsureTerraformComponentExists(atmosConfig, info)
}

// extractBackendConfig delegates to the Terraform component implementation.
func extractBackendConfig(componentSection map[string]any) backendConfig {
	return terraformcomponent.ExtractBackendConfig(componentSection)
}

// extractJSONFromOutput delegates to the Terraform component implementation.
func extractJSONFromOutput(output string) (string, error) {
	return terraformcomponent.ExtractJSONFromOutput(output)
}

// filterPlanDiffFlags delegates to the Terraform component implementation.
func filterPlanDiffFlags(arg1 []string) []string { return terraformcomponent.FilterPlanDiffFlags(arg1) }

// parsePlanDiffFlags delegates to the Terraform component implementation.
func parsePlanDiffFlags(arg1 []string) (string, string, error) {
	return terraformcomponent.ParsePlanDiffFlags(arg1)
}

// runTerraformShowWithExecutor delegates to the Terraform component implementation.
func runTerraformShowWithExecutor(info *schema.ConfigAndStacksInfo, planFile string, executor terraformPlanDiffExecutor) (string, error) {
	return terraformOperations.RunTerraformShowWithExecutor(info, planFile, executor)
}

// validateComponent delegates to the Terraform component implementation.
func validateComponent(component string) error {
	return terraformcomponent.ValidateComponent(component)
}

// validateOriginalPlanFile delegates to the Terraform component implementation.
func validateOriginalPlanFile(origPlanFile string, componentPath string) (string, error) {
	return terraformcomponent.ValidateOriginalPlanFile(origPlanFile, componentPath)
}

// validatePlanfileFormat delegates to the Terraform component implementation.
func validatePlanfileFormat(format *string) error {
	return terraformcomponent.ValidatePlanfileFormat(format)
}

// varfileVarsToWrite delegates to the Terraform component implementation.
func varfileVarsToWrite(info *schema.ConfigAndStacksInfo, withSecrets bool, varFilePath string) map[string]any {
	return terraformcomponent.VarfileVarsToWrite(info, withSecrets, varFilePath)
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
