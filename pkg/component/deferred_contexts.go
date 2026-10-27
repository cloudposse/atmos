package component

import m "github.com/cloudposse/atmos/pkg/merge"

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
