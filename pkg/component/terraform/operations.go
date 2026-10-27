package terraform

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_operation_services_test.go -package=$GOPACKAGE

import (
	cobra "github.com/spf13/cobra"

	auth "github.com/cloudposse/atmos/pkg/auth"
	component "github.com/cloudposse/atmos/pkg/component"
	degradation "github.com/cloudposse/atmos/pkg/degradation"
	perf "github.com/cloudposse/atmos/pkg/perf"
	schema "github.com/cloudposse/atmos/pkg/schema"
)

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
