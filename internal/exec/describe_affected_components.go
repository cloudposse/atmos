//nolint:gocognit,revive,nestif // Complex component processing logic requires nested conditionals
package exec

import (
	"fmt"
	"reflect"

	"github.com/go-viper/mapstructure/v2"

	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
)

// Affected reason constants.
const (
	affectedReasonStackMetadata   = "stack.metadata"
	affectedReasonComponent       = "component"
	affectedReasonComponentModule = "component.module"
	affectedReasonStackVars       = "stack.vars"
	affectedReasonStackEnv        = "stack.env"
	affectedReasonStackSettings   = "stack.settings"
	affectedReasonStackSource     = "stack.source"
	affectedReasonStackProvision  = "stack.provision"
	affectedReasonStackGenerate   = "stack.generate"
	affectedReasonStackPaths      = "stack.paths"
	affectedReasonStackManifests  = "stack.manifests"
	affectedReasonStackRender     = "stack.render"
	affectedReasonStackValues     = "stack.values"
	affectedReasonStackValuesFile = "stack.values_files"
	affectedReasonStackChart      = "stack.chart"
	affectedReasonStackRepos      = "stack.repositories"
	affectedReasonDeleted         = "deleted"
	affectedReasonDeletedStack    = "deleted.stack"

	// Secret-file dependency: a component is affected because a SOPS secret file it consumes
	// changed. SOPS secret files are implicit file dependencies derived from the component's
	// declared secrets (store-backed secrets are not files and contribute nothing).
	affectedReasonSecretFile = "secret.file"

	// Affected reasons for the remaining top-level component sections written by the
	// stack processor. Keep these (and componentSectionChecks below) in sync with the
	// sections assigned in stack_processor_merge.go (the comp[...] block) and with the
	// "Evaluated sections" list in website/docs/cli/commands/describe/describe-affected.mdx.
	affectedReasonStackProviders              = "stack.providers"
	affectedReasonStackRequiredProviders      = "stack.required_providers"
	affectedReasonStackRequiredVersion        = "stack.required_version"
	affectedReasonStackBackend                = "stack.backend"
	affectedReasonStackBackendType            = "stack.backend_type"
	affectedReasonStackRemoteStateBackend     = "stack.remote_state_backend"
	affectedReasonStackRemoteStateBackendType = "stack.remote_state_backend_type"
	affectedReasonStackAuth                   = "stack.auth"
	affectedReasonStackCommand                = "stack.command"
	affectedReasonStackDependencies           = "stack.dependencies"
)

// Deletion type constants.
const (
	deletionTypeComponent = "component"
	deletionTypeStack     = "stack"
)

// Section name constants for isEqual comparisons.
const (
	sectionNameMetadata  = "metadata"
	sectionNameVars      = "vars"
	sectionNameEnv       = "env"
	sectionNameSource    = "source"
	sectionNameProvision = "provision"
	sectionNameGenerate  = "generate"
	sectionNamePaths     = "paths"
	sectionNameManifests = "manifests"
	sectionNameRender    = "render"
	sectionNameValues    = "values"
	sectionNameValuesF   = "values_files"
	sectionNameChart     = "chart"
	sectionNameRepos     = "repositories"
)

// shouldSkipComponent determines if a component should be skipped based on metadata.
func shouldSkipComponent(metadataSection map[string]any, componentName string, excludeLocked bool) bool {
	// Skip abstract components.
	if metadataType, ok := metadataSection["type"].(string); ok {
		if metadataType == "abstract" {
			return true
		}
	}

	// Skip disabled components.
	if !isComponentEnabled(metadataSection, componentName) {
		return true
	}

	// Skip locked components if requested.
	if excludeLocked && isComponentLocked(metadataSection) {
		return true
	}

	return false
}

// addAffectedComponent adds an affected component to the list.
// This is a thread-safe helper that doesn't modify shared state.
func addAffectedComponent(
	affected *[]schema.Affected,
	atmosConfig *schema.AtmosConfiguration,
	componentName string,
	stackName string,
	componentType string,
	componentSection *map[string]any,
	affectedReason string,
	includeSpaceliftAdminStacks bool,
	currentStacks *map[string]any,
	includeSettings bool,
) error {
	affectedItem := schema.Affected{
		ComponentType: componentType,
		Component:     componentName,
		Stack:         stackName,
		Affected:      affectedReason,
	}

	// Append to the local slice (thread-safe as each goroutine has its own slice).
	return appendToAffected(
		atmosConfig,
		componentName,
		stackName,
		componentSection,
		affected,
		&affectedItem,
		includeSpaceliftAdminStacks,
		currentStacks,
		includeSettings,
	)
}

// sectionCheck pairs a top-level component section name with the `affected` reason
// reported when that section differs between the two refs.
type sectionCheck struct {
	section string
	reason  string
}

// componentSectionChecks lists the top-level component sections compared verbatim
// between refs to determine if a component is affected. `metadata` and `settings`
// are intentionally absent: they have bespoke handling (metadata gates component
// skipping; settings also drives dependency checks).
//
// This list MUST stay in sync with the sections the stack processor writes into the
// final component map (the comp[...] assignments in stack_processor_merge.go) and with
// the "Evaluated sections" list in
// website/docs/cli/commands/describe/describe-affected.mdx. `locals`, `overrides`,
// `inheritance`, `retry`, and `hooks` are deliberately excluded (see that doc for
// rationale): in particular `hooks` is operational/execution-time behavior (what runs
// before/after a command), not provisioned infrastructure, so it does not mark a
// component as affected by default. Users who want it can add `hooks` to
// `describe.affected.sections`, where it reports as `stack.hooks`.
//
// Order is significant: the first changed section becomes the headline `affected`
// reason (all changed sections are still recorded in `affected_all`).
var componentSectionChecks = []sectionCheck{
	{sectionNameVars, affectedReasonStackVars},
	{sectionNameEnv, affectedReasonStackEnv},
	{cfg.ProvidersSectionName, affectedReasonStackProviders},
	{cfg.RequiredProvidersSectionName, affectedReasonStackRequiredProviders},
	{cfg.RequiredVersionSectionName, affectedReasonStackRequiredVersion},
	{cfg.GenerateSectionName, affectedReasonStackGenerate},
	{cfg.BackendSectionName, affectedReasonStackBackend},
	{cfg.BackendTypeSectionName, affectedReasonStackBackendType},
	{cfg.RemoteStateBackendSectionName, affectedReasonStackRemoteStateBackend},
	{cfg.RemoteStateBackendTypeSectionName, affectedReasonStackRemoteStateBackendType},
	{cfg.AuthSectionName, affectedReasonStackAuth},
	{cfg.CommandSectionName, affectedReasonStackCommand},
	{cfg.DependenciesSectionName, affectedReasonStackDependencies},
	{sectionNameSource, affectedReasonStackSource},
	{sectionNameProvision, affectedReasonStackProvision},
}

// resolveComponentSectionChecks returns the effective list of section checks. When
// `describe.affected.sections` is configured it fully replaces the built-in defaults:
// each configured name is mapped to its labeled reason when known, otherwise to a
// generic `stack.<name>` reason so custom sections still report sensibly.
func resolveComponentSectionChecks(atmosConfig *schema.AtmosConfiguration) []sectionCheck {
	if atmosConfig == nil || len(atmosConfig.Describe.Affected.Sections) == 0 {
		return componentSectionChecks
	}

	reasonByName := make(map[string]string, len(componentSectionChecks))
	for _, c := range componentSectionChecks {
		reasonByName[c.section] = c.reason
	}

	checks := make([]sectionCheck, 0, len(atmosConfig.Describe.Affected.Sections))
	for _, name := range atmosConfig.Describe.Affected.Sections {
		reason, ok := reasonByName[name]
		if !ok {
			reason = fmt.Sprintf("stack.%s", name)
		}
		checks = append(checks, sectionCheck{section: name, reason: reason})
	}
	return checks
}

// checkComponentSections compares every section in the effective check list that is
// present on the component and records the component as affected on any difference.
func checkComponentSections(
	affected *[]schema.Affected,
	atmosConfig *schema.AtmosConfiguration,
	componentName string,
	stackName string,
	componentType string,
	componentSection *map[string]any,
	remoteStacks *map[string]any,
	currentStacks *map[string]any,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
) error {
	locator := remoteComponentLocator{
		remoteStacks:  remoteStacks,
		stackName:     stackName,
		componentType: componentType,
		componentName: componentName,
	}

	for _, c := range resolveComponentSectionChecks(atmosConfig) {
		value, ok := (*componentSection)[c.section]
		if !ok {
			continue
		}
		if isSectionValueEqual(locator, value, c.section) {
			continue
		}
		if err := addAffectedComponent(affected, atmosConfig, componentName, stackName, componentType,
			componentSection, c.reason, includeSpaceliftAdminStacks, currentStacks, includeSettings); err != nil {
			return err
		}
	}
	return nil
}

// processTerraformComponentsIndexed processes Terraform components using the files index.
//
//nolint:cyclop,funlen // Component processing requires checking multiple sections (metadata, vars, env, settings, modules)
func processTerraformComponentsIndexed(
	stackName string,
	terraformSection map[string]any,
	remoteStacks *map[string]any,
	currentStacks *map[string]any,
	atmosConfig *schema.AtmosConfiguration,
	filesIndex *changedFilesIndex,
	patternCache *componentPathPatternCache,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	excludeLocked bool,
) ([]schema.Affected, error) {
	var affected []schema.Affected

	for componentName, compSection := range terraformSection {
		componentSection, ok := compSection.(map[string]any)
		if !ok {
			continue
		}

		// Check metadata section and skip if needed.
		metadataSection, hasMetadata := componentSection[sectionNameMetadata].(map[string]any)
		if hasMetadata {
			if shouldSkipComponent(metadataSection, componentName, excludeLocked) {
				continue
			}

			// Check metadata changes.
			if !isEqual(remoteStacks, stackName, cfg.TerraformComponentType, componentName, metadataSection, sectionNameMetadata) {
				err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.TerraformComponentType,
					&componentSection, affectedReasonStackMetadata, includeSpaceliftAdminStacks, currentStacks, includeSettings)
				if err != nil {
					return nil, err
				}
			}
		}

		// Resolve the component folder for path matching.
		component := GetComponentFolder(&componentSection, componentName)

		// Check component folder and module changes.
		changed, err := areTerraformComponentModulesChangedIndexed(component, atmosConfig, filesIndex, patternCache)
		if err != nil {
			return nil, err
		}
		if changed {
			err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.TerraformComponentType,
				&componentSection, affectedReasonComponentModule, false, nil, includeSettings)
			if err != nil {
				return nil, err
			}
		}

		// Check component folder changes.
		changed, err = isComponentFolderChangedIndexed(component, cfg.TerraformComponentType, atmosConfig, filesIndex, patternCache)
		if err != nil {
			return nil, err
		}
		if changed {
			err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.TerraformComponentType,
				&componentSection, affectedReasonComponent, includeSpaceliftAdminStacks, currentStacks, includeSettings)
			if err != nil {
				return nil, err
			}
		}

		// Check the comparable component sections (vars, env, providers, hooks, generate,
		// backend, source, provision, ...) via the shared section table. `metadata` is
		// handled above; `settings` is handled below because it also drives dependency checks.
		err = checkComponentSections(
			&affected, atmosConfig, componentName, stackName, cfg.TerraformComponentType,
			&componentSection, remoteStacks, currentStacks,
			includeSpaceliftAdminStacks, includeSettings,
		)
		if err != nil {
			return nil, err
		}

		if settingsSection, ok := componentSection[cfg.SettingsSectionName].(map[string]any); ok {
			err := checkSettingsAndDependenciesIndexed(
				&affected, atmosConfig, componentName, stackName, cfg.TerraformComponentType,
				&componentSection, settingsSection, remoteStacks, currentStacks, filesIndex,
				includeSpaceliftAdminStacks, includeSettings,
			)
			if err != nil {
				return nil, err
			}
		}
	}

	return affected, nil
}

// processHelmfileComponentsIndexed processes Helmfile components using the files index.
//
//nolint:cyclop,dupl,funlen // Similar structure to processPackerComponentsIndexed but for different component type
func processHelmfileComponentsIndexed(
	stackName string,
	helmfileSection map[string]any,
	remoteStacks *map[string]any,
	currentStacks *map[string]any,
	atmosConfig *schema.AtmosConfiguration,
	filesIndex *changedFilesIndex,
	patternCache *componentPathPatternCache,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	excludeLocked bool,
) ([]schema.Affected, error) {
	var affected []schema.Affected

	for componentName, compSection := range helmfileSection {
		componentSection, ok := compSection.(map[string]any)
		if !ok {
			continue
		}

		metadataSection, hasMetadata := componentSection[sectionNameMetadata].(map[string]any)
		if hasMetadata {
			if shouldSkipComponent(metadataSection, componentName, excludeLocked) {
				continue
			}

			if !isEqual(remoteStacks, stackName, cfg.HelmfileComponentType, componentName, metadataSection, sectionNameMetadata) {
				err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.HelmfileComponentType,
					&componentSection, affectedReasonStackMetadata, false, nil, includeSettings)
				if err != nil {
					return nil, err
				}
			}
		}

		// Resolve the component folder for path matching.
		component := GetComponentFolder(&componentSection, componentName)

		changed, err := isComponentFolderChangedIndexed(component, cfg.HelmfileComponentType, atmosConfig, filesIndex, patternCache)
		if err != nil {
			return nil, err
		}
		if changed {
			err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.HelmfileComponentType,
				&componentSection, affectedReasonComponent, false, nil, includeSettings)
			if err != nil {
				return nil, err
			}
		}

		// Check the comparable component sections (vars, env, source, provision, ...) via the
		// shared section table. `metadata` is handled above; `settings` is handled below
		// because it also drives dependency checks.
		err = checkComponentSections(
			&affected, atmosConfig, componentName, stackName, cfg.HelmfileComponentType,
			&componentSection, remoteStacks, currentStacks,
			false, includeSettings,
		)
		if err != nil {
			return nil, err
		}

		if settingsSection, ok := componentSection[cfg.SettingsSectionName].(map[string]any); ok {
			err := checkSettingsAndDependenciesIndexed(
				&affected, atmosConfig, componentName, stackName, cfg.HelmfileComponentType,
				&componentSection, settingsSection, remoteStacks, currentStacks, filesIndex,
				includeSpaceliftAdminStacks, includeSettings,
			)
			if err != nil {
				return nil, err
			}
		}
	}

	return affected, nil
}

// processPackerComponentsIndexed processes Packer components using the files index.
//
//nolint:cyclop,dupl,funlen // Similar structure to processHelmfileComponentsIndexed but for different component type
func processPackerComponentsIndexed(
	stackName string,
	packerSection map[string]any,
	remoteStacks *map[string]any,
	currentStacks *map[string]any,
	atmosConfig *schema.AtmosConfiguration,
	filesIndex *changedFilesIndex,
	patternCache *componentPathPatternCache,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	excludeLocked bool,
) ([]schema.Affected, error) {
	var affected []schema.Affected

	for componentName, compSection := range packerSection {
		componentSection, ok := compSection.(map[string]any)
		if !ok {
			continue
		}

		metadataSection, hasMetadata := componentSection[sectionNameMetadata].(map[string]any)
		if hasMetadata {
			if shouldSkipComponent(metadataSection, componentName, excludeLocked) {
				continue
			}

			if !isEqual(remoteStacks, stackName, cfg.PackerComponentType, componentName, metadataSection, sectionNameMetadata) {
				err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.PackerComponentType,
					&componentSection, affectedReasonStackMetadata, false, nil, includeSettings)
				if err != nil {
					return nil, err
				}
			}
		}

		// Resolve the component folder for path matching.
		component := GetComponentFolder(&componentSection, componentName)

		changed, err := isComponentFolderChangedIndexed(component, cfg.PackerComponentType, atmosConfig, filesIndex, patternCache)
		if err != nil {
			return nil, err
		}
		if changed {
			err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.PackerComponentType,
				&componentSection, affectedReasonComponent, false, nil, includeSettings)
			if err != nil {
				return nil, err
			}
		}

		// Check the comparable component sections (vars, env, source, provision, ...) via the
		// shared section table. `metadata` is handled above; `settings` is handled below
		// because it also drives dependency checks.
		err = checkComponentSections(
			&affected, atmosConfig, componentName, stackName, cfg.PackerComponentType,
			&componentSection, remoteStacks, currentStacks,
			false, includeSettings,
		)
		if err != nil {
			return nil, err
		}

		if settingsSection, ok := componentSection[cfg.SettingsSectionName].(map[string]any); ok {
			err := checkSettingsAndDependenciesIndexed(
				&affected, atmosConfig, componentName, stackName, cfg.PackerComponentType,
				&componentSection, settingsSection, remoteStacks, currentStacks, filesIndex,
				includeSpaceliftAdminStacks, includeSettings,
			)
			if err != nil {
				return nil, err
			}
		}
	}

	return affected, nil
}

// processKubernetesComponentsIndexed processes Kubernetes components using the files index.
//
//nolint:funlen // Similar structure to Helmfile/Packer with Kubernetes-specific sections
func processKubernetesComponentsIndexed(
	stackName string,
	kubernetesSection map[string]any,
	remoteStacks *map[string]any,
	currentStacks *map[string]any,
	atmosConfig *schema.AtmosConfiguration,
	filesIndex *changedFilesIndex,
	patternCache *componentPathPatternCache,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	excludeLocked bool,
) ([]schema.Affected, error) {
	var affected []schema.Affected

	for componentName, compSection := range kubernetesSection {
		componentSection, ok := compSection.(map[string]any)
		if !ok {
			continue
		}

		metadataSection, hasMetadata := componentSection[sectionNameMetadata].(map[string]any)
		if hasMetadata {
			if shouldSkipComponent(metadataSection, componentName, excludeLocked) {
				continue
			}

			if !isEqual(remoteStacks, stackName, cfg.KubernetesComponentType, componentName, metadataSection, sectionNameMetadata) {
				err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.KubernetesComponentType,
					&componentSection, affectedReasonStackMetadata, includeSpaceliftAdminStacks, currentStacks, includeSettings)
				if err != nil {
					return nil, err
				}
			}
		}

		component := GetComponentFolder(&componentSection, componentName)

		changed, err := isComponentFolderChangedIndexed(component, cfg.KubernetesComponentType, atmosConfig, filesIndex, patternCache)
		if err != nil {
			return nil, err
		}
		if changed {
			err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.KubernetesComponentType,
				&componentSection, affectedReasonComponent, includeSpaceliftAdminStacks, currentStacks, includeSettings)
			if err != nil {
				return nil, err
			}
		}

		if err := addKubernetesSectionAffected(&affected, atmosConfig, componentName, stackName, &componentSection, remoteStacks, currentStacks, includeSpaceliftAdminStacks, includeSettings); err != nil {
			return nil, err
		}

		if settingsSection, ok := componentSection[cfg.SettingsSectionName].(map[string]any); ok {
			err := checkSettingsAndDependenciesIndexed(
				&affected, atmosConfig, componentName, stackName, cfg.KubernetesComponentType,
				&componentSection, settingsSection, remoteStacks, currentStacks, filesIndex,
				includeSpaceliftAdminStacks, includeSettings,
			)
			if err != nil {
				return nil, err
			}
		}
	}

	return affected, nil
}

func addKubernetesSectionAffected(
	affected *[]schema.Affected,
	atmosConfig *schema.AtmosConfiguration,
	componentName string,
	stackName string,
	componentSection *map[string]any,
	remoteStacks *map[string]any,
	currentStacks *map[string]any,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
) error {
	// These sections include the Kubernetes-specific paths/manifests/render, which are
	// not part of the shared componentSectionChecks table; comparison reuses the shared
	// isSectionValueEqual primitive via remoteComponentLocator.
	sections := append([]sectionCheck{}, []sectionCheck{
		{sectionNameVars, affectedReasonStackVars},
		{sectionNameEnv, affectedReasonStackEnv},
		{sectionNameSource, affectedReasonStackSource},
		{sectionNameProvision, affectedReasonStackProvision},
		{sectionNameGenerate, affectedReasonStackGenerate},
		{cfg.ProviderSectionName, fmt.Sprintf("stack.%s", cfg.ProviderSectionName)},
		{sectionNamePaths, affectedReasonStackPaths},
		{sectionNameManifests, affectedReasonStackManifests},
		{sectionNameRender, affectedReasonStackRender},
	}...)
	sections = appendSectionChecks(sections, resolveComponentSectionChecks(atmosConfig)...)

	locator := remoteComponentLocator{
		remoteStacks:  remoteStacks,
		stackName:     stackName,
		componentType: cfg.KubernetesComponentType,
		componentName: componentName,
	}

	for _, section := range sections {
		value, ok := (*componentSection)[section.section]
		if !ok {
			continue
		}
		if isSectionValueEqual(locator, value, section.section) {
			continue
		}
		err := addAffectedComponent(affected, atmosConfig, componentName, stackName, cfg.KubernetesComponentType,
			componentSection, section.reason, includeSpaceliftAdminStacks, currentStacks, includeSettings)
		if err != nil {
			return err
		}
	}

	return nil
}

// processHelmComponentsIndexed processes native Helm components using the files index.
//
//nolint:funlen,dupl // Similar structure to Kubernetes with Helm-specific sections.
func processHelmComponentsIndexed(
	stackName string,
	helmSection map[string]any,
	remoteStacks *map[string]any,
	currentStacks *map[string]any,
	atmosConfig *schema.AtmosConfiguration,
	filesIndex *changedFilesIndex,
	patternCache *componentPathPatternCache,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
	excludeLocked bool,
) ([]schema.Affected, error) {
	var affected []schema.Affected

	for componentName, compSection := range helmSection {
		componentSection, ok := compSection.(map[string]any)
		if !ok {
			continue
		}

		metadataSection, hasMetadata := componentSection[sectionNameMetadata].(map[string]any)
		if hasMetadata {
			if shouldSkipComponent(metadataSection, componentName, excludeLocked) {
				continue
			}

			if !isEqual(remoteStacks, stackName, cfg.HelmComponentType, componentName, metadataSection, sectionNameMetadata) {
				err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.HelmComponentType,
					&componentSection, affectedReasonStackMetadata, includeSpaceliftAdminStacks, currentStacks, includeSettings)
				if err != nil {
					return nil, err
				}
			}
		}

		component := GetComponentFolder(&componentSection, componentName)

		changed, err := isComponentFolderChangedIndexed(component, cfg.HelmComponentType, atmosConfig, filesIndex, patternCache)
		if err != nil {
			return nil, err
		}
		if changed {
			err := addAffectedComponent(&affected, atmosConfig, componentName, stackName, cfg.HelmComponentType,
				&componentSection, affectedReasonComponent, includeSpaceliftAdminStacks, currentStacks, includeSettings)
			if err != nil {
				return nil, err
			}
		}

		if err := addHelmSectionAffected(&affected, atmosConfig, componentName, stackName, &componentSection, remoteStacks, currentStacks, includeSpaceliftAdminStacks, includeSettings); err != nil {
			return nil, err
		}

		if settingsSection, ok := componentSection[cfg.SettingsSectionName].(map[string]any); ok {
			err := checkSettingsAndDependenciesIndexed(
				&affected, atmosConfig, componentName, stackName, cfg.HelmComponentType,
				&componentSection, settingsSection, remoteStacks, currentStacks, filesIndex,
				includeSpaceliftAdminStacks, includeSettings,
			)
			if err != nil {
				return nil, err
			}
		}
	}

	return affected, nil
}

func addHelmSectionAffected(
	affected *[]schema.Affected,
	atmosConfig *schema.AtmosConfiguration,
	componentName string,
	stackName string,
	componentSection *map[string]any,
	remoteStacks *map[string]any,
	currentStacks *map[string]any,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
) error {
	sections := []struct {
		name   string
		reason string
	}{
		{sectionNameVars, affectedReasonStackVars},
		{sectionNameEnv, affectedReasonStackEnv},
		{sectionNameSource, affectedReasonStackSource},
		{sectionNameProvision, affectedReasonStackProvision},
		{sectionNameGenerate, affectedReasonStackGenerate},
		{sectionNameRender, affectedReasonStackRender},
		{sectionNameValues, affectedReasonStackValues},
		{sectionNameValuesF, affectedReasonStackValuesFile},
		{sectionNameChart, affectedReasonStackChart},
		{sectionNameRepos, affectedReasonStackRepos},
	}

	for _, section := range sections {
		value, ok := (*componentSection)[section.name]
		if !ok {
			continue
		}
		if isComponentSectionEqual(remoteStacks, stackName, cfg.HelmComponentType, componentName, value, section.name) {
			continue
		}
		err := addAffectedComponent(affected, atmosConfig, componentName, stackName, cfg.HelmComponentType,
			componentSection, section.reason, includeSpaceliftAdminStacks, currentStacks, includeSettings)
		if err != nil {
			return err
		}
	}

	return nil
}

func isComponentSectionEqual(
	remoteStacks *map[string]any,
	localStackName string,
	componentType string,
	localComponentName string,
	localSection any,
	sectionName string,
) bool {
	if remoteStackSection, ok := (*remoteStacks)[localStackName].(map[string]any); ok {
		if remoteComponentsSection, ok := remoteStackSection["components"].(map[string]any); ok {
			if remoteComponentTypeSection, ok := remoteComponentsSection[componentType].(map[string]any); ok {
				if remoteComponentSection, ok := remoteComponentTypeSection[localComponentName].(map[string]any); ok {
					remoteSection, ok := remoteComponentSection[sectionName]
					if ok {
						return reflect.DeepEqual(localSection, remoteSection)
					}
				}
			}
		}
	}
	return false
}

func appendSectionChecks(base []sectionCheck, extras ...sectionCheck) []sectionCheck {
	seen := make(map[string]struct{}, len(base)+len(extras))
	for _, section := range base {
		seen[section.section] = struct{}{}
	}
	for _, section := range extras {
		if _, ok := seen[section.section]; ok {
			continue
		}
		base = append(base, section)
		seen[section.section] = struct{}{}
	}
	return base
}

// checkSettingsAndDependenciesIndexed checks settings using indexed files.
func checkSettingsAndDependenciesIndexed(
	affected *[]schema.Affected,
	atmosConfig *schema.AtmosConfiguration,
	componentName string,
	stackName string,
	componentType string,
	componentSection *map[string]any,
	settingsSection map[string]any,
	remoteStacks *map[string]any,
	currentStacks *map[string]any,
	filesIndex *changedFilesIndex,
	includeSpaceliftAdminStacks bool,
	includeSettings bool,
) error {
	// Check settings section changes.
	if !isEqual(remoteStacks, stackName, componentType, componentName, settingsSection, cfg.SettingsSectionName) {
		err := addAffectedComponent(affected, atmosConfig, componentName, stackName, componentType,
			componentSection, affectedReasonStackSettings, includeSpaceliftAdminStacks, currentStacks, includeSettings)
		if err != nil {
			return err
		}
	}

	// Check settings.depends_on using indexed version.
	return checkDependencyChangesIndexed(
		affected, atmosConfig, componentName, stackName, componentType,
		componentSection, settingsSection, filesIndex,
		includeSpaceliftAdminStacks, currentStacks, includeSettings,
	)
}

// checkDependencyChangesIndexed checks if dependent files or folders have changed.
// This helper reduces cyclomatic complexity of checkSettingsAndDependenciesIndexed.
// It checks both dependencies.components (preferred) and settings.depends_on (legacy) for file/folder dependencies.
func checkDependencyChangesIndexed(
	affected *[]schema.Affected,
	atmosConfig *schema.AtmosConfiguration,
	componentName string,
	stackName string,
	componentType string,
	componentSection *map[string]any,
	settingsSection map[string]any,
	filesIndex *changedFilesIndex,
	includeSpaceliftAdminStacks bool,
	currentStacks *map[string]any,
	includeSettings bool,
) error {
	// Get file/folder dependencies from dependencies.components or settings.depends_on.
	deps := getFileFolderDependencies(*componentSection, settingsSection)
	if len(deps) > 0 {
		isFolderOrFileChanged, changedType, changedFileOrFolder, err := isComponentDependentFolderOrFileChangedIndexed(
			filesIndex,
			deps,
		)
		if err != nil {
			return err
		}
		if isFolderOrFileChanged {
			return addDependencyAffectedItem(
				affected, atmosConfig, componentName, stackName, componentType,
				componentSection, changedType, changedFileOrFolder,
				includeSpaceliftAdminStacks, currentStacks, includeSettings,
			)
		}
	}

	// SOPS secret files the component consumes are implicit file dependencies: a changed secret
	// file marks every component that reads it affected (store-backed secrets are not files).
	return checkSecretFileChangesIndexed(
		affected, atmosConfig, componentName, stackName, componentType,
		componentSection, filesIndex, includeSpaceliftAdminStacks, currentStacks, includeSettings,
	)
}

// checkSecretFileChangesIndexed marks the component affected (reason secret.file) when any SOPS
// secret file it consumes has changed. The backing files are derived from the component's resolved
// `secrets:` section via the same path logic the SOPS provider uses, so detection and storage can
// never drift.
func checkSecretFileChangesIndexed(
	affected *[]schema.Affected,
	atmosConfig *schema.AtmosConfiguration,
	componentName string,
	stackName string,
	componentType string,
	componentSection *map[string]any,
	filesIndex *changedFilesIndex,
	includeSpaceliftAdminStacks bool,
	currentStacks *map[string]any,
	includeSettings bool,
) error {
	secretDeps := getSecretFileDependencies(atmosConfig, stackName, componentName, *componentSection)
	if len(secretDeps) == 0 {
		return nil
	}

	changed, _, changedFile, err := isComponentDependentFolderOrFileChangedIndexed(filesIndex, secretDeps)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}

	return addDependencyAffectedItem(
		affected, atmosConfig, componentName, stackName, componentType,
		componentSection, affectedReasonSecretFile, changedFile,
		includeSpaceliftAdminStacks, currentStacks, includeSettings,
	)
}

// getSecretFileDependencies returns the SOPS files the component's declared secrets resolve to,
// as file dependencies. Store-backed secrets contribute nothing (they are not files). It is
// best-effort: declarations whose provider/path cannot be resolved are skipped.
func getSecretFileDependencies(atmosConfig *schema.AtmosConfiguration, stackName, componentName string, componentSection map[string]any) []schema.ComponentDependency {
	files := secrets.NewService(atmosConfig, stackName, componentName, componentSection).FileDependencies()
	if len(files) == 0 {
		return nil
	}
	deps := make([]schema.ComponentDependency, 0, len(files))
	for _, f := range files {
		deps = append(deps, schema.ComponentDependency{Kind: "file", Path: f})
	}
	return deps
}

// getFileFolderDependencies extracts file/folder dependencies from dependencies.components or settings.depends_on.
// Returns a slice of ComponentDependency with kind="file" or kind="folder".
func getFileFolderDependencies(componentSection map[string]any, settingsSection map[string]any) []schema.ComponentDependency {
	// Check dependencies.components first (preferred location).
	if result := getFileFolderDependenciesFromNewFormat(componentSection); len(result) > 0 {
		return result
	}

	// Fall back to settings.depends_on (legacy location).
	return getFileFolderDependenciesFromLegacyFormat(settingsSection)
}

// getFileFolderDependenciesFromNewFormat extracts file/folder deps from the
// `dependencies` section. It accepts both the v2 surface
// (`dependencies.files` / `dependencies.folders` sibling keys) and the legacy
// inline shape (`dependencies.components[]` with `kind: file` / `kind: folder`).
// Both surfaces produce equivalent ComponentDependency entries — Normalize
// reconciles them.
func getFileFolderDependenciesFromNewFormat(componentSection map[string]any) []schema.ComponentDependency {
	depsSection, ok := componentSection[cfg.DependenciesSectionName].(map[string]any)
	if !ok {
		return nil
	}

	// Fast path: nothing to read if none of the entry-bearing keys are present.
	if !hasDependencyEntries(depsSection) {
		return nil
	}

	var deps schema.Dependencies
	if err := mapstructure.Decode(depsSection, &deps); err != nil {
		return nil
	}
	if err := deps.Normalize(); err != nil {
		log.Warn("invalid dependencies section; file/folder deps may be silently ignored", "error", err)
		return nil
	}
	if len(deps.Components) == 0 {
		return nil
	}

	// Filter to only file/folder dependencies. Normalize has already mirrored
	// any v2 sibling-key entries into Components, so this single filter
	// covers both surfaces.
	var result []schema.ComponentDependency
	for i := range deps.Components {
		if deps.Components[i].IsFileDependency() || deps.Components[i].IsFolderDependency() {
			result = append(result, deps.Components[i])
		}
	}
	return result
}

// getFileFolderDependenciesFromLegacyFormat extracts file/folder deps from settings.depends_on.
func getFileFolderDependenciesFromLegacyFormat(settingsSection map[string]any) []schema.ComponentDependency {
	if settingsSection == nil {
		return nil
	}

	var stackComponentSettings schema.Settings
	if err := mapstructure.Decode(settingsSection, &stackComponentSettings); err != nil {
		return nil
	}

	if reflect.ValueOf(stackComponentSettings.DependsOn).IsZero() || len(stackComponentSettings.DependsOn) == 0 {
		return nil
	}

	// Filter to only file/folder entries and convert to ComponentDependency.
	var result []schema.ComponentDependency
	for key := range stackComponentSettings.DependsOn {
		dep := stackComponentSettings.DependsOn[key]
		if dep.File != "" {
			result = append(result, schema.ComponentDependency{
				Kind: "file",
				Path: dep.File,
			})
		}
		if dep.Folder != "" {
			result = append(result, schema.ComponentDependency{
				Kind: "folder",
				Path: dep.Folder,
			})
		}
	}

	if len(result) > 0 {
		log.Debug("'settings.depends_on' is deprecated, use 'dependencies.components' instead. See: https://atmos.tools/stacks/dependencies/components")
	}

	return result
}

// addDependencyAffectedItem adds an affected item for a dependency change.
// This helper further reduces complexity by handling the affected item creation.
func addDependencyAffectedItem(
	affected *[]schema.Affected,
	atmosConfig *schema.AtmosConfiguration,
	componentName string,
	stackName string,
	componentType string,
	componentSection *map[string]any,
	changedType string,
	changedFileOrFolder string,
	includeSpaceliftAdminStacks bool,
	currentStacks *map[string]any,
	includeSettings bool,
) error {
	changedFile := ""
	if changedType == "file" || changedType == affectedReasonSecretFile {
		changedFile = changedFileOrFolder
	}

	changedFolder := ""
	if changedType == "folder" {
		changedFolder = changedFileOrFolder
	}

	affectedItem := schema.Affected{
		ComponentType: componentType,
		Component:     componentName,
		Stack:         stackName,
		Affected:      changedType,
		File:          changedFile,
		Folder:        changedFolder,
	}

	return appendToAffected(
		atmosConfig,
		componentName,
		stackName,
		componentSection,
		affected,
		&affectedItem,
		includeSpaceliftAdminStacks,
		currentStacks,
		includeSettings,
	)
}
