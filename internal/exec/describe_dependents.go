package exec

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/go-viper/mapstructure/v2"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

type DescribeDependentsExecProps struct {
	File                 string
	Format               string
	Query                string
	Stack                string
	Component            string
	IncludeSettings      bool
	ProcessTemplates     bool
	ProcessYamlFunctions bool
	Skip                 []string
	AuthManager          auth.AuthManager // Optional: Auth manager for credential management (from --identity flag).
}

// DescribeDependentsArgs holds arguments for ExecuteDescribeDependents.
type DescribeDependentsArgs struct {
	Component            string
	Stack                string
	IncludeSettings      bool
	ProcessTemplates     bool
	ProcessYamlFunctions bool
	Skip                 []string
	OnlyInStack          string
	AuthManager          auth.AuthManager // Optional: Auth manager for credential management (from --identity flag).
	// Stacks is an optional pre-computed result from ExecuteDescribeStacks.
	// When provided, ExecuteDescribeDependents skips the expensive stack resolution
	// and uses this cached result instead. This avoids O(N) full stack resolutions
	// when computing dependents for N affected components.
	Stacks map[string]any
	// DepIndex is an optional pre-computed reverse dependency index.
	// When provided, ExecuteDescribeDependents skips the O(all_stacks × all_components)
	// scan and uses the index for O(1) lookup per component name.
	DepIndex dependencyIndex
}

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type DescribeDependentsExec interface {
	Execute(describeDependentsExecProps *DescribeDependentsExecProps) error
}

type describeDependentsExec struct {
	atmosConfig               *schema.AtmosConfiguration
	executeDescribeDependents func(
		atmosConfig *schema.AtmosConfiguration,
		args *DescribeDependentsArgs,
	) ([]schema.Dependent, error)
	newPageCreator        pager.PageCreator
	isTTYSupportForStdout func() bool
	evaluateYqExpression  func(
		atmosConfig *schema.AtmosConfiguration,
		data any,
		yq string,
	) (any, error)
}

// NewDescribeDependentsExec creates a new `describe dependents` executor.
func NewDescribeDependentsExec(atmosConfig *schema.AtmosConfiguration) DescribeDependentsExec {
	defer perf.Track(atmosConfig, "exec.NewDescribeDependentsExec")()

	return &describeDependentsExec{
		executeDescribeDependents: ExecuteDescribeDependents,
		newPageCreator:            pager.New(),
		isTTYSupportForStdout:     term.IsTTYSupportForStdout,
		atmosConfig:               atmosConfig,
		evaluateYqExpression:      u.EvaluateYqExpression,
	}
}

func (d *describeDependentsExec) Execute(describeDependentsExecProps *DescribeDependentsExecProps) error {
	defer perf.Track(nil, "exec.Execute")()

	dependents, err := d.executeDescribeDependents(
		d.atmosConfig,
		&DescribeDependentsArgs{
			Component:            describeDependentsExecProps.Component,
			Stack:                describeDependentsExecProps.Stack,
			IncludeSettings:      describeDependentsExecProps.IncludeSettings,
			ProcessTemplates:     describeDependentsExecProps.ProcessTemplates,
			ProcessYamlFunctions: describeDependentsExecProps.ProcessYamlFunctions,
			Skip:                 describeDependentsExecProps.Skip,
			OnlyInStack:          "", // empty string means process all stacks for direct CLI usage
			AuthManager:          describeDependentsExecProps.AuthManager,
		},
	)
	if err != nil {
		return err
	}

	var res any

	if describeDependentsExecProps.Query != "" {
		res, err = d.evaluateYqExpression(d.atmosConfig, dependents, describeDependentsExecProps.Query)
		if err != nil {
			return err
		}
	} else {
		res = dependents
	}

	return viewWithScroll(&viewWithScrollProps{
		atmosConfig:           d.atmosConfig,
		format:                describeDependentsExecProps.Format,
		file:                  describeDependentsExecProps.File,
		res:                   res,
		pageCreator:           d.newPageCreator,
		isTTYSupportForStdout: d.isTTYSupportForStdout,
		displayName:           fmt.Sprintf("Dependents of '%s' in stack '%s'", describeDependentsExecProps.Component, describeDependentsExecProps.Stack),
		printOrWriteToFile:    printOrWriteToFile,
	})
}

// ExecuteDescribeDependents produces a list of Atmos components in Atmos stacks that depend on the provided Atmos component.
func ExecuteDescribeDependents(
	atmosConfig *schema.AtmosConfiguration,
	args *DescribeDependentsArgs,
) ([]schema.Dependent, error) {
	defer perf.Track(atmosConfig, "exec.ExecuteDescribeDependents")()

	if atmosConfig == nil {
		return nil, errUtils.ErrAtmosConfigIsNil
	}

	dependents := []schema.Dependent{}
	var ok bool

	// Use pre-computed stacks if provided (avoids redundant full stack resolution
	// when called in a loop from addDependentsToAffected).
	stacks := args.Stacks
	if stacks == nil {
		var err error
		stacks, err = ExecuteDescribeStacks(
			atmosConfig,
			args.OnlyInStack,
			nil,
			nil,
			nil,
			false,
			args.ProcessTemplates,
			args.ProcessYamlFunctions,
			false,
			args.Skip,
			args.AuthManager,
		)
		if err != nil {
			return nil, err
		}
	}

	// Get the provided component section.
	// When stacks are cached, extract directly from the cache to avoid redundant stack resolution.
	var providedComponentSection map[string]any
	if args.Stacks != nil {
		providedComponentSection = findComponentSectionInCachedStacks(stacks, args.Stack, args.Component)
	}
	if providedComponentSection == nil {
		var err error
		providedComponentSection, err = ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
			Component:            args.Component,
			Stack:                args.Stack,
			ProcessTemplates:     args.ProcessTemplates,
			ProcessYamlFunctions: args.ProcessYamlFunctions,
			Skip:                 args.Skip,
			AuthManager:          args.AuthManager,
		})
		if err != nil {
			return nil, err
		}
	}

	// Get the provided component `vars`.
	var providedComponentVarsSection map[string]any
	if providedComponentVarsSection, ok = providedComponentSection["vars"].(map[string]any); !ok {
		return dependents, nil
	}

	// Convert the provided component `vars` section to the `Context` structure.
	var providedComponentVars schema.Context
	if err := mapstructure.Decode(providedComponentVarsSection, &providedComponentVars); err != nil {
		return nil, err
	}

	// Find all components that depend on the provided component.
	// When a pre-computed dependency index is available, use O(1) lookup.
	// Otherwise, fall back to the full O(stacks × components) scan.
	if args.DepIndex != nil {
		dependents = findDependentsFromIndex(atmosConfig, args, &providedComponentVars)
	} else {
		var err error
		dependents, err = findDependentsByScan(atmosConfig, args, stacks, &providedComponentVars)
		if err != nil {
			return nil, err
		}
	}
	if dependents == nil {
		dependents = []schema.Dependent{}
	}

	sortDependentsByStackSlugRecursive(dependents)
	return dependents, nil
}

// findDependentsFromIndex uses the pre-computed dependency index for O(1) lookup.
func findDependentsFromIndex(
	atmosConfig *schema.AtmosConfiguration,
	args *DescribeDependentsArgs,
	providedComponentVars *schema.Context,
) []schema.Dependent {
	var dependents []schema.Dependent

	entries := args.DepIndex[args.Component]
	for i := range entries {
		e := &entries[i]

		// Skip self-references.
		if e.StackComponentName == args.Component {
			continue
		}

		dep := e.DependsOn
		if !isDependencyMatch(&dependencyMatchParams{
			depSource:             e.DepSource,
			dependsOn:             &dep,
			args:                  args,
			stackName:             e.StackName,
			providedComponentVars: providedComponentVars,
			stackComponentVars:    &e.StackComponentVars,
		}) {
			continue
		}

		dependent := buildDependentEntry(atmosConfig, args, e)
		dependents = append(dependents, dependent)
	}

	return dependents
}

// findDependentsByScan falls back to the full O(stacks * components) scan.
func findDependentsByScan(
	atmosConfig *schema.AtmosConfiguration,
	args *DescribeDependentsArgs,
	stacks map[string]any,
	providedComponentVars *schema.Context,
) ([]schema.Dependent, error) {
	var dependents []schema.Dependent

	for stackName, stackSection := range stacks {
		stackSectionMap, ok := stackSection.(map[string]any)
		if !ok {
			continue
		}
		stackComponentsSection, ok := stackSectionMap["components"].(map[string]any)
		if !ok {
			continue
		}

		for stackComponentType, stackComponentTypeSection := range stackComponentsSection {
			stackComponentTypeSectionMap, ok := stackComponentTypeSection.(map[string]any)
			if !ok {
				continue
			}

			for stackComponentName, stackComponent := range stackComponentTypeSectionMap {
				deps, err := scanComponentForDependents(&scanComponentParams{
					AtmosConfig:           atmosConfig,
					Args:                  args,
					StackName:             stackName,
					StackComponentType:    stackComponentType,
					StackComponentName:    stackComponentName,
					StackComponent:        stackComponent,
					ProvidedComponentVars: providedComponentVars,
				})
				if err != nil {
					return nil, err
				}
				dependents = append(dependents, deps...)
			}
		}
	}

	return dependents, nil
}

// scanComponentParams groups parameters for scanComponentForDependents.
type scanComponentParams struct {
	AtmosConfig           *schema.AtmosConfiguration
	Args                  *DescribeDependentsArgs
	StackName             string
	StackComponentType    string
	StackComponentName    string
	StackComponent        any
	ProvidedComponentVars *schema.Context
}

// scanComponentForDependents checks a single component for dependencies on the provided component.
func scanComponentForDependents(p *scanComponentParams) ([]schema.Dependent, error) {
	stackComponentMap, ok := p.StackComponent.(map[string]any)
	if !ok {
		return nil, nil
	}

	if p.StackComponentName == p.Args.Component {
		return nil, nil
	}

	if isAbstractOrDisabled(stackComponentMap, p.StackComponentName) {
		return nil, nil
	}

	stackComponentVarsSection, ok := stackComponentMap["vars"].(map[string]any)
	if !ok {
		return nil, nil
	}

	var stackComponentVars schema.Context
	if err := mapstructure.Decode(stackComponentVarsSection, &stackComponentVars); err != nil {
		return nil, err
	}

	componentDeps, settingsSection, depSource := getComponentDependencies(stackComponentMap)
	if len(componentDeps) == 0 {
		return nil, nil
	}

	var dependents []schema.Dependent
	for depIdx := range componentDeps {
		dependsOn := &componentDeps[depIdx]
		if dependsOn.Component != p.Args.Component {
			continue
		}

		if !isDependencyMatch(&dependencyMatchParams{
			depSource:             depSource,
			dependsOn:             dependsOn,
			args:                  p.Args,
			stackName:             p.StackName,
			providedComponentVars: p.ProvidedComponentVars,
			stackComponentVars:    &stackComponentVars,
		}) {
			continue
		}

		e := &dependencyIndexEntry{
			StackName:                 p.StackName,
			StackComponentName:        p.StackComponentName,
			StackComponentType:        p.StackComponentType,
			StackComponentMap:         stackComponentMap,
			StackComponentVarsSection: stackComponentVarsSection,
			StackComponentVars:        stackComponentVars,
			SettingsSection:           settingsSection,
		}
		dependents = append(dependents, buildDependentEntry(p.AtmosConfig, p.Args, e))
	}

	return dependents, nil
}

// buildDependentEntry constructs a Dependent struct from a dependency index entry.
func buildDependentEntry(
	atmosConfig *schema.AtmosConfiguration,
	args *DescribeDependentsArgs,
	e *dependencyIndexEntry,
) schema.Dependent {
	dependent := schema.Dependent{
		Component:     e.StackComponentName,
		ComponentPath: BuildComponentPath(atmosConfig, &e.StackComponentMap, e.StackComponentType),
		ComponentType: e.StackComponentType,
		Stack:         e.StackName,
		StackSlug:     fmt.Sprintf("%s-%s", e.StackName, strings.ReplaceAll(e.StackComponentName, "/", "-")),
		Namespace:     e.StackComponentVars.Namespace,
		Tenant:        e.StackComponentVars.Tenant,
		Environment:   e.StackComponentVars.Environment,
		Stage:         e.StackComponentVars.Stage,
	}

	if e.StackComponentType == "terraform" {
		configAndStacksInfo := schema.ConfigAndStacksInfo{
			ComponentFromArg:         e.StackComponentName,
			Stack:                    e.StackName,
			ComponentVarsSection:     e.StackComponentVarsSection,
			ComponentSettingsSection: e.SettingsSection,
			ComponentSection: map[string]any{
				cfg.VarsSectionName:     e.StackComponentVarsSection,
				cfg.SettingsSectionName: e.SettingsSection,
			},
		}

		if spaceliftStackName, err := BuildSpaceliftStackNameFromComponentConfig(atmosConfig, configAndStacksInfo); err == nil {
			dependent.SpaceliftStack = spaceliftStackName
		}
		if atlantisProjectName, err := BuildAtlantisProjectNameFromComponentConfig(atmosConfig, configAndStacksInfo); err == nil {
			dependent.AtlantisProject = atlantisProjectName
		}
	}

	if args.IncludeSettings {
		dependent.Settings = e.SettingsSection
	}

	return dependent
}

// sortDependentsByStackSlug sorts the dependents by stack slug.
func sortDependentsByStackSlug(deps []schema.Dependent) {
	if len(deps) == 0 {
		return
	}
	sort.SliceStable(deps, func(i, j int) bool {
		// primary key.
		if deps[i].StackSlug != deps[j].StackSlug {
			return deps[i].StackSlug < deps[j].StackSlug
		}
		// tie-breakers to keep order stable across runs.
		if deps[i].Component != deps[j].Component {
			return deps[i].Component < deps[j].Component
		}
		return deps[i].Stack < deps[j].Stack
	})
}

// sortDependentsByStackSlugRecursive sorts the dependents by stack slug recursively.
func sortDependentsByStackSlugRecursive(deps []schema.Dependent) {
	if len(deps) == 0 {
		return
	}
	for i := range deps {
		sortDependentsByStackSlugRecursive(deps[i].Dependents)
	}
	sortDependentsByStackSlug(deps)
}

// dependencySource indicates where dependencies were loaded from.
type dependencySource int

const (
	dependencySourceNone dependencySource = iota
	dependencySourceDependenciesComponents
	dependencySourceSettingsDependsOn
)

// getComponentDependencies extracts component dependencies from a component section.
// It checks dependencies.components first (preferred), then falls back to settings.depends_on (legacy).
// Returns the list of dependencies, the settings section, and the source of the dependencies.
func getComponentDependencies(componentMap map[string]any) ([]schema.ComponentDependency, map[string]any, dependencySource) {
	// Get settings section for later use (Spacelift/Atlantis config and IncludeSettings).
	settingsSection, _ := componentMap["settings"].(map[string]any)

	// Check dependencies.* first (preferred location).
	if depsSection, ok := componentMap[cfg.DependenciesSectionName].(map[string]any); ok {
		if hasDependencyEntries(depsSection) {
			var deps schema.Dependencies
			if err := mapstructure.Decode(depsSection, &deps); err == nil {
				if normErr := deps.Normalize(); normErr != nil {
					log.Debug("failed to normalize dependencies section", "error", normErr)
				}
				if len(deps.Components) > 0 {
					return deps.Components, settingsSection, dependencySourceDependenciesComponents
				}
			}
		}
	}

	// Fall back to settings.depends_on (legacy location).
	if settingsSection != nil {
		var settings schema.Settings
		if err := mapstructure.Decode(settingsSection, &settings); err == nil {
			if !reflect.ValueOf(settings.DependsOn).IsZero() && len(settings.DependsOn) > 0 {
				log.Debug("'settings.depends_on' is deprecated, use 'dependencies.components' instead. See: https://atmos.tools/stacks/dependencies/components")
				// Convert legacy Context to ComponentDependency.
				deps := make([]schema.ComponentDependency, 0, len(settings.DependsOn))
				for key := range settings.DependsOn {
					ctx := settings.DependsOn[key]
					deps = append(deps, contextToComponentDependency(&ctx))
				}
				return deps, settingsSection, dependencySourceSettingsDependsOn
			}
		}
	}

	return nil, settingsSection, dependencySourceNone
}

// contextToComponentDependency converts a legacy schema.Context to schema.ComponentDependency.
// This is used to support the deprecated settings.depends_on format.
// All context fields are preserved for matching logic.
func contextToComponentDependency(ctx *schema.Context) schema.ComponentDependency {
	return schema.ComponentDependency{
		Component:   ctx.Component,
		Stack:       ctx.Stack,
		Namespace:   ctx.Namespace,
		Tenant:      ctx.Tenant,
		Environment: ctx.Environment,
		Stage:       ctx.Stage,
	}
}

// dependencyMatchParams groups parameters for dependency matching to stay within argument limits.
type dependencyMatchParams struct {
	depSource             dependencySource
	dependsOn             *schema.ComponentDependency
	args                  *DescribeDependentsArgs
	stackName             string
	providedComponentVars *schema.Context
	stackComponentVars    *schema.Context
}

// isDependencyMatch checks whether a dependency matches the provided component based on the source format.
// For the new format (dependencies.components), it matches on stack field only.
// For the legacy format (settings.depends_on), it preserves original context-field matching behavior.
func isDependencyMatch(p *dependencyMatchParams) bool {
	if p.depSource == dependencySourceDependenciesComponents {
		return matchNewFormatStack(p.dependsOn, p.args.Stack, p.stackName)
	}
	return matchLegacyStack(p.dependsOn, p.args.Stack, p.stackName) &&
		matchLegacyContextFields(p.dependsOn, p.providedComponentVars, p.stackComponentVars)
}

// matchNewFormatStack checks stack matching for the new dependencies.components format.
func matchNewFormatStack(dependsOn *schema.ComponentDependency, argsStack, stackName string) bool {
	if dependsOn.Stack != "" {
		return argsStack == dependsOn.Stack
	}
	return argsStack == stackName
}

// matchLegacyStack checks stack matching for the legacy settings.depends_on format.
func matchLegacyStack(dependsOn *schema.ComponentDependency, argsStack, stackName string) bool {
	if dependsOn.Stack != "" {
		return argsStack == dependsOn.Stack
	}
	// If no context fields are set, require same stack.
	if dependsOn.Namespace == "" && dependsOn.Tenant == "" &&
		dependsOn.Environment == "" && dependsOn.Stage == "" {
		return argsStack == stackName
	}
	return true
}

// matchLegacyContextFields checks context field matching for the legacy settings.depends_on format.
// If a field is specified in depends_on, compare against the provided component's context.
// If a field is NOT specified, compare the depending component's vars against the provided component's vars.
func matchLegacyContextFields(dependsOn *schema.ComponentDependency, provided, stack *schema.Context) bool {
	return matchContextField(dependsOn.Namespace, provided.Namespace, stack.Namespace) &&
		matchContextField(dependsOn.Tenant, provided.Tenant, stack.Tenant) &&
		matchContextField(dependsOn.Environment, provided.Environment, stack.Environment) &&
		matchContextField(dependsOn.Stage, provided.Stage, stack.Stage)
}

// matchContextField checks a single context field for dependency matching.
// If depValue is set, it must match providedValue. If depValue is empty, providedValue must match stackValue.
func matchContextField(depValue, providedValue, stackValue string) bool {
	if depValue != "" {
		return providedValue == depValue
	}
	return providedValue == stackValue
}

// hasDependencyEntries reports whether a `dependencies` section declares any
// of the entry-bearing keys that getComponentDependencies cares about. Tools
// alone is intentionally excluded because tool dependencies are not what this
// function returns.
func hasDependencyEntries(depsSection map[string]any) bool {
	if _, ok := depsSection["components"]; ok {
		return true
	}
	if _, ok := depsSection["files"]; ok {
		return true
	}
	if _, ok := depsSection["folders"]; ok {
		return true
	}
	return false
}

// findComponentSectionInCachedStacks extracts a component section from pre-computed stacks.
// Returns nil if the stack or component is not found (caller falls back to ExecuteDescribeComponent).
func findComponentSectionInCachedStacks(stacks map[string]any, stackName, componentName string) map[string]any {
	stackSection, ok := stacks[stackName].(map[string]any)
	if !ok {
		return nil
	}
	componentsSection, ok := stackSection["components"].(map[string]any)
	if !ok {
		return nil
	}
	// Check terraform components (the common case).
	if tfSection, ok := componentsSection["terraform"].(map[string]any); ok {
		if comp, ok := tfSection[componentName].(map[string]any); ok {
			return comp
		}
	}
	// Check helmfile components.
	if hfSection, ok := componentsSection["helmfile"].(map[string]any); ok {
		if comp, ok := hfSection[componentName].(map[string]any); ok {
			return comp
		}
	}
	return nil
}
