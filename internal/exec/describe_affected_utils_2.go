package exec

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// appendToAffected adds an item to the affected list, and adds the Spacelift stack and Atlantis project (if configured).
func appendToAffected(
	atmosConfig *schema.AtmosConfiguration,
	componentName string,
	stackName string,
	componentSection *map[string]any,
	affectedList *[]schema.Affected,
	affected *schema.Affected,
	includeSpaceliftAdminStacks bool,
	stacks *map[string]any,
	includeSettings bool,
) error {
	// Append the affected section to the `affected_all` slice.
	affected.AffectedAll = append(affected.AffectedAll, affected.Affected)

	// If the affected component in the stack was already added to the result, don't add it again
	for i := range *affectedList {
		v := &(*affectedList)[i]
		if v.Component == affected.Component && v.Stack == affected.Stack && v.ComponentType == affected.ComponentType {
			// For the found item in the list, append the affected section to the `affected_all` slice.
			v.AffectedAll = append(v.AffectedAll, affected.Affected)
			return nil
		}
	}

	settingsSection := map[string]any{}

	if i, ok2 := (*componentSection)[cfg.SettingsSectionName]; ok2 {
		settingsSection = i.(map[string]any)

		if includeSettings {
			affected.Settings = settingsSection
		}
	}

	if affected.ComponentType == cfg.TerraformComponentType {
		varSection := map[string]any{}

		if i, ok2 := (*componentSection)[cfg.VarsSectionName]; ok2 {
			varSection = i.(map[string]any)
		}

		configAndStacksInfo := schema.ConfigAndStacksInfo{
			ComponentFromArg:         componentName,
			Stack:                    stackName,
			ComponentVarsSection:     varSection,
			ComponentSettingsSection: settingsSection,
			ComponentSection: map[string]any{
				cfg.VarsSectionName:     varSection,
				cfg.SettingsSectionName: settingsSection,
			},
		}

		// Affected Spacelift stack
		spaceliftStackName, err := BuildSpaceliftStackNameFromComponentConfig(atmosConfig, configAndStacksInfo)
		if err != nil {
			return err
		}
		affected.SpaceliftStack = spaceliftStackName

		// Affected Atlantis project
		atlantisProjectName, err := BuildAtlantisProjectNameFromComponentConfig(atmosConfig, configAndStacksInfo)
		if err != nil {
			return err
		}
		affected.AtlantisProject = atlantisProjectName

		if includeSpaceliftAdminStacks {
			affectedList, err = addAffectedSpaceliftAdminStack(
				atmosConfig,
				affectedList,
				&settingsSection,
				stacks,
				stackName,
				componentName,
				&configAndStacksInfo,
				includeSettings,
			)
			if err != nil {
				return err
			}
		}
	}

	// Check the `component` section and add `ComponentPath` to the output.
	// Pass componentName as fallback for JIT vendored components without explicit `component` field.
	affected.ComponentPath = BuildComponentPath(atmosConfig, componentSection, affected.ComponentType, componentName)
	affected.StackSlug = fmt.Sprintf("%s-%s", stackName, strings.Replace(componentName, "/", "-", -1))

	*affectedList = append(*affectedList, *affected)
	return nil
}

// isEqual compares a section of a component from the remote stacks with a section of a local component.
// Uses optimized custom deep comparison instead of reflect.DeepEqual for 15-25% improvement.
func isEqual(
	remoteStacks *map[string]any,
	localStackName string,
	componentType string,
	localComponentName string,
	localSection map[string]any,
	sectionName string,
) bool {
	if remoteStackSection, ok := (*remoteStacks)[localStackName].(map[string]any); ok {
		if remoteComponentsSection, ok := remoteStackSection["components"].(map[string]any); ok {
			if remoteComponentTypeSection, ok := remoteComponentsSection[componentType].(map[string]any); ok {
				if remoteComponentSection, ok := remoteComponentTypeSection[localComponentName].(map[string]any); ok {
					if remoteSection, ok := remoteComponentSection[sectionName].(map[string]any); ok {
						return deepEqualMaps(localSection, remoteSection)
					}
				}
			}
		}
	}
	return false
}

// remoteComponentLocator identifies a single component within the remote (target ref)
// stacks map, so section lookups don't need to thread the navigation keys as separate
// function arguments.
type remoteComponentLocator struct {
	remoteStacks  *map[string]any
	stackName     string
	componentType string
	componentName string
}

// remoteComponentMap resolves the raw section map for the located remote component, and
// whether the remote component path itself was found (not whether any specific section
// key exists within it).
func (l remoteComponentLocator) remoteComponentMap() (map[string]any, bool) {
	remoteStackSection, ok := (*l.remoteStacks)[l.stackName].(map[string]any)
	if !ok {
		return nil, false
	}
	remoteComponentsSection, ok := remoteStackSection["components"].(map[string]any)
	if !ok {
		return nil, false
	}
	remoteComponentTypeSection, ok := remoteComponentsSection[l.componentType].(map[string]any)
	if !ok {
		return nil, false
	}
	remoteComponentSection, ok := remoteComponentTypeSection[l.componentName].(map[string]any)
	return remoteComponentSection, ok
}

// section returns the raw value of the named section for the located remote component,
// and whether it was found.
func (l remoteComponentLocator) section(sectionName string) (any, bool) {
	m, ok := l.remoteComponentMap()
	if !ok {
		return nil, false
	}
	return m[sectionName], true
}

// sectionPresent reports whether sectionName exists as an explicit key on the located
// remote component. Unlike section, which defaults an absent key to nil for value
// comparison, this distinguishes "explicitly set" from "never set" — needed to detect
// when the LOCAL side removes a section the remote still has (section's ok only reflects
// whether the remote component path was found, not per-key presence).
func (l remoteComponentLocator) sectionPresent(sectionName string) bool {
	m, ok := l.remoteComponentMap()
	if !ok {
		return false
	}
	_, present := m[sectionName]
	return present
}

// isSectionValueEqual compares a local component section value with the corresponding value
// on the located remote component. Unlike isEqual, it accepts a value of any type, so it also
// handles scalar sections such as `backend_type`, `required_version`, and `command` (not just
// map sections). When the remote component path is absent it returns false (treated as
// changed/affected), matching isEqual semantics.
func isSectionValueEqual(locator remoteComponentLocator, localValue any, sectionName string) bool {
	remoteValue, ok := locator.section(sectionName)
	if !ok {
		return false
	}
	return deepEqualValues(localValue, remoteValue)
}

// deepEqualMaps performs optimized deep comparison of two maps.
// This avoids the overhead of reflect.DeepEqual by using type assertions.
// Correctly distinguishes between nil and empty maps to match reflect.DeepEqual behavior.
func deepEqualMaps(a, b map[string]any) bool {
	// Check if exactly one is nil (XOR).
	// This preserves reflect.DeepEqual behavior where nil != empty map.
	if (a == nil) != (b == nil) {
		return false
	}

	// Quick length check.
	if len(a) != len(b) {
		return false
	}

	// Compare all keys and values.
	for key, valueA := range a {
		valueB, exists := b[key]
		if !exists {
			return false
		}

		if !deepEqualValues(valueA, valueB) {
			return false
		}
	}

	return true
}

// deepEqualValues recursively compares two values of any type.
// Optimized for common types in Atmos configurations.
//
//nolint:cyclop,funlen,revive // Type switch for deep comparison requires explicit handling of all common types
func deepEqualValues(a, b any) bool {
	// Handle nil cases.
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Type assertions for common types to avoid reflection.
	switch aTyped := a.(type) {
	case map[string]any:
		bTyped, ok := b.(map[string]any)
		if !ok {
			return false
		}
		return deepEqualMaps(aTyped, bTyped)

	case []any:
		bTyped, ok := b.([]any)
		if !ok {
			return false
		}
		return deepEqualSlices(aTyped, bTyped)

	case string:
		bTyped, ok := b.(string)
		if !ok {
			return false
		}
		return aTyped == bTyped

	case int:
		bTyped, ok := b.(int)
		if !ok {
			return false
		}
		return aTyped == bTyped

	case int64:
		bTyped, ok := b.(int64)
		if !ok {
			return false
		}
		return aTyped == bTyped

	case float64:
		bTyped, ok := b.(float64)
		if !ok {
			return false
		}
		return aTyped == bTyped

	case bool:
		bTyped, ok := b.(bool)
		if !ok {
			return false
		}
		return aTyped == bTyped

	default:
		// Fallback to reflect.DeepEqual for uncommon types.
		return reflect.DeepEqual(a, b)
	}
}

// deepEqualSlices compares two slices recursively.
// Correctly distinguishes between nil and empty slices to match reflect.DeepEqual behavior.
func deepEqualSlices(a, b []any) bool {
	// Check if exactly one is nil (XOR).
	// This preserves reflect.DeepEqual behavior where nil != empty slice.
	if (a == nil) != (b == nil) {
		return false
	}

	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if !deepEqualValues(a[i], b[i]) {
			return false
		}
	}

	return true
}

// isComponentFolderChanged checks if the component folder changed (has changed files in the folder or its subfolders).
func isComponentFolderChanged(
	component string,
	componentType string,
	atmosConfig *schema.AtmosConfiguration,
	changedFiles []string,
) (bool, error) {
	var componentPath string

	switch componentType {
	case cfg.TerraformComponentType:
		componentPath = filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath, component)
	case cfg.HelmfileComponentType:
		componentPath = filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Helmfile.BasePath, component)
	case cfg.PackerComponentType:
		componentPath = filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Packer.BasePath, component)
	case cfg.KubernetesComponentType:
		componentPath = filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Kubernetes.BasePath, component)
	default:
		return false, fmt.Errorf("%w: %s", errUtils.ErrUnsupportedComponentType, componentType)
	}

	componentPathAbs, err := filepath.Abs(componentPath)
	if err != nil {
		return false, err
	}

	componentPathPattern := filepath.Join(componentPathAbs, "**")

	for _, changedFile := range changedFiles {
		changedFileAbs, err := filepath.Abs(changedFile)
		if err != nil {
			return false, err
		}

		match, err := u.PathMatch(componentPathPattern, changedFileAbs)
		if err != nil {
			return false, err
		}

		if match {
			return true, nil
		}
	}

	return false, nil
}

// areTerraformComponentModulesChanged checks if any of the external Terraform modules (but on the local filesystem) that the component uses have changed.
func areTerraformComponentModulesChanged(
	component string,
	atmosConfig *schema.AtmosConfiguration,
	changedFiles []string,
) (bool, error) {
	componentPath := filepath.Join(atmosConfig.BasePath, atmosConfig.Components.Terraform.BasePath, component)

	componentPathAbs, err := filepath.Abs(componentPath)
	if err != nil {
		return false, err
	}

	terraformConfiguration, diags := tfconfig.LoadModule(componentPathAbs)
	if diags.HasErrors() {
		diagErr := diags.Err()

		// Try structured error detection first (most robust).
		if errors.Is(diagErr, os.ErrNotExist) || errors.Is(diagErr, fs.ErrNotExist) {
			return false, nil
		}

		// Fallback to error message inspection for cases where tfconfig doesn't wrap errors properly.
		// This handles missing subdirectory modules (e.g., ./modules/security-group referenced in main.tf
		// but the directory doesn't exist). Such missing paths are valid in affected detection—components
		// or their modules may be deleted or not yet created when tracking changes over time.
		errMsg := diagErr.Error()
		if strings.Contains(errMsg, "does not exist") || strings.Contains(errMsg, "Failed to read directory") {
			return false, nil
		}

		// For other errors (syntax errors, permission issues, etc.), return error.
		return false, componentLoadError(component, diags)
	}

	// If no configuration, there are no modules to check.
	if terraformConfiguration == nil {
		return false, nil
	}

	for _, changedFile := range changedFiles {
		changedFileAbs, err := filepath.Abs(changedFile)
		if err != nil {
			return false, err
		}

		for _, moduleConfig := range terraformConfiguration.ModuleCalls {
			// We are processing the local modules only (not from terraform registry), they will have `Version` as an empty string
			if moduleConfig.Version != "" {
				continue
			}

			modulePath := filepath.Join(filepath.Dir(moduleConfig.Pos.Filename), moduleConfig.Source)

			modulePathAbs, err := filepath.Abs(modulePath)
			if err != nil {
				return false, err
			}

			modulePathPattern := filepath.Join(modulePathAbs, "**")

			match, err := u.PathMatch(modulePathPattern, changedFileAbs)
			if err != nil {
				return false, err
			}

			if match {
				return true, nil
			}
		}
	}

	return false, nil
}

// addAffectedSpaceliftAdminStack adds the affected Spacelift admin stack that manages the affected child stack.
func addAffectedSpaceliftAdminStack(
	atmosConfig *schema.AtmosConfiguration,
	affectedList *[]schema.Affected,
	settingsSection *map[string]any,
	stacks *map[string]any,
	currentStackName string,
	currentComponentName string,
	configAndStacksInfo *schema.ConfigAndStacksInfo,
	includeSettings bool,
) (*[]schema.Affected, error) {
	// Convert the `settings` section to the `Settings` structure
	var componentSettings schema.Settings
	err := mapstructure.Decode(settingsSection, &componentSettings)
	if err != nil {
		return nil, err
	}

	// Skip if the component has an empty `settings.spacelift` section
	if reflect.ValueOf(componentSettings).IsZero() ||
		reflect.ValueOf(componentSettings.Spacelift).IsZero() {
		return affectedList, nil
	}

	// Find and process `settings.spacelift.admin_stack_config` section
	var adminStackContextSection any
	var adminStackContext schema.Context
	var ok bool

	if adminStackContextSection, ok = componentSettings.Spacelift["admin_stack_selector"]; !ok {
		return affectedList, nil
	}

	err = mapstructure.Decode(adminStackContextSection, &adminStackContext)
	if err != nil {
		return nil, err
	}

	// Skip if the component has an empty `settings.spacelift.admin_stack_selector` section
	if reflect.ValueOf(adminStackContext).IsZero() {
		return affectedList, nil
	}

	var adminStackContextPrefix string

	if atmosConfig.Stacks.NameTemplate != "" {
		adminStackContextPrefix, err = processStackNameTemplate(atmosConfig, currentStackName, atmosConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, atmosConfig.Templates.Settings.IgnoreMissingTemplateValues)
		if err != nil {
			return nil, err
		}
	} else {
		adminStackContextPrefix, err = cfg.GetContextPrefix(currentStackName, adminStackContext, GetStackNamePattern(atmosConfig), currentStackName)
		if err != nil {
			return nil, err
		}
	}
	if err = ensureLiteralStackIdentity(currentStackName, adminStackContextPrefix, configAndStacksInfo.ComponentSection); err != nil {
		return nil, err
	}

	var componentVarsSection map[string]any
	var componentSettingsSection map[string]any
	var componentSettingsSpaceliftSection map[string]any

	// Find the Spacelift admin stack that manages the current stack
	if stacks == nil {
		return affectedList, nil
	}
	for stackName, stackSection := range *stacks {
		if stackSectionMap, ok := stackSection.(map[string]any); ok {
			if componentsSection, ok := stackSectionMap["components"].(map[string]any); ok {
				if terraformSection, ok := componentsSection[cfg.TerraformComponentType].(map[string]any); ok {
					for componentName, compSection := range terraformSection {
						if componentSection, ok := compSection.(map[string]any); ok {

							if componentVarsSection, ok = componentSection["vars"].(map[string]any); !ok {
								return affectedList, nil
							}

							var context schema.Context
							err = mapstructure.Decode(componentVarsSection, &context)
							if err != nil {
								return nil, err
							}

							var contextPrefix string

							if atmosConfig.Stacks.NameTemplate != "" {
								contextPrefix, err = processStackNameTemplate(atmosConfig, stackName, atmosConfig.Stacks.NameTemplate, configAndStacksInfo.ComponentSection, atmosConfig.Templates.Settings.IgnoreMissingTemplateValues)
								if err != nil {
									return nil, err
								}
							} else {
								contextPrefix, err = cfg.GetContextPrefix(stackName, context, GetStackNamePattern(atmosConfig), stackName)
								if err != nil {
									return nil, err
								}
							}
							if err = ensureLiteralStackIdentity(stackName, contextPrefix, configAndStacksInfo.ComponentSection); err != nil {
								return nil, err
							}

							if adminStackContext.Component == componentName && adminStackContextPrefix == contextPrefix {
								if componentSettingsSection, ok = componentSection[cfg.SettingsSectionName].(map[string]any); !ok {
									return affectedList, nil
								}

								if componentSettingsSpaceliftSection, ok = componentSettingsSection["spacelift"].(map[string]any); !ok {
									return affectedList, nil
								}

								if spaceliftWorkspaceEnabled, ok := componentSettingsSpaceliftSection["workspace_enabled"].(bool); !ok || !spaceliftWorkspaceEnabled {
									return nil, errors.New(fmt.Sprintf(
										"component '%s' in the stack '%s' has the section 'settings.spacelift.admin_stack_selector' "+
											"to point to the Spacelift admin component '%s' in the stack '%s', "+
											"but that component has Spacelift workspace disabled "+
											"in the 'settings.spacelift.workspace_enabled' section "+
											"and can't be added to the affected stacks",
										currentComponentName,
										currentStackName,
										componentName,
										stackName,
									))
								}

								affectedSpaceliftAdminStack := schema.Affected{
									ComponentType: cfg.TerraformComponentType,
									Component:     componentName,
									Stack:         stackName,
									Affected:      "stack.settings.spacelift.admin_stack_selector",
								}

								err = appendToAffected(
									atmosConfig,
									componentName,
									stackName,
									&componentSection,
									affectedList,
									&affectedSpaceliftAdminStack,
									false,
									nil,
									includeSettings,
								)
								if err != nil {
									return nil, err
								}
							}
						}
					}
				}
			}
		}
	}

	return affectedList, nil
}

// addDependentsToAffected adds dependent components and stacks to each affected component.
// It resolves all stacks once upfront and reuses the result for every affected component,
// avoiding O(N) full stack resolutions that made --upload hang on large infrastructures.
func addDependentsToAffected(
	atmosConfig *schema.AtmosConfiguration,
	affected *[]schema.Affected,
	includeSettings bool,
	processTemplates bool,
	processYamlFunctions bool,
	skip []string,
	onlyInStack string,
	authManager auth.AuthManager,
	authDisabled bool,
	errOptions DescribeStacksErrorOptions,
) error {
	return addDependentsToAffectedWithFilter(atmosConfig, affected, &dependentsOptions{
		IncludeSettings:      includeSettings,
		ProcessTemplates:     processTemplates,
		ProcessYamlFunctions: processYamlFunctions,
		Skip:                 skip,
		OnlyInStack:          onlyInStack,
		AuthManager:          authManager,
		AuthDisabled:         authDisabled,
		ErrOptions:           errOptions,
	})
}

// dependentsOptions bundles the settings shared by every dependents-resolution step.
type dependentsOptions struct {
	IncludeSettings      bool
	ProcessTemplates     bool
	ProcessYamlFunctions bool
	Skip                 []string
	OnlyInStack          string
	AuthManager          auth.AuthManager
	AuthDisabled         bool
	ErrOptions           DescribeStacksErrorOptions
	// Filter holds the `--tags` / `--labels` selectors. When it has selectors, each dependent's `metadata`
	// is recorded as the dependents are resolved so the nested lists can be pruned afterward.
	Filter AffectedFilter
	// RecordMetadata records each dependent's `metadata` even without selectors. `--exclude-locked` with
	// `--flatten` needs it to tell which lifted dependents are locked.
	RecordMetadata bool

	// stacks and depIdx are resolved once by addDependentsToAffectedWithFilter and shared by every step.
	stacks map[string]any
	depIdx dependencyIndex
}

// addDependentsToAffectedWithFilter is addDependentsToAffected plus the `--tags` / `--labels` selectors.
// The stacks are already loaded when the dependents are resolved, so recording each dependent's metadata
// needs no second stack resolution. Without selectors it behaves exactly like addDependentsToAffected and
// leaves the dependents untouched.
func addDependentsToAffectedWithFilter(
	atmosConfig *schema.AtmosConfiguration,
	affected *[]schema.Affected,
	opts *dependentsOptions,
) error {
	// Resolve all stacks once and build a reverse dependency index — these are the expensive
	// operations (~1s for large infras). Previously ExecuteDescribeStacks was called inside
	// ExecuteDescribeDependents for every affected component, causing O(N) full resolutions
	// (e.g., 2,422 × ~1s = 40+ minutes). The dependency index further eliminates the
	// O(stacks × components) scan per affected item.
	var err error
	opts.stacks, err = ExecuteDescribeStacksWithOptions(
		atmosConfig,
		opts.OnlyInStack,
		nil,
		nil,
		nil,
		false,
		opts.ProcessTemplates,
		opts.ProcessYamlFunctions,
		false,
		opts.Skip,
		opts.AuthManager,
		opts.AuthDisabled,
		opts.ErrOptions,
	)
	if err != nil {
		return err
	}

	// Build the reverse dependency index once from the cached stacks.
	leftDelim, _ := tags.TemplateDelims(atmosConfig.Templates.Settings.Delimiters)
	opts.depIdx, err = buildDependencyIndexWithError(opts.stacks, leftDelim)
	if err != nil {
		return err
	}

	for i := 0; i < len(*affected); i++ {
		a := &(*affected)[i]

		// Skip deleted components — they don't exist in HEAD and can't have dependents.
		// Attempting to resolve them would cause "invalid component" errors.
		if a.Deleted {
			a.Dependents = []schema.Dependent{}
			continue
		}

		// Skip if `onlyInStack` is specified and the affected component is not in the specified stack.
		if opts.OnlyInStack != "" && a.Stack != opts.OnlyInStack {
			continue
		}

		if err := addDependentsToAffectedItem(atmosConfig, a, opts); err != nil {
			return err
		}
	}

	processIncludedInDependencies(affected)
	return nil
}

// needsDependentMetadata reports whether each resolved dependent must record its `metadata` section:
// to apply the `--tags` / `--labels` selectors, or to honour `--exclude-locked` when dependents are lifted.
func (o *dependentsOptions) needsDependentMetadata() bool {
	return o.Filter.hasSelectors() || o.RecordMetadata
}

// isExcludedLockedDependent reports whether the dependent is locked (`metadata.locked: true`) and the filter
// excludes locked components. It needs the metadata recorded by attachDependentMetadata; a dependent
// without recorded metadata is never reported as locked.
func (f AffectedFilter) isExcludedLockedDependent(d *schema.Dependent) bool {
	return f.ExcludeLocked && d.Metadata != nil && isComponentLocked(d.Metadata)
}

// addDependentsToAffectedItem resolves the (nested) dependents of one affected component from the
// pre-computed stacks and reverse dependency index.
func addDependentsToAffectedItem(
	atmosConfig *schema.AtmosConfiguration,
	a *schema.Affected,
	opts *dependentsOptions,
) error {
	deps, err := describeDependentsFor(atmosConfig, a.Component, a.Stack, opts)
	if err != nil {
		return err
	}

	if len(deps) == 0 {
		a.Dependents = []schema.Dependent{}
		return nil
	}

	if opts.needsDependentMetadata() {
		attachDependentMetadata(deps, opts.stacks)
	}
	a.Dependents = deps

	return addDependentsToDependents(atmosConfig, &deps, opts)
}

// describeDependentsFor returns the direct dependents of a component from the pre-computed stacks and index.
func describeDependentsFor(
	atmosConfig *schema.AtmosConfiguration,
	component string,
	stack string,
	opts *dependentsOptions,
) ([]schema.Dependent, error) {
	return ExecuteDescribeDependents(
		atmosConfig,
		&DescribeDependentsArgs{
			Component:            component,
			Stack:                stack,
			IncludeSettings:      opts.IncludeSettings,
			ProcessTemplates:     opts.ProcessTemplates,
			ProcessYamlFunctions: opts.ProcessYamlFunctions,
			Skip:                 opts.Skip,
			OnlyInStack:          opts.OnlyInStack,
			Stacks:               opts.stacks,
			DepIndex:             opts.depIdx,
		},
	)
}

// addDependentsToDependents recursively adds dependent components and stacks to each dependent component.
// The stacks and depIdx parameters are pre-computed and shared across all calls.
func addDependentsToDependents(
	atmosConfig *schema.AtmosConfiguration,
	dependents *[]schema.Dependent,
	opts *dependentsOptions,
) error {
	for i := 0; i < len(*dependents); i++ {
		d := &(*dependents)[i]

		deps, err := describeDependentsFor(atmosConfig, d.Component, d.Stack, opts)
		if err != nil {
			return err
		}

		if len(deps) == 0 {
			d.Dependents = []schema.Dependent{}
			continue
		}

		if opts.needsDependentMetadata() {
			attachDependentMetadata(deps, opts.stacks)
		}
		d.Dependents = deps

		if err := addDependentsToDependents(atmosConfig, &deps, opts); err != nil {
			return err
		}
	}

	return nil
}

// attachDependentMetadata records each dependent's `metadata` section (looked up from the already-resolved
// stacks) so the `--tags` / `--labels` selectors can be applied to dependents afterward.
// A dependent that cannot be found, or has no metadata section, keeps a nil Metadata.
func attachDependentMetadata(dependents []schema.Dependent, stacks map[string]any) {
	for i := range dependents {
		componentSection, _ := findComponentSectionInCachedStacksWithType(stacks, dependents[i].Stack, dependents[i].Component)
		if metadataSection, ok := componentSection[sectionNameMetadata].(map[string]any); ok {
			dependents[i].Metadata = metadataSection
		}
	}
}

// filterAffectedDependents applies the `--tags` / `--labels` selectors to the (nested) dependents of every
// affected component and then recomputes `included_in_dependents` for the pruned trees.
// It does nothing when the filter has no selectors, so `ExcludeLocked` never changes dependents.
func filterAffectedDependents(affected *[]schema.Affected, filter AffectedFilter) {
	if !filter.hasSelectors() {
		return
	}

	for i := range *affected {
		a := &(*affected)[i]
		a.Dependents = pruneDependentsBySelectors(a.Dependents, filter)
		clearDependentMetadata(a.Dependents)
	}

	processIncludedInDependencies(affected)
}

// maxFlattenDependentDepth bounds the walk over nested dependents, guarding against a dependency cycle
// or an extremely deep chain. It matches the limit used when `list affected` flattens dependents.
const maxFlattenDependentDepth = 100

// dependentToAffected converts a dependent into a top-level affected entry with the given reason
// (for example "dependent"). Every field the two types share is copied one to one; the transient
// Metadata that selector matching records on a dependent is never copied.
func dependentToAffected(d *schema.Dependent, reason string) schema.Affected {
	return schema.Affected{
		Component:            d.Component,
		ComponentType:        d.ComponentType,
		ComponentPath:        d.ComponentPath,
		Namespace:            d.Namespace,
		Tenant:               d.Tenant,
		Environment:          d.Environment,
		Stage:                d.Stage,
		Stack:                d.Stack,
		StackSlug:            d.StackSlug,
		SpaceliftStack:       d.SpaceliftStack,
		AtlantisProject:      d.AtlantisProject,
		Affected:             reason,
		AffectedAll:          []string{reason},
		Dependents:           d.Dependents,
		IncludedInDependents: d.IncludedInDependents,
		Settings:             d.Settings,
	}
}

// affectedKey identifies a component instance in the affected list: the same
// (component, stack, component type) triple that appendToAffected de-duplicates on.
func affectedKey(component, stack, componentType string) string {
	return componentType + "\x00" + stack + "\x00" + component
}

// topLevelAffectedMetadata returns the `metadata` section of a top-level affected component as it is in
// HEAD (looked up from the already-resolved stacks), or nil when the component or its metadata is missing.
func topLevelAffectedMetadata(a *schema.Affected, stacks map[string]any) map[string]any {
	componentSection := findComponentSectionInCachedStacksByType(stacks, a.Stack, a.Component, a.ComponentType)
	metadataSection, _ := componentSection[sectionNameMetadata].(map[string]any)
	return metadataSection
}

// applySelectorsToAffectedForest applies the `--tags` / `--labels` selectors to the whole affected forest,
// after the dependents are resolved: the live affected components were not filtered while they were
// computed (see AffectedFilter.DeferSelectors), so a component that fails the selectors can still
// contribute the dependents that pass them.
//
//   - Every item's nested dependents are pruned by the selectors.
//   - A top-level item is kept when it is deleted (deleted items were already matched against their BASE
//     metadata) or when its HEAD metadata satisfies the selectors. A live item that is missing from the
//     stacks, or has no metadata, cannot match and is dropped.
//   - A dropped item's matching dependents are promoted into the top-level list at its position, with the
//     reason "dependent", unless the same component is already a top-level item or was already promoted.
//
// `included_in_dependents` is recomputed for the result, and the transient dependent metadata is cleared.
// With `--exclude-locked`, a locked dependent is never promoted. Without selectors the input is returned
// unchanged.
func applySelectorsToAffectedForest(affected []schema.Affected, filter AffectedFilter, stacks map[string]any) []schema.Affected {
	out := applySelectorsToAffectedForestKeepMetadata(affected, filter, stacks)
	clearAffectedDependentMetadata(out)
	return out
}

// applySelectorsToAffectedForestKeepMetadata is applySelectorsToAffectedForest without the final metadata
// clearing. The dependents keep the metadata recorded for selector matching, so a later step (flattening
// with `--exclude-locked`) can still tell which dependents are locked. The caller must clear it afterwards
// with clearAffectedDependentMetadata.
func applySelectorsToAffectedForestKeepMetadata(affected []schema.Affected, filter AffectedFilter, stacks map[string]any) []schema.Affected {
	if !filter.hasSelectors() {
		return affected
	}

	keep := make([]bool, len(affected))
	seen := make(map[string]struct{}, len(affected))
	for i := range affected {
		a := &affected[i]
		a.Dependents = pruneDependentsBySelectors(a.Dependents, filter)
		keep[i] = a.Deleted || filter.matchesSelectors(topLevelAffectedMetadata(a, stacks))
		if keep[i] {
			seen[affectedKey(a.Component, a.Stack, a.ComponentType)] = struct{}{}
		}
	}

	out := make([]schema.Affected, 0, len(affected))
	for i := range affected {
		if keep[i] {
			out = append(out, affected[i])
			continue
		}
		out = appendPromotedDependents(out, affected[i].Dependents, filter, seen)
	}

	processIncludedInDependencies(&out)

	return out
}

// clearAffectedDependentMetadata drops the transient dependent metadata from every affected item's
// (nested) dependents.
func clearAffectedDependentMetadata(affected []schema.Affected) {
	for i := range affected {
		clearDependentMetadata(affected[i].Dependents)
	}
}

// appendPromotedDependents appends the dependents of a dropped top-level item to out as top-level entries
// with the reason "dependent", skipping components already in seen. A promoted dependent becomes a top-level
// entry, which is what reaches the matrix, so `--exclude-locked` drops a locked one here even though nested
// dependents are left alone.
func appendPromotedDependents(out []schema.Affected, deps []schema.Dependent, filter AffectedFilter, seen map[string]struct{}) []schema.Affected {
	for j := range deps {
		d := &deps[j]
		if filter.isExcludedLockedDependent(d) {
			continue
		}
		key := affectedKey(d.Component, d.Stack, d.ComponentType)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, dependentToAffected(d, affectedReasonDependent))
	}
	return out
}

// flattenAffectedDependents lifts every dependent into the top-level affected list as an entry with the
// reason "dependent". Each top-level item keeps its position and loses its nested dependents; its
// dependents follow it in pre-order. A component that is already a top-level item, or was already lifted
// from another parent, appears only once. Deleted items pass through untouched (they have no dependents).
// With `--exclude-locked`, a locked dependent is not lifted, but its own dependents still are (mirroring how
// pruning promotes the children of a removed dependent). This needs the dependent metadata to have been
// recorded. The result is never nil.
func flattenAffectedDependents(affected []schema.Affected, filter AffectedFilter) []schema.Affected {
	seen := make(map[string]struct{}, len(affected))
	for i := range affected {
		a := &affected[i]
		seen[affectedKey(a.Component, a.Stack, a.ComponentType)] = struct{}{}
	}

	out := make([]schema.Affected, 0, len(affected))
	for i := range affected {
		a := affected[i]
		if a.Deleted {
			out = append(out, a)
			continue
		}

		dependents := a.Dependents
		a.Dependents = []schema.Dependent{}
		a.IncludedInDependents = false
		out = append(out, a)
		out = appendFlattenedDependents(out, dependents, seen, filter, 1)
	}

	return out
}

// appendFlattenedDependents appends the dependents (pre-order) that are not in seen yet to out. The
// subtree of a dependent that was already seen is skipped, because it is reached from that earlier
// occurrence: from the top-level item that owns it, or from the parent it was first lifted from. A locked
// dependent excluded by the filter is not appended, but it is marked seen and its children are still walked.
func appendFlattenedDependents(out []schema.Affected, dependents []schema.Dependent, seen map[string]struct{}, filter AffectedFilter, depth int) []schema.Affected {
	for i := range dependents {
		d := &dependents[i]
		key := affectedKey(d.Component, d.Stack, d.ComponentType)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}

		if !filter.isExcludedLockedDependent(d) {
			lifted := dependentToAffected(d, affectedReasonDependent)
			lifted.Dependents = []schema.Dependent{}
			lifted.IncludedInDependents = false
			out = append(out, lifted)
		}

		if depth < maxFlattenDependentDepth {
			out = appendFlattenedDependents(out, d.Dependents, seen, filter, depth+1)
		}
	}
	return out
}

// clearDependentMetadata drops the transient `metadata` recorded on dependents for selector matching, so
// the pruned result has the same shape as one produced without selectors.
func clearDependentMetadata(dependents []schema.Dependent) {
	for i := range dependents {
		dependents[i].Metadata = nil
		clearDependentMetadata(dependents[i].Dependents)
	}
}

// pruneDependentsBySelectors removes the dependents whose metadata does not satisfy the selectors.
// Dependents are hierarchical, so a dependent that is removed does not hide its own matching dependents:
// those are promoted to the removed dependent's parent (de-duplicated by stack slug) rather than lost.
// A dependent without a metadata section cannot match a selector and is removed.
func pruneDependentsBySelectors(dependents []schema.Dependent, filter AffectedFilter) []schema.Dependent {
	if len(dependents) == 0 {
		return dependents
	}

	kept := make([]schema.Dependent, 0, len(dependents))
	seen := make(map[string]struct{}, len(dependents))
	appendUnique := func(d schema.Dependent) {
		if _, dup := seen[d.StackSlug]; dup {
			return
		}
		seen[d.StackSlug] = struct{}{}
		kept = append(kept, d)
	}

	for i := range dependents {
		d := dependents[i]
		d.Dependents = pruneDependentsBySelectors(d.Dependents, filter)
		if filter.matchesSelectors(d.Metadata) {
			appendUnique(d)
			continue
		}
		for j := range d.Dependents {
			appendUnique(d.Dependents[j])
		}
	}

	sortDependentsByStackSlug(kept)
	return kept
}

func processIncludedInDependencies(affected *[]schema.Affected) {
	for i := 0; i < len(*affected); i++ {
		a := &(*affected)[i]
		a.IncludedInDependents = processIncludedInDependenciesForAffected(affected, a.StackSlug, i)
		if !a.IncludedInDependents {
			processPeerDependencies(&a.Dependents)
		}
	}
}

func processIncludedInDependenciesForAffected(affected *[]schema.Affected, stackSlug string, affectedIndex int) bool {
	for i := 0; i < len(*affected); i++ {
		if i == affectedIndex {
			continue
		}

		a := &(*affected)[i]

		if len(a.Dependents) > 0 {
			includedInDeps := processIncludedInDependenciesForDependents(&a.Dependents, stackSlug)
			if includedInDeps {
				return true
			}
		}
	}
	return false
}

func processIncludedInDependenciesForDependents(dependents *[]schema.Dependent, stackSlug string) bool {
	for i := 0; i < len(*dependents); i++ {
		d := &(*dependents)[i]

		if d.StackSlug == stackSlug {
			return true
		}

		if len(d.Dependents) > 0 {
			includedInDeps := processIncludedInDependenciesForDependents(&d.Dependents, stackSlug)
			if includedInDeps {
				return true
			}
		}
	}
	return false
}

func processPeerDependencies(dependents *[]schema.Dependent) {
	for i := 0; i < len(*dependents); i++ {
		d := &(*dependents)[i]
		d.IncludedInDependents = processIncludedInDependenciesForPeerDependencies(dependents, d.StackSlug, i)
		processPeerDependencies(&d.Dependents)
	}
}

func processIncludedInDependenciesForPeerDependencies(dependents *[]schema.Dependent, stackSlug string, depIndex int) bool {
	for i := 0; i < len(*dependents); i++ {
		if i == depIndex {
			continue
		}

		d := &(*dependents)[i]

		if d.StackSlug == stackSlug {
			return true
		}

		if len(d.Dependents) > 0 {
			includedInDeps := processIncludedInDependenciesForPeerDependencies(&d.Dependents, stackSlug, -1)
			if includedInDeps {
				return true
			}
		}
	}
	return false
}
