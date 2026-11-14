package terraform

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/go-viper/mapstructure/v2"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/function/literalsource"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	atmosYaml "github.com/cloudposse/atmos/pkg/yaml"
)

// backendConfig holds extracted backend configuration from a component section.
type BackendConfig struct {
	BackendSection map[string]any
	BackendType    string
	Err            error
}

// ExtractBackendConfig extracts and validates backend configuration from a component section.
func ExtractBackendConfig(componentSection map[string]any) BackendConfig {
	defer perf.Track(nil, "terraform.ExtractBackendConfig")()

	backendSection, ok := componentSection[cfg.BackendSectionName].(map[string]any)
	if !ok {
		return BackendConfig{Err: errUtils.ErrBackendSectionMissing}
	}

	backendType, ok := componentSection[cfg.BackendTypeSectionName].(string)
	if !ok {
		return BackendConfig{Err: errUtils.ErrBackendTypeMissing}
	}

	// Check if backend section is empty for backends that require configuration.
	// The "local" backend can work without any configuration, but remote backends
	// like s3, gcs, azurerm, and cloud require at least some configuration.
	if len(backendSection) == 0 && backendType != cfg.BackendTypeLocal {
		return BackendConfig{Err: errUtils.ErrBackendConfigEmpty}
	}

	return BackendConfig{
		BackendSection: backendSection,
		BackendType:    backendType,
	}
}

// CheckBackendTypeAfterProcessing validates that backend_type is not empty after template processing.
func CheckBackendTypeAfterProcessing(backendType string) error {
	defer perf.Track(nil, "terraform.CheckBackendTypeAfterProcessing")()

	if backendType == "" {
		return errUtils.ErrBackendTypeEmptyAfterRender
	}
	return nil
}

// ErrTemplatingDisabledWithTemplates is returned when stack manifests contain Go templates but templating is disabled.
var ErrTemplatingDisabledWithTemplates = errors.New("the stack manifests contain Go templates, but templating is disabled in atmos.yaml in 'templates.settings.enabled'\n" +
	"to enable templating, refer to https://atmos.tools/core-concepts/stacks/templating")

const (
	backendFormatJSON          = "json"
	backendFormatHCL           = "hcl"
	backendFormatBackendConfig = "backend-config"
	abstractComponentType      = "abstract"
)

// generateRun holds the inputs shared by every component processed by a generate command.
type generateRun struct {
	atmosConfig      *schema.AtmosConfiguration
	fileTemplate     string
	format           string
	stacks           []string
	rawStackConfigs  map[string]map[string]any
	deferredContexts component.AllStacksDeferredContexts
	// processed tracks the terraform components whose backend was already written.
	processed map[string]any
}

// componentRef identifies the stack manifest, component, and terraform component being generated.
type componentRef struct {
	stackFileName          string
	componentName          string
	terraformComponent     string
	terraformComponentPath string
}

// generatorRender holds the inputs needed to render a component section for a generate command.
type generatorRender struct {
	componentSection map[string]any
	settingsSection  map[string]any
	rawStackConfigs  map[string]map[string]any
	stackFileName    string
	stackName        string
	templateName     string
}

// terraformComponentsInStack returns the terraform components section of a stack, or nil when absent.
func terraformComponentsInStack(stackSection any) map[string]any {
	stackMap, _ := stackSection.(map[string]any)
	componentsSection, _ := stackMap["components"].(map[string]any)
	terraformSection, _ := componentsSection[cfg.TerraformSectionName].(map[string]any)
	return terraformSection
}

// componentSelected reports whether the `components` filter selects the component.
func componentSelected(components []string, componentName string) bool {
	return len(components) == 0 || slices.Contains(components, componentName)
}

// stackSelected reports whether the `stacks` filter selects the stack.
func stackSelected(stacks []string, stackFileName, stackName string) bool {
	return len(stacks) == 0 ||
		// `stacks` filter can contain the names of the top-level stack config files:
		// atmos terraform generate varfiles --stacks=orgs/cp/tenant1/staging/us-east-2,orgs/cp/tenant2/dev/us-east-2
		slices.Contains(stacks, stackFileName) ||
		// `stacks` filter can also contain the logical stack names (derived from the context vars):
		// atmos terraform generate varfiles --stacks=tenant1-ue2-staging,tenant1-ue2-prod
		slices.Contains(stacks, stackName)
}

// isAbstractComponent reports whether the component metadata marks the component as abstract.
func isAbstractComponent(metadataSection map[string]any) bool {
	componentType, _ := metadataSection["type"].(string)
	return componentType == abstractComponentType
}

// sectionOrEmpty returns the named map section, or an empty map when it is absent or not a map.
func sectionOrEmpty(componentSection map[string]any, name string) map[string]any {
	if section, ok := componentSection[name].(map[string]any); ok {
		return section
	}
	return map[string]any{}
}

// terraformComponentAndPath finds the terraform component and its path.
// If the `component` attribute is present, it's the terraform component.
// Otherwise, the YAML component name is the terraform component.
func terraformComponentAndPath(atmosConfig *schema.AtmosConfiguration, componentName string, componentSection map[string]any) (string, string) {
	terraformComponent := componentName
	if componentAttribute, ok := componentSection[cfg.ComponentSectionName].(string); ok {
		terraformComponent = componentAttribute
	}

	return terraformComponent, filepath.Join(
		atmosConfig.BasePath,
		atmosConfig.Components.Terraform.BasePath,
		terraformComponent,
	)
}

// setGeneratorIdentity records the component, stack, and stack manifest file in the section.
func setGeneratorIdentity(section map[string]any, componentName, stackName, stackFileName string) {
	section["atmos_component"] = componentName
	section["atmos_stack"] = stackName
	section["stack"] = stackName
	section["atmos_stack_file"] = stackFileName
	section["atmos_manifest"] = stackFileName
}

// resolveGeneratorStackName computes the stack name from the name template or the name pattern.
func (s *Operations) resolveGeneratorStackName(
	atmosConfig *schema.AtmosConfiguration,
	templateName string,
	stackFileName string,
	info *schema.ConfigAndStacksInfo,
) (string, error) {
	var stackName string
	var err error
	if atmosConfig.Stacks.NameTemplate != "" {
		stackName, err = s.host.ProcessTmpl(atmosConfig, templateName, atmosConfig.Stacks.NameTemplate, info.ComponentSection, atmosConfig.Templates.Settings.IgnoreMissingTemplateValues)
	} else {
		stackName, err = cfg.GetContextPrefix(stackFileName, info.Context, atmosConfig.Stacks.NamePattern, stackFileName)
	}
	if err != nil {
		return "", err
	}

	// A computed name is not final yet. Keep derived metadata from being
	// interpreted as another Starlark program during value evaluation.
	if s.host.ContainsStarlark(stackName) {
		stackName = stackFileName
	}
	return stackName, nil
}

// withTemplatingDisabledHint joins the templating-disabled error when the manifest contains Go templates.
func withTemplatingDisabledHint(err error, atmosConfig *schema.AtmosConfiguration, componentSectionStr string) error {
	if atmosConfig.Templates.Settings.Enabled {
		return err
	}
	if !strings.Contains(componentSectionStr, "{{") && !strings.Contains(componentSectionStr, "}}") {
		return err
	}
	return errors.Join(err, ErrTemplatingDisabledWithTemplates)
}

// renderGeneratorComponent processes the Go templates and YAML functions of the component section.
// The rendered section, context, and stack name are stored in the provided info.
func (s *Operations) renderGeneratorComponent(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, req generatorRender) error {
	// Process `Go` templates
	componentSectionStr, err := atmosYaml.ConvertToYAMLPreservingDelimiters(req.componentSection, atmosConfig.Templates.Settings.Delimiters)
	if err != nil {
		return err
	}

	var settingsSectionStruct schema.Settings
	if err := mapstructure.Decode(req.settingsSection, &settingsSectionStruct); err != nil {
		return err
	}

	// Snapshot the component context for deferred-YAML-function template rendering
	// below (a deferred value's own {{ }} expression, e.g. a parametrized
	// !terraform.state stack argument, needs the same data source ProcessTmplWithDatasources
	// uses here).
	componentTemplateContext := maps.Clone(info.ComponentSection)

	componentSectionProcessed, err := s.host.ProcessTmplWithDatasources(
		atmosConfig,
		info,
		settingsSectionStruct,
		req.templateName,
		componentSectionStr,
		info.ComponentSection,
		true,
	)
	if err != nil {
		return err
	}

	componentSectionConverted, err := u.UnmarshalYAML[schema.AtmosSectionMapType](componentSectionProcessed)
	if err != nil {
		errUtils.CheckErrorPrintAndExit(withTemplatingDisabledHint(err, atmosConfig, componentSectionStr), "", "")
	}

	info.ComponentSection = componentSectionConverted
	info.StackLocalsSection = s.host.StackLocalsForComponent(req.rawStackConfigs, req.stackFileName, cfg.TerraformComponentType)
	yamlSkip, finishValues := s.host.PrepareConfigurationValues(atmosConfig, info, nil, nil)
	componentSectionFinal, err := s.host.ProcessCustomYamlTags(atmosConfig, componentSectionConverted, req.stackName, yamlSkip, info)
	if err != nil {
		return err
	}
	info.ComponentSection = componentSectionFinal

	// Resolve deferred YAML functions (!template, !labels, !tags, !terraform.output,
	// etc.) and deep-merge their results against any concrete override at the same
	// path. Without this, a section that only survived Stage 2's structural merge as an
	// unresolved function string (or a placeholder) would silently lose the function's
	// contribution here, the same #2888 data-loss bug the main describe/plan path fixes
	// via this same call (see internal/exec/utils.go).
	if err := s.host.ResolveDeferredYamlFunctions(atmosConfig, info, &settingsSectionStruct, componentTemplateContext, nil, nil); err != nil {
		return err
	}
	if err := finishValues(); err != nil {
		return err
	}
	info.ComponentSection = literalsource.RestoreMap(info.ComponentSection)
	return s.refreshTerraformGeneratorContext(atmosConfig, info, req.stackFileName)
}

// ExecuteTerraformGenerateBackends generates backend configs for all terraform components.
func (s *Operations) ExecuteTerraformGenerateBackends(
	atmosConfig *schema.AtmosConfiguration,
	fileTemplate string,
	format string,
	stacks []string,
	components []string,
) error {
	defer perf.Track(atmosConfig, "exec.ExecuteTerraformGenerateBackends")()

	stacksMap, rawStackConfigs, deferredContexts, err := s.host.FindStacksMap(atmosConfig, false)
	if err != nil {
		return err
	}

	run := &generateRun{
		atmosConfig:      atmosConfig,
		fileTemplate:     fileTemplate,
		format:           format,
		stacks:           stacks,
		rawStackConfigs:  rawStackConfigs,
		deferredContexts: deferredContexts,
		processed:        map[string]any{},
	}

	for stackFileName, stackSection := range stacksMap {
		for componentName, compSection := range terraformComponentsInStack(stackSection) {
			componentSection, ok := compSection.(map[string]any)
			// Check if `components` filter is provided.
			if !ok || !componentSelected(components, componentName) {
				continue
			}
			if err := s.generateComponentBackend(run, stackFileName, componentName, componentSection); err != nil {
				return err
			}
		}
	}

	return nil
}

// newBackendGeneratorInfo builds the config and stacks info used to render a component for backend generation.
func (s *Operations) newBackendGeneratorInfo(
	componentName string,
	componentSection map[string]any,
	metadataSection map[string]any,
	backend BackendConfig,
) schema.ConfigAndStacksInfo {
	varsSection := sectionOrEmpty(componentSection, cfg.VarsSectionName)
	settingsSection := sectionOrEmpty(componentSection, cfg.SettingsSectionName)
	envSection := sectionOrEmpty(componentSection, cfg.EnvSectionName)
	providersSection := sectionOrEmpty(componentSection, cfg.ProvidersSectionName)
	authSection := sectionOrEmpty(componentSection, cfg.AuthSectionName)
	hooksSection := sectionOrEmpty(componentSection, cfg.HooksSectionName)
	overridesSection := sectionOrEmpty(componentSection, cfg.OverridesSectionName)

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg:          componentName,
		ComponentType:             cfg.TerraformComponentType,
		ComponentMetadataSection:  metadataSection,
		ComponentVarsSection:      varsSection,
		ComponentSettingsSection:  settingsSection,
		ComponentEnvSection:       envSection,
		ComponentAuthSection:      authSection,
		ComponentProvidersSection: providersSection,
		ComponentHooksSection:     hooksSection,
		ComponentOverridesSection: overridesSection,
		ComponentBackendSection:   backend.BackendSection,
		ComponentBackendType:      backend.BackendType,
		// Snapshot the complete merged component section (auth, required_providers,
		// generate, etc. included), not just the sections this generator directly
		// consumes — see cloneComponentSectionWithOverrides.
		ComponentSection: s.host.CloneComponentSectionWithOverrides(componentSection, map[string]any{
			cfg.VarsSectionName:        varsSection,
			cfg.MetadataSectionName:    metadataSection,
			cfg.SettingsSectionName:    settingsSection,
			cfg.EnvSectionName:         envSection,
			cfg.AuthSectionName:        authSection,
			cfg.ProvidersSectionName:   providersSection,
			cfg.HooksSectionName:       hooksSection,
			cfg.OverridesSectionName:   overridesSection,
			cfg.BackendSectionName:     backend.BackendSection,
			cfg.BackendTypeSectionName: backend.BackendType,
		}),
	}

	if comp, ok := info.ComponentSection[cfg.ComponentSectionName].(string); !ok || comp == "" {
		info.ComponentSection[cfg.ComponentSectionName] = componentName
	}

	return info
}

// warnSkipBackend logs a warning and returns true when the backend generation must be skipped due to the error.
func warnSkipBackend(err error, componentName, stackName string) bool {
	if err == nil {
		return false
	}
	log.Warn("Skipping backend generation: "+err.Error()+". "+
		"Set 'components.terraform.auto_generate_backend_file: false' in atmos.yaml to disable.",
		"component", componentName, "stack", stackName)
	return true
}

// generateComponentBackend renders one component and writes its backend config when the stack filter selects it.
func (s *Operations) generateComponentBackend(run *generateRun, stackFileName, componentName string, componentSection map[string]any) error {
	atmosConfig := run.atmosConfig

	// Component metadata. Don't include abstract components.
	metadataSection, _ := componentSection[cfg.MetadataSectionName].(map[string]any)
	if isAbstractComponent(metadataSection) {
		return nil
	}

	// Extract backend configuration.
	backend := ExtractBackendConfig(componentSection)
	if warnSkipBackend(backend.Err, componentName, stackFileName) {
		return nil
	}

	info := s.newBackendGeneratorInfo(componentName, componentSection, metadataSection, backend)

	// Recover this component's deferred-merge contexts (per section: vars, settings,
	// env, auth, providers, etc.) from the FindStacksMap cache so deferred YAML
	// functions (!template, !labels, !tags, !terraform.output, etc.) can be resolved
	// and deep-merged against a concrete override at the same path below, instead of
	// silently losing data the way #2888 described for the main describe/plan path.
	if compDctx, ok := run.deferredContexts[stackFileName][cfg.TerraformComponentType][componentName]; ok {
		info.DeferredMergeContexts = compDctx
	}

	terraformComponent, terraformComponentPath := terraformComponentAndPath(atmosConfig, componentName, componentSection)
	ref := componentRef{stackFileName, componentName, terraformComponent, terraformComponentPath}

	// Context
	info.Context = cfg.GetContextFromVars(info.ComponentVarsSection)
	info.Context.Component = strings.ReplaceAll(componentName, "/", "-")
	info.Context.ComponentPath = terraformComponentPath

	stackName, err := s.resolveGeneratorStackName(atmosConfig, "terraform-generate-backends-template", stackFileName, &info)
	if err != nil {
		return err
	}

	setGeneratorIdentity(info.ComponentSection, componentName, stackName, stackFileName)

	// Populate stack name for YAML/template processing
	info.Stack = stackName

	err = s.renderGeneratorComponent(atmosConfig, &info, generatorRender{
		componentSection: componentSection,
		settingsSection:  info.ComponentSettingsSection,
		rawStackConfigs:  run.rawStackConfigs,
		stackFileName:    stackFileName,
		stackName:        stackName,
		templateName:     "terraform-generate-backends",
	})
	if err != nil {
		return err
	}

	return s.writeComponentBackend(run, &info, backend, ref)
}

// writeComponentBackend writes the backend config of a rendered component when the stack filter selects it.
func (s *Operations) writeComponentBackend(
	run *generateRun,
	info *schema.ConfigAndStacksInfo,
	backend BackendConfig,
	ref componentRef,
) error {
	backendSection := backend.BackendSection
	if i, ok := info.ComponentSection[cfg.BackendSectionName].(map[string]any); ok {
		backendSection = i
	}

	backendTypeSection := backend.BackendType
	if i, ok := info.ComponentSection[cfg.BackendTypeSectionName].(string); ok {
		backendTypeSection = i
	}

	// Skip if backend_type is empty after template processing.
	if warnSkipBackend(CheckBackendTypeAfterProcessing(backendTypeSection), ref.componentName, info.Stack) {
		return nil
	}

	// Check if `stacks` filter is provided.
	if !stackSelected(run.stacks, ref.stackFileName, info.Stack) {
		return nil
	}

	backendFilePath, skip := resolveBackendFilePath(run, info, ref)
	if skip {
		return nil
	}

	backendFileAbsolutePath, err := filepath.Abs(backendFilePath)
	if err != nil {
		return err
	}

	// Create all the intermediate subdirectories when the file template is used.
	if run.fileTemplate != "" {
		if err := u.EnsureDir(backendFileAbsolutePath); err != nil {
			return err
		}
	}

	// Write the backend config to the file
	log.Debug("Writing backend config for the component to file", "component", ref.terraformComponent, "file", backendFilePath)

	return writeBackendFile(run.format, backendFileAbsolutePath, backendTypeSection, backendSection)
}

// resolveBackendFilePath returns the backend file path.
// The skip result is true when the terraform component backend was already written.
func resolveBackendFilePath(run *generateRun, info *schema.ConfigAndStacksInfo, ref componentRef) (string, bool) {
	// If '--file-template' is specified, replace the tokens in the file template.
	// Supported context tokens: {namespace}, {tenant}, {environment}, {region}, {stage}, {base-component}, {component}, {component-path}
	if run.fileTemplate != "" {
		return cfg.ReplaceContextTokens(info.Context, run.fileTemplate), false
	}

	// If '--file-template' is not specified, don't check if we've already processed the terraform component,
	// and write the backends to the terraform components folders.
	// If the terraform component has been already processed, skip it.
	if u.MapKeyExists(run.processed, ref.terraformComponent) {
		return "", true
	}

	run.processed[ref.terraformComponent] = ref.terraformComponent

	backendFilePath := filepath.Join(
		ref.terraformComponentPath,
		"backend.tf",
	)

	if run.format == backendFormatJSON {
		backendFilePath += ".json"
	}

	return backendFilePath, false
}

// writeBackendFile writes the backend config to the file in the requested format.
func writeBackendFile(format, backendFileAbsolutePath, backendType string, backendSection map[string]any) error {
	switch format {
	case backendFormatJSON:
		componentBackendConfig, err := generateComponentBackendConfig(backendType, backendSection, "", nil)
		if err != nil {
			return err
		}
		return u.WriteToFileAsJSON(backendFileAbsolutePath, componentBackendConfig, filePermissions)
	case backendFormatHCL:
		return u.WriteTerraformBackendConfigToFileAsHcl(backendFileAbsolutePath, backendType, backendSection)
	case backendFormatBackendConfig:
		return u.WriteToFileAsHcl(backendFileAbsolutePath, backendSection, filePermissions)
	default:
		return fmt.Errorf("%w: invalid '--format' argument '%s'. Valid values are 'hcl' (default), 'json' and 'backend-config'", errUtils.ErrInvalidFlag, format)
	}
}
