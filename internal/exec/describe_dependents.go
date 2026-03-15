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

	// Get all stacks with all components, filtered by onlyInStack if provided.
	stacks, err := ExecuteDescribeStacks(
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
		args.AuthManager, // AuthManager passed from describe dependents command layer
	)
	if err != nil {
		return nil, err
	}

	providedComponentSection, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
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

	// Get the provided component `vars`.
	var providedComponentVarsSection map[string]any
	if providedComponentVarsSection, ok = providedComponentSection["vars"].(map[string]any); !ok {
		return dependents, nil
	}

	// Convert the provided component `vars` section to the `Context` structure.
	var providedComponentVars schema.Context
	err = mapstructure.Decode(providedComponentVarsSection, &providedComponentVars)
	if err != nil {
		return nil, err
	}

	// Iterate over all stacks and all components in the stacks.
	for stackName, stackSection := range stacks {
		var stackSectionMap map[string]any
		if stackSectionMap, ok = stackSection.(map[string]any); !ok {
			continue
		}

		// Get the stack `components` section.
		var stackComponentsSection map[string]any
		if stackComponentsSection, ok = stackSectionMap["components"].(map[string]any); !ok {
			continue
		}

		for stackComponentType, stackComponentTypeSection := range stackComponentsSection {
			var stackComponentTypeSectionMap map[string]any
			if stackComponentTypeSectionMap, ok = stackComponentTypeSection.(map[string]any); !ok {
				continue
			}

			for stackComponentName, stackComponent := range stackComponentTypeSectionMap {
				var stackComponentMap map[string]any
				if stackComponentMap, ok = stackComponent.(map[string]any); !ok {
					continue
				}

				// Skip the stack component if it's the same as the provided component.
				if stackComponentName == args.Component {
					continue
				}

				// Skip abstract and disabled components.
				if metadataSection, ok := stackComponentMap["metadata"].(map[string]any); ok {
					if metadataType, ok := metadataSection["type"].(string); ok {
						if metadataType == "abstract" {
							continue
						}
					}
					if !isComponentEnabled(metadataSection, stackComponentName) {
						continue
					}
				}

				// Get the stack component `vars`.
				var stackComponentVarsSection map[string]any
				if stackComponentVarsSection, ok = stackComponentMap["vars"].(map[string]any); !ok {
					continue
				}

				// Convert the stack component `vars` section to the `Context` structure.
				var stackComponentVars schema.Context
				err = mapstructure.Decode(stackComponentVarsSection, &stackComponentVars)
				if err != nil {
					return nil, err
				}

				// Get component dependencies from dependencies.components (preferred) or settings.depends_on (legacy).
				componentDeps, settingsSection, depSource := getComponentDependencies(stackComponentMap)
				if len(componentDeps) == 0 {
					continue
				}

				// Check if the stack component is a dependent of the provided component.
				for depIdx := range componentDeps {
					dependsOn := &componentDeps[depIdx]
					if dependsOn.Component != args.Component {
						continue
					}

					// Matching logic depends on the source format.
					if !isDependencyMatch(&dependencyMatchParams{
						depSource:             depSource,
						dependsOn:             dependsOn,
						args:                  args,
						stackName:             stackName,
						providedComponentVars: &providedComponentVars,
						stackComponentVars:    &stackComponentVars,
					}) {
						continue
					}

					dependent := schema.Dependent{
						Component:     stackComponentName,
						ComponentPath: BuildComponentPath(atmosConfig, &stackComponentMap, stackComponentType),
						ComponentType: stackComponentType,
						Stack:         stackName,
						StackSlug:     fmt.Sprintf("%s-%s", stackName, strings.Replace(stackComponentName, "/", "-", -1)),
						Namespace:     stackComponentVars.Namespace,
						Tenant:        stackComponentVars.Tenant,
						Environment:   stackComponentVars.Environment,
						Stage:         stackComponentVars.Stage,
					}

					// Add Spacelift stack and Atlantis project if they are configured for the dependent stack component.
					if stackComponentType == "terraform" {
						// Spacelift stack.
						configAndStacksInfo := schema.ConfigAndStacksInfo{
							ComponentFromArg:         stackComponentName,
							Stack:                    stackName,
							ComponentVarsSection:     stackComponentVarsSection,
							ComponentSettingsSection: settingsSection,
							ComponentSection: map[string]any{
								cfg.VarsSectionName:     stackComponentVarsSection,
								cfg.SettingsSectionName: settingsSection,
							},
						}

						spaceliftStackName, err := BuildSpaceliftStackNameFromComponentConfig(atmosConfig, configAndStacksInfo)
						if err != nil {
							return nil, err
						}
						dependent.SpaceliftStack = spaceliftStackName

						// Atlantis project.
						atlantisProjectName, err := BuildAtlantisProjectNameFromComponentConfig(atmosConfig, configAndStacksInfo)
						if err != nil {
							return nil, err
						}
						dependent.AtlantisProject = atlantisProjectName
					}

					if args.IncludeSettings {
						dependent.Settings = settingsSection
					}

					dependents = append(dependents, dependent)
				}
			}
		}
	}

	sortDependentsByStackSlugRecursive(dependents)
	return dependents, nil
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

	// Check dependencies.components first (preferred location).
	if depsSection, ok := componentMap[cfg.DependenciesSectionName].(map[string]any); ok {
		if _, hasComponents := depsSection["components"]; hasComponents {
			var deps schema.Dependencies
			if err := mapstructure.Decode(depsSection, &deps); err == nil && len(deps.Components) > 0 {
				return deps.Components, settingsSection, dependencySourceDependenciesComponents
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
