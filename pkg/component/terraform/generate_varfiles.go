package terraform

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/terraform/tfvars"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const (
	varfileFormatYAML = "yaml"
	varfileFormatJSON = "json"
	varfileFormatHCL  = "hcl"
)

// ExecuteTerraformGenerateVarfiles generates varfiles for all terraform components in all stacks.
func (s *Operations) ExecuteTerraformGenerateVarfiles(
	atmosConfig *schema.AtmosConfiguration,
	fileTemplate string,
	format string,
	stacks []string,
	components []string,
) error {
	defer perf.Track(atmosConfig, "exec.ExecuteTerraformGenerateVarfiles")()

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
	}

	for stackFileName, stackSection := range stacksMap {
		for componentName, compSection := range terraformComponentsInStack(stackSection) {
			componentSection, ok := compSection.(map[string]any)
			// Check if `components` filter is provided.
			if !ok || !componentSelected(components, componentName) {
				continue
			}
			if err := s.generateComponentVarfile(run, stackFileName, componentName, componentSection); err != nil {
				return err
			}
		}
	}

	return nil
}

// newVarfileGeneratorInfo builds the config and stacks info used to render a component for varfile generation.
func (s *Operations) newVarfileGeneratorInfo(
	componentName string,
	componentSection map[string]any,
	metadataSection map[string]any,
	varsSection map[string]any,
) schema.ConfigAndStacksInfo {
	settingsSection := sectionOrEmpty(componentSection, cfg.SettingsSectionName)
	envSection := sectionOrEmpty(componentSection, cfg.EnvSectionName)
	authSection := sectionOrEmpty(componentSection, cfg.AuthSectionName)
	providersSection := sectionOrEmpty(componentSection, cfg.ProvidersSectionName)
	overridesSection := sectionOrEmpty(componentSection, cfg.OverridesSectionName)

	// Component backend and backend type.
	backendSection := sectionOrEmpty(componentSection, cfg.BackendSectionName)
	backendTypeSection, _ := componentSection[cfg.BackendTypeSectionName].(string)

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg:          componentName,
		ComponentType:             cfg.TerraformComponentType,
		ComponentMetadataSection:  metadataSection,
		ComponentVarsSection:      varsSection,
		ComponentSettingsSection:  settingsSection,
		ComponentEnvSection:       envSection,
		ComponentAuthSection:      authSection,
		ComponentProvidersSection: providersSection,
		ComponentOverridesSection: overridesSection,
		ComponentBackendSection:   backendSection,
		ComponentBackendType:      backendTypeSection,
		// Snapshot the complete merged component section (hooks, generate,
		// required_providers, etc. included), not just the sections this generator
		// directly consumes — see cloneComponentSectionWithOverrides.
		ComponentSection: s.host.CloneComponentSectionWithOverrides(componentSection, map[string]any{
			cfg.VarsSectionName:        varsSection,
			cfg.MetadataSectionName:    metadataSection,
			cfg.SettingsSectionName:    settingsSection,
			cfg.EnvSectionName:         envSection,
			cfg.AuthSectionName:        authSection,
			cfg.ProvidersSectionName:   providersSection,
			cfg.OverridesSectionName:   overridesSection,
			cfg.BackendSectionName:     backendSection,
			cfg.BackendTypeSectionName: backendTypeSection,
		}),
	}

	if comp, ok := info.ComponentSection[cfg.ComponentSectionName].(string); !ok || comp == "" {
		info.ComponentSection[cfg.ComponentSectionName] = componentName
	}

	return info
}

// provisionalWorkspace computes the workspace before computed vars are evaluated.
// Batch generation starts from a physical manifest before evaluating computed vars. Keep its
// provisional workspace physical as well; BuildTerraformWorkspace runs again after finishValues
// with the resolved context.
func (s *Operations) provisionalWorkspace(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, stackFileName string) (string, error) {
	workspace, err := s.host.BuildTerraformWorkspace(atmosConfig, *info)
	if err != nil && !errors.Is(err, errUtils.ErrStarlarkStackIdentity) {
		return "", err
	}
	if errors.Is(err, errUtils.ErrStarlarkStackIdentity) || s.host.ContainsStarlark(workspace) {
		workspace = strings.ReplaceAll(stackFileName, "/", "-")
	}
	return workspace, nil
}

// generateComponentVarfile renders one component and writes its varfile when the stack filter selects it.
func (s *Operations) generateComponentVarfile(run *generateRun, stackFileName, componentName string, componentSection map[string]any) error {
	atmosConfig := run.atmosConfig

	// Component vars
	varsSection, ok := componentSection[cfg.VarsSectionName].(map[string]any)
	if !ok {
		return nil
	}

	// Component metadata. Don't include abstract components.
	metadataSection, _ := componentSection[cfg.MetadataSectionName].(map[string]any)
	if isAbstractComponent(metadataSection) {
		return nil
	}

	info, ref, err := s.prepareVarfileComponent(run, componentRef{stackFileName: stackFileName, componentName: componentName}, componentSection, metadataSection, varsSection)
	if err != nil {
		return err
	}

	err = s.renderGeneratorComponent(atmosConfig, &info, generatorRender{
		componentSection: componentSection,
		settingsSection:  info.ComponentSettingsSection,
		rawStackConfigs:  run.rawStackConfigs,
		stackFileName:    stackFileName,
		stackName:        info.Stack,
		templateName:     "terraform-generate-varfiles",
	})
	if err != nil {
		return err
	}

	workspace, err := s.host.BuildTerraformWorkspace(atmosConfig, info)
	if err != nil {
		return err
	}
	info.ComponentSection["workspace"] = workspace

	if i, ok := info.ComponentSection[cfg.VarsSectionName].(map[string]any); ok {
		varsSection = i
	}

	// Check if `stacks` filter is provided.
	if !stackSelected(run.stacks, stackFileName, info.Stack) {
		return nil
	}

	return writeComponentVarfile(run, &info, varsSection, ref)
}

// prepareVarfileComponent builds the info for a component and resolves its context, stack name, and provisional workspace.
func (s *Operations) prepareVarfileComponent(
	run *generateRun,
	ref componentRef,
	componentSection map[string]any,
	metadataSection map[string]any,
	varsSection map[string]any,
) (schema.ConfigAndStacksInfo, componentRef, error) {
	atmosConfig := run.atmosConfig
	info := s.newVarfileGeneratorInfo(ref.componentName, componentSection, metadataSection, varsSection)

	// Recover this component's deferred-merge contexts (per section: vars, settings,
	// env, auth, providers, etc.) from the FindStacksMap cache so deferred YAML
	// functions (!template, !labels, !tags, !terraform.output, etc.) can be resolved
	// and deep-merged against a concrete override at the same path below, instead of
	// silently losing data the way #2888 described for the main describe/plan path.
	if compDctx, ok := run.deferredContexts[ref.stackFileName][cfg.TerraformComponentType][ref.componentName]; ok {
		info.DeferredMergeContexts = compDctx
	}

	ref.terraformComponent, ref.terraformComponentPath = terraformComponentAndPath(atmosConfig, ref.componentName, componentSection)

	// Context
	info.Context = cfg.GetContextFromVars(varsSection)
	info.Context.Component = strings.ReplaceAll(ref.componentName, "/", "-")
	info.Context.ComponentPath = ref.terraformComponentPath

	stackName, err := s.resolveGeneratorStackName(atmosConfig, "terraform-generate-varfiles-template", ref.stackFileName, &info)
	if err != nil {
		return info, ref, err
	}

	info.Stack = stackName
	setGeneratorIdentity(info.ComponentSection, ref.componentName, stackName, ref.stackFileName)

	// Terraform workspace.
	workspace, err := s.provisionalWorkspace(atmosConfig, &info, ref.stackFileName)
	if err != nil {
		return info, ref, err
	}
	componentSection["workspace"] = workspace
	info.ComponentSection["workspace"] = workspace

	// Atmos component, stack, and stack manifest file
	setGeneratorIdentity(componentSection, ref.componentName, stackName, ref.stackFileName)

	return info, ref, nil
}

// writeComponentVarfile writes the varfile of a rendered component.
func writeComponentVarfile(
	run *generateRun,
	info *schema.ConfigAndStacksInfo,
	varsSection map[string]any,
	ref componentRef,
) error {
	// Replace the tokens in the file template
	// Supported context tokens: {namespace}, {tenant}, {environment}, {region}, {stage}, {base-component}, {component}, {component-path}
	fileName := cfg.ReplaceContextTokens(info.Context, run.fileTemplate)
	fileAbsolutePath, err := filepath.Abs(fileName)
	if err != nil {
		return err
	}

	// Create all the intermediate subdirectories
	if err := u.EnsureDir(fileAbsolutePath); err != nil {
		return err
	}

	// Keep resolved secrets out of generated varfiles so plaintext secrets
	// never hit disk. Batch generation has no env-injection target, so
	// secret-bearing variables are omitted (use the singular
	// `generate varfile --with-secrets` to export them deliberately).
	varsToWrite, secretVars := tfvars.Partition(varsSection, atmosio.ContainsSecret)
	if len(secretVars) > 0 {
		log.Warn("Omitting secret-bearing variables from the generated varfile",
			"file", fileName, "count", len(secretVars))
	}

	// Write the varfile
	if err := writeVarfile(run.format, fileAbsolutePath, varsToWrite); err != nil {
		return err
	}

	log.Debug("varfile", fileName)
	log.Debug("terraform component", ref.terraformComponent)
	log.Debug("atmos component", ref.componentName)
	log.Debug("atmos stack", info.Stack)
	log.Debug("stack config file", ref.stackFileName)
	return nil
}

// writeVarfile writes the vars to the file in the requested format.
func writeVarfile(format, fileAbsolutePath string, vars map[string]any) error {
	switch format {
	case varfileFormatYAML:
		return u.WriteToFileAsYAML(fileAbsolutePath, vars, filePermissions)
	case varfileFormatJSON:
		return u.WriteToFileAsJSON(fileAbsolutePath, vars, filePermissions)
	case varfileFormatHCL:
		return u.WriteToFileAsHcl(fileAbsolutePath, vars, filePermissions)
	default:
		return fmt.Errorf("%w: invalid '--format' argument '%s'. Valid values are 'json' (default), 'yaml' and 'hcl'", errUtils.ErrInvalidFlag, format)
	}
}
