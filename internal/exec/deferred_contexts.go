package exec

import (
	"fmt"

	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ComponentDeferredContexts holds the per-section *merge.DeferredMergeContext produced while
// merging a single component's configuration, keyed by section name (vars, settings, env, auth,
// providers, required_providers, hooks, test, generate — see the cfg.*SectionName constants used
// as keys). Each context tracks the deferred YAML functions (!template, !terraform.output,
// !labels, etc.) collected for that section during the raw structural merge; a later,
// per-invocation stage resolves them with a real YAMLFunctionProcessor and deep-merges the result
// against any concrete override at the same path.
type ComponentDeferredContexts map[string]*m.DeferredMergeContext

// StackComponentDeferredContexts holds, for a single stack, every component's
// ComponentDeferredContexts keyed by [componentType][component] (e.g.
// ["terraform"]["vpc"]). It is the shape ProcessStackConfig produces per stack.
type StackComponentDeferredContexts map[string]map[string]ComponentDeferredContexts

// AllStacksDeferredContexts holds every stack's deferred contexts keyed by
// [stack][componentType][component]. This is the shape threaded from
// ProcessYAMLConfigFiles through FindStacksMap to the per-invocation Stage 3
// resolver (see resolveDeferredYamlFunctions).
type AllStacksDeferredContexts map[string]StackComponentDeferredContexts

// resolveDeferredYamlFunctions is Stage 3 of the deferred-merge pipeline: for each section with
// deferred YAML functions recovered from the FindStacksMap cache
// (configAndStacksInfo.DeferredMergeContexts), resolve them with a real
// TemplateAwareYAMLProcessor and deep-merge the result into configAndStacksInfo.ComponentSection,
// in place. See the call site in processStacks (utils.go) for why this must run where it does.
func resolveDeferredYamlFunctions(
	atmosConfig *schema.AtmosConfiguration,
	configAndStacksInfo *schema.ConfigAndStacksInfo,
	settingsSectionStruct *schema.Settings,
	componentTemplateContext map[string]any,
	skip []string,
) error {
	defer perf.Track(atmosConfig, "exec.resolveDeferredYamlFunctions")()

	compDctx, ok := configAndStacksInfo.DeferredMergeContexts.(ComponentDeferredContexts)
	if !ok || len(compDctx) == 0 {
		return nil
	}

	resolutionCtx := GetOrCreateResolutionContext()
	processor := NewTemplateAwareYAMLProcessor(&TemplateAwareYAMLProcessorOptions{
		AtmosConfig:              atmosConfig,
		ConfigAndStacksInfo:      configAndStacksInfo,
		SettingsSection:          *settingsSectionStruct,
		ComponentTemplateContext: componentTemplateContext,
		Skip:                     skip,
		ResolutionCtx:            resolutionCtx,
	})

	// Mirror mergeComponentConfigurations' effectiveAtmosConfig: a component-level
	// `settings.list_merge_strategy` must govern list-typed deferred merges here the same way it
	// governed the Stage 2 structural merge, or a component that opts into append/merge semantics
	// would see Stage 2 and Stage 3 disagree on how its own lists combine.
	mergeConfig := effectiveAtmosConfig(atmosConfig, configAndStacksInfo.ComponentSettingsSection)

	for sectionName, sectionDctx := range compDctx {
		if sectionDctx == nil || !sectionDctx.HasDeferredValues() {
			continue
		}
		sectionMap, ok := configAndStacksInfo.ComponentSection[sectionName].(map[string]any)
		if !ok {
			continue
		}
		// Clone before resolving: sectionDctx may be a reference into the shared FindStacksMap
		// cache (see DeferredMergeContexts' doc comment on ConfigAndStacksInfo).
		if err := m.ApplyDeferredMerges(sectionDctx.Clone(), sectionMap, mergeConfig, processor); err != nil {
			return fmt.Errorf("failed to resolve deferred YAML functions in %q: %w", sectionName, err)
		}
	}

	return nil
}
