package exec

import (
	"fmt"
	"sort"

	"github.com/go-viper/mapstructure/v2"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/list/dependencies"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/tags"
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
	AuthDisabled         bool             // True when --identity=false (or alias) explicitly disables authentication; forwarded to DescribeDependentsArgs.
	ErrorMode            string           // How to handle recoverable errors: "strict" (default), "warn", or "silent".
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
	AuthDisabled         bool             // True when --identity=false (or alias) explicitly disables authentication; routes inner stack resolution to ExecuteDescribeStacksWithAuthDisabled.
	// Stacks is an optional pre-computed result from ExecuteDescribeStacks.
	// When provided, ExecuteDescribeDependents skips the expensive stack resolution
	// and uses this cached result instead. This avoids O(N) full stack resolutions
	// when computing dependents for N affected components.
	Stacks map[string]any
	// DepIndex is an optional pre-computed reverse dependency index.
	// When provided, ExecuteDescribeDependents skips the O(all_stacks × all_components)
	// scan and uses the index for O(1) lookup per component name.
	DepIndex dependencyIndex
	// ErrOptions configures graceful degradation for the internal stack resolution when
	// Stacks is not pre-computed. The zero value (OnErrorStrict) matches the historical
	// fail-fast behavior.
	ErrOptions    DescribeStacksErrorOptions
	componentType string
	leftDelim     string
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

	errOptions, collector := ErrorOptionsFromMode(describeDependentsExecProps.ErrorMode)

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
			AuthDisabled:         describeDependentsExecProps.AuthDisabled,
			ErrOptions:           errOptions,
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

	if err := viewWithScroll(&viewWithScrollProps{
		atmosConfig:           d.atmosConfig,
		format:                describeDependentsExecProps.Format,
		file:                  describeDependentsExecProps.File,
		res:                   res,
		pageCreator:           d.newPageCreator,
		isTTYSupportForStdout: d.isTTYSupportForStdout,
		displayName:           fmt.Sprintf("Dependents of '%s' in stack '%s'", describeDependentsExecProps.Component, describeDependentsExecProps.Stack),
		printOrWriteToFile:    printOrWriteToFile,
	}); err != nil {
		return err
	}

	PrintErrorModeSummary(describeDependentsExecProps.ErrorMode, collector)
	return nil
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
	args.leftDelim, _ = tags.TemplateDelims(atmosConfig.Templates.Settings.Delimiters)

	dependents := []schema.Dependent{}
	var ok bool

	// Use pre-computed stacks if provided (avoids redundant full stack resolution
	// when called in a loop from addDependentsToAffected).
	stacks := args.Stacks
	if stacks == nil {
		var err error
		if shouldScopeDescribeDependents(atmosConfig, args) {
			stacks, err = resolveScopedDependentStacks(atmosConfig, args)
		} else {
			stacks, err = ExecuteDescribeStacksWithOptions(
				atmosConfig, args.OnlyInStack, nil, nil, nil, false,
				args.ProcessTemplates, args.ProcessYamlFunctions, false, args.Skip,
				args.AuthManager, args.AuthDisabled, args.ErrOptions,
			)
		}
		if err != nil {
			return nil, err
		}
	}

	// Get the provided component section.
	// When stacks are cached, extract directly from the cache to avoid redundant stack resolution.
	providedComponentSection, componentType := findComponentSectionInCachedStacksWithType(stacks, args.Stack, args.Component)
	args.componentType = componentType
	targetUnavailable := providedComponentSection == nil
	if targetUnavailable {
		skip, err := skipUnavailableOptionalTarget(stacks, args)
		if err != nil {
			return nil, err
		}
		if skip {
			return dependents, nil
		}
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
			AuthDisabled:         args.AuthDisabled,
			ErrorOptions:         args.ErrOptions,
		})
		if err != nil {
			return nil, err
		}
	}
	targetUnavailable = isAbstractOrDisabled(providedComponentSection, args.Component)

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
		var err error
		dependents, err = findDependentsFromIndexWithStacks(atmosConfig, args, &providedComponentVars, targetUnavailable, stacks)
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		dependents, err = findDependentsByScan(atmosConfig, args, stacks, &providedComponentVars, targetUnavailable)
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

func shouldScopeDescribeDependents(atmosConfig *schema.AtmosConfiguration, args *DescribeDependentsArgs) bool {
	return args.OnlyInStack == "" && (args.ProcessTemplates || args.ProcessYamlFunctions) && !GetEagerEvaluationSetting(atmosConfig)
}

func resolveScopedDependentStacks(atmosConfig *schema.AtmosConfiguration, args *DescribeDependentsArgs) (map[string]any, error) {
	leftDelim, rightDelim := tags.TemplateDelims(atmosConfig.Templates.Settings.Delimiters)
	result, err := dependencies.ResolveScopedClosure(
		func(stack string, components []string, processTemplates, processFunctions bool) (map[string]any, error) {
			return ExecuteDescribeStacksWithOptions(
				atmosConfig, stack, components, nil, nil, false, processTemplates, processFunctions,
				false, args.Skip, args.AuthManager, args.AuthDisabled, args.ErrOptions,
			)
		},
		&dependencies.ScopeRequest{
			Components:                    []string{args.Component},
			Stack:                         args.Stack,
			Direction:                     dependencies.DirectionReverse,
			ProcessTemplates:              args.ProcessTemplates,
			ProcessFunctions:              args.ProcessYamlFunctions,
			LeftDelim:                     leftDelim,
			RightDelim:                    rightDelim,
			SkipTargetValidation:          true,
			IncludeLegacyReverseSources:   true,
			IncludeRequiredReverseSources: true,
		},
	)
	if err != nil {
		return nil, err
	}
	return result.Stacks, nil
}

func skipUnavailableOptionalTarget(stacks map[string]any, args *DescribeDependentsArgs) (bool, error) {
	depIndex := args.DepIndex
	if depIndex == nil {
		var err error
		depIndex, err = buildDependencyIndexWithError(stacks, args.leftDelim)
		if err != nil {
			return false, err
		}
	}
	return onlyOptionalTargetDependencies(depIndex[args.Component], args.Stack), nil
}

func onlyOptionalTargetDependencies(entries []dependencyIndexEntry, stackName string) bool {
	hasMatching := false
	for i := range entries {
		if !dependencyTargetsStack(&entries[i], stackName) {
			continue
		}
		hasMatching = true
		if entries[i].DependsOn.IsRequired() {
			return false
		}
	}
	return hasMatching
}

func dependencyTargetsStack(entry *dependencyIndexEntry, stackName string) bool {
	return dependencyTargetsStackValues(&entry.DependsOn, entry.StackName, stackName)
}

func dependencyTargetsStackValues(dep *schema.ComponentDependency, sourceStack, stackName string) bool {
	if stackName == "" {
		return true
	}
	targetStack := dep.Stack
	if targetStack == "" {
		targetStack = sourceStack
	}
	return targetStack == stackName
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
// non-tool dependency entries. Tools alone is intentionally excluded because
// file/folder and component dependency extraction paths do not return tool
// dependencies.
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
	component, _ := findComponentSectionInCachedStacksWithType(stacks, stackName, componentName)
	return component
}

func findComponentSectionInCachedStacksWithType(stacks map[string]any, stackName, componentName string) (map[string]any, string) {
	stackSection, ok := stacks[stackName].(map[string]any)
	if !ok {
		return nil, ""
	}
	componentsSection, ok := stackSection["components"].(map[string]any)
	if !ok {
		return nil, ""
	}
	componentTypes := []string{
		cfg.TerraformComponentType,
		cfg.HelmfileComponentType,
		cfg.PackerComponentType,
		cfg.AnsibleComponentType,
		cfg.ContainerComponentType,
		cfg.EmulatorComponentType,
		cfg.KubernetesComponentType,
		cfg.HelmComponentType,
	}
	// Match the same configured component-type precedence as describe component.
	// Unknown component types are considered afterward in lexical order.
	for _, componentType := range componentTypes {
		if comp := findComponentSectionInCachedStacksByType(stacks, stackName, componentName, componentType); comp != nil {
			return comp, componentType
		}
	}
	knownTypes := make(map[string]struct{}, len(componentTypes))
	for _, componentType := range componentTypes {
		knownTypes[componentType] = struct{}{}
	}
	remainingTypes := make([]string, 0, len(componentsSection))
	for componentType := range componentsSection {
		if _, ok := knownTypes[componentType]; !ok {
			remainingTypes = append(remainingTypes, componentType)
		}
	}
	sort.Strings(remainingTypes)
	for _, componentType := range remainingTypes {
		if comp := findComponentSectionInCachedStacksByType(stacks, stackName, componentName, componentType); comp != nil {
			return comp, componentType
		}
	}
	return nil, ""
}

func findComponentSectionInCachedStacksByType(stacks map[string]any, stackName, componentName, componentType string) map[string]any {
	stackSection, ok := stacks[stackName].(map[string]any)
	if !ok {
		return nil
	}
	componentsSection, ok := stackSection["components"].(map[string]any)
	if !ok {
		return nil
	}
	componentTypeMap, ok := componentsSection[componentType].(map[string]any)
	if !ok {
		return nil
	}
	component, _ := componentTypeMap[componentName].(map[string]any)
	return component
}
