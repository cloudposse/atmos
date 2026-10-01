package merge

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"dario.cat/mergo"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func isAtmosYAMLFunction(s string) bool {
	if s == "" {
		return false
	}

	// YAML functions processed after merging. Sourced from the canonical constants in
	// pkg/utils/yaml_utils.go rather than a second, independently-maintained literal list — the
	// literal #2888 report was exactly this: !labels/!tags/!labels.keys/!labels.values existed as
	// recognized YAML functions but were missing from this list, so they were never deferred and
	// silently lost data on merge (see docs/prd/deferred-yaml-functions-evaluation-in-merge.md).
	// Deliberately excluded: !unset/!append (handled by merge-structural mechanisms, see
	// processAppendTags in merge.go) and !include/!include.raw/!literal (resolved at parse time,
	// before merge ever sees them).
	postMergeFunctions := []string{
		u.AtmosYamlFuncTemplate,
		u.AtmosYamlFuncTerraformOutput,
		u.AtmosYamlFuncTerraformState,
		u.AtmosYamlFuncStoreGet,
		u.AtmosYamlFuncStore,
		u.AtmosYamlFuncExec,
		u.AtmosYamlFuncEnv,
		u.AtmosYamlFuncLabels,
		u.AtmosYamlFuncLabelsKeys,
		u.AtmosYamlFuncLabelsValues,
		u.AtmosYamlFuncTags,
	}

	for _, fn := range postMergeFunctions {
		if strings.HasPrefix(s, fn) {
			return true
		}
	}

	return false
}

// hasAnyYAMLFunction reports whether `data` contains any Atmos YAML function
// string anywhere in its nested map structure. Used by WalkAndDeferYAMLFunctions
// to short-circuit the deep copy when there is nothing to defer.
//
// This is a non-allocating recursive scan. It returns on first hit.
func hasAnyYAMLFunction(data map[string]interface{}) bool {
	for _, value := range data {
		if strVal, ok := value.(string); ok && isAtmosYAMLFunction(strVal) {
			return true
		}
		if mapVal, ok := value.(map[string]interface{}); ok && hasAnyYAMLFunction(mapVal) {
			return true
		}
	}
	return false
}

// WalkAndDeferYAMLFunctions walks through a map and defers any YAML functions.
// Returns a new map with YAML functions replaced by nil placeholders.
//
// When `data` contains no YAML functions anywhere in its nested structure,
// returns `data` as-is without allocation. Callers must treat the returned
// map as read-only; the existing call site (MergeWithDeferred → Merge) does
// not mutate inputs, which is verified by TestWalkAndDeferYAMLFunctions_NoFunctionsShortCircuit.
//
// Heatmap context: in a large-stack workload (~9k component instances), this
// function previously accounted for 527k recursive calls / 1m26s of CPU time,
// with most subtrees containing zero YAML functions and being copied for no
// reason. The short-circuit eliminates the deep-copy allocation in that case.
//
// NOTE: The recursive call re-enters this same function (with its
// hasAnyYAMLFunction short-circuit) by design — for function-sparse trees,
// avoiding the per-subtree allocation outweighs the cost of the redundant
// pre-check. A split into an outer/inner pair (Phase 7 style) was tried in
// Phase 10 and reverted: the inner-only walker had to allocate on every
// recursive level, regressing the common case.
func WalkAndDeferYAMLFunctions(dctx *DeferredMergeContext, data map[string]interface{}, basePath []string) map[string]interface{} {
	defer perf.Track(nil, "merge.WalkAndDeferYAMLFunctions")()

	if data == nil {
		return nil
	}

	// Fast path: no YAML functions anywhere in this subtree — no walk, no
	// allocation. Callers must treat the result as read-only (Merge does).
	if !hasAnyYAMLFunction(data) {
		return data
	}

	result := make(map[string]interface{}, len(data))

	for key, value := range data {
		currentPath := append(slices.Clone(basePath), key)

		// Check if this value is a YAML function string.
		if strVal, ok := value.(string); ok && isAtmosYAMLFunction(strVal) {
			// Defer this value.
			dctx.AddDeferred(currentPath, strVal)
			// Replace with placeholder (nil) to avoid type conflicts during merge.
			result[key] = nil
			continue
		}

		// Recursively process nested maps.
		if mapVal, ok := value.(map[string]interface{}); ok {
			result[key] = WalkAndDeferYAMLFunctions(dctx, mapVal, currentPath)
			continue
		}

		// Keep all other values as-is.
		result[key] = value
	}

	return result
}

// isMap checks if a value is a map[string]interface{}.
func isMap(v interface{}) bool {
	_, ok := v.(map[string]interface{})
	return ok
}

// isSlice checks if a value is a []interface{}.
func isSlice(v interface{}) bool {
	_, ok := v.([]interface{})
	return ok
}

// SetValueAtPath sets a value at a specific path in a nested map structure.
// Creates intermediate maps as needed.
// Precondition: data must be a non-nil map (panics if nil).
func SetValueAtPath(data map[string]interface{}, path []string, value interface{}) error {
	defer perf.Track(nil, "merge.SetValueAtPath")()

	if len(path) == 0 {
		return errUtils.ErrEmptyPath
	}

	// Navigate to the parent of the target field.
	current := data
	for i := 0; i < len(path)-1; i++ {
		key := path[i]

		// Get or create the next level.
		next, exists := current[key]
		if !exists {
			// Create a new map for this level.
			newMap := make(map[string]interface{})
			current[key] = newMap
			current = newMap
			continue
		}

		// Check if it's a map we can navigate into.
		nextMap, ok := next.(map[string]interface{})
		if !ok {
			return fmt.Errorf("%w: path=%v field=%s", errUtils.ErrCannotNavigatePath, path, key)
		}
		current = nextMap
	}

	// Set the value at the final key.
	current[path[len(path)-1]] = value
	return nil
}

// GetValueAtPath returns the value at a specific path in a nested map structure.
func GetValueAtPath(data map[string]interface{}, path []string) (interface{}, bool) {
	if len(path) == 0 || data == nil {
		return nil, false
	}

	current := data
	for i := 0; i < len(path)-1; i++ {
		key := path[i]
		next, exists := current[key]
		if !exists {
			return nil, false
		}
		nextMap, ok := next.(map[string]interface{})
		if !ok {
			return nil, false
		}
		current = nextMap
	}

	val, exists := current[path[len(path)-1]]
	return val, exists
}

// mergeSlicesAppendStrategy concatenates all slice values in precedence order.
// Non-slice values are silently skipped to handle type mismatches gracefully.
func mergeSlicesAppendStrategy(values []*DeferredValue) []interface{} {
	var result []interface{}
	for _, dv := range values {
		if slice, ok := dv.Value.([]interface{}); ok {
			result = append(result, slice...)
		}
		// Skip non-slice values in append mode.
	}
	return result
}

// mergeSliceItems merges two slice items, deep-merging if both are maps.
func mergeSliceItems(dst, src interface{}) (interface{}, error) {
	srcMap, srcIsMap := src.(map[string]interface{})
	dstMap, dstIsMap := dst.(map[string]interface{})

	if srcIsMap && dstIsMap {
		mergedMap := make(map[string]interface{})
		// Copy destination first.
		for k, v := range dstMap {
			mergedMap[k] = v
		}
		// NOTE: This still uses mergo (not the native merge) because YAML function
		// slice merging has different semantics — it operates on individual slice
		// elements during !include/!merge resolution, not on full stack configs.
		// The hot-path merge (MergeWithOptions, ~118k calls/run) uses native merge.
		// TODO: migrate to native merge to eliminate the mergo dependency entirely.
		if err := mergo.Merge(&mergedMap, srcMap, mergo.WithOverride); err != nil {
			return nil, fmt.Errorf("%w: merge slice items: %w", errUtils.ErrMerge, err)
		}
		return mergedMap, nil
	}

	// Override with source value if not both maps.
	return src, nil
}

// mergeSlicesMergeStrategy deep-merges slice items by index position.
func mergeSlicesMergeStrategy(values []*DeferredValue) (interface{}, error) {
	// Start with first value.
	firstSlice, ok := values[0].Value.([]interface{})
	if !ok {
		return values[0].Value, nil
	}

	// Make a copy to avoid modifying the original.
	result := make([]interface{}, len(firstSlice))
	copy(result, firstSlice)

	// Merge each subsequent value.
	for i := 1; i < len(values); i++ {
		sourceSlice, ok := values[i].Value.([]interface{})
		if !ok {
			// Type mismatch - use source value.
			return values[i].Value, nil
		}

		// Merge items up to length of source slice.
		for idx := 0; idx < len(sourceSlice) && idx < len(result); idx++ {
			merged, err := mergeSliceItems(result[idx], sourceSlice[idx])
			if err != nil {
				return nil, err
			}
			result[idx] = merged
		}

		// Append remaining source items if source is longer.
		if len(sourceSlice) > len(result) {
			result = append(result, sourceSlice[len(result):]...)
		}
	}
	return result, nil
}

// mergeSlices merges slice values according to the configured list merge strategy.
func mergeSlices(values []*DeferredValue, strategy string) (interface{}, error) {
	if len(values) == 0 {
		return nil, nil
	}

	switch strategy {
	case ListMergeStrategyReplace:
		// Default: latest value wins.
		return values[len(values)-1].Value, nil
	case ListMergeStrategyAppend:
		return mergeSlicesAppendStrategy(values), nil
	case ListMergeStrategyMerge:
		return mergeSlicesMergeStrategy(values)
	default:
		return nil, fmt.Errorf("%w: %s", errUtils.ErrUnknownListMergeStrategy, strategy)
	}
}

// mergeDeferredMaps deep-merges map values.
func mergeDeferredMaps(values []*DeferredValue) (interface{}, error) {
	resultMap, ok := values[0].Value.(map[string]interface{})
	if !ok {
		return values[len(values)-1].Value, nil
	}

	// Make a copy to avoid modifying the original.
	mergedMap := make(map[string]interface{})
	for k, v := range resultMap {
		mergedMap[k] = v
	}

	for i := 1; i < len(values); i++ {
		valueMap, ok := values[i].Value.(map[string]interface{})
		if !ok {
			// Type changed - override completely.
			return values[i].Value, nil
		}

		// NOTE: Uses mergo (not native merge) — see comment in mergeSliceItems above.
		if err := mergo.Merge(&mergedMap, valueMap, mergo.WithOverride); err != nil {
			return nil, fmt.Errorf("%w: merge deferred maps: %w", errUtils.ErrMerge, err)
		}
	}

	return mergedMap, nil
}

// MergeDeferredValues merges all values for a single field path.
func MergeDeferredValues(values []*DeferredValue, atmosConfig *schema.AtmosConfiguration) (interface{}, error) {
	defer perf.Track(atmosConfig, "merge.MergeDeferredValues")()

	if len(values) == 0 {
		return nil, nil
	}

	result := values[0].Value

	// For simple types: override with highest precedence.
	if !isMap(result) && !isSlice(result) {
		return values[len(values)-1].Value, nil
	}

	// For slices: respect list_merge_strategy.
	if isSlice(result) {
		strategy := atmosConfig.Settings.ListMergeStrategy
		if strategy == "" {
			strategy = ListMergeStrategyReplace
		}
		return mergeSlices(values, strategy)
	}

	// For maps: deep-merge all values.
	return mergeDeferredMaps(values)
}

// MergeWithDeferred performs a merge operation with YAML function deferral.
// It creates a deferred merge context, walks each input to defer YAML functions,
// performs the merge, and returns both the result and the deferred context.
// The caller is responsible for calling ApplyDeferredMerges to process deferred functions.
//
// Integration Example:
//
//	// In stack processor (internal/exec/stack_processor_merge.go):
//	// Replace: finalVars, err := m.Merge(atmosConfig, inputs)
//	// With:
//	finalVars, dctx, err := m.MergeWithDeferred(atmosConfig, inputs)
//	if err != nil {
//	    return nil, err
//	}
//
//	// After all sections are merged, apply deferred merges with YAML function processor:
//	processor := &YAMLProcessor{...} // Implement YAMLFunctionProcessor interface
//	if err := m.ApplyDeferredMerges(dctx, finalVars, atmosConfig, processor); err != nil {
//	    return nil, err
//	}
//
// Note: For full integration, YAML function processing during loading must be modified
// to keep YAML functions as strings rather than processing them immediately.
// See docs/prd/yaml-function-merge-handling.md for complete implementation plan.
func MergeWithDeferred(
	atmosConfig *schema.AtmosConfiguration,
	inputs []map[string]any,
) (map[string]any, *DeferredMergeContext, error) {
	defer perf.Track(atmosConfig, "merge.MergeWithDeferred")()

	// Create deferred merge context.
	dctx := NewDeferredMergeContext()

	// Fast path for the all-empty case. The mergeComponentConfigurations
	// pipeline calls this function ~9 times per component instance with 3-4
	// candidate inputs (e.g., GlobalVars / BaseComponentVars / ComponentVars /
	// ComponentOverridesVars). When every layer is empty (common for the
	// terraform-only sections on non-terraform components), there is nothing
	// to walk or merge — return an empty map and an empty dctx directly.
	//
	// A 1-input fast path was tried and reverted: when the single non-empty
	// input contained no Atmos YAML functions, WalkAndDeferYAMLFunctions's
	// Phase 3 short-circuit returned the input map as-is, and we were
	// returning that shared reference to the caller. Downstream consumers in
	// mergeComponentConfigurations mutate the result map (storing values into
	// the assembled component map and re-using merge results), which then
	// corrupted the upstream cached settings/vars/auth/etc. for sibling
	// components — surfaced as TestSpaceliftStackProcessor losing 7 stacks
	// because mutated settings.spacelift.workspace_enabled bled across
	// components. The Merge slow path always returns a deep-copied result
	// (via MergeWithOptions's own 1-input fast path → DeepCopyMap), so falling
	// through preserves the contract; the wrapper overhead saved by the
	// 1-input shortcut was small compared to the deep-copy cost that has to
	// happen regardless.
	allEmpty := true
	for _, input := range inputs {
		if len(input) > 0 {
			allEmpty = false
			break
		}
	}
	if allEmpty {
		return map[string]any{}, dctx, nil
	}

	// Walk each input and defer YAML functions.
	processedInputs := make([]map[string]any, len(inputs))
	for i, input := range inputs {
		processedInputs[i] = WalkAndDeferYAMLFunctions(dctx, input, []string{})
		dctx.IncrementPrecedence()
	}

	// For every deferred path, also capture the concrete (non-function) value from every OTHER
	// layer at that same path, at that layer's own precedence.
	//
	// Without this, a concrete value at a LOWER-precedence layer than the deferred function is
	// silently destroyed by the raw structural merge below: WalkAndDeferYAMLFunctions replaces the
	// function with a literal nil placeholder, and a higher-precedence nil overrides a
	// lower-precedence concrete map/slice during the merge — the lower layer's contribution is
	// gone before ApplyDeferredMerges (which discovers competing concrete values by reading back
	// the merge *result*, via addExistingConcreteValue) ever runs. That path only recovers a
	// concrete value that structurally *survives* the raw merge (i.e., has the highest precedence
	// at that path) — the exact mirror image of the #2888 bug this whole mechanism exists to fix.
	//
	// Reads from processedInputs (not the raw inputs) so a nested deferred function inside this
	// same competing value is already a nil placeholder here, not the raw, still-untouched
	// function string — its own DeferredValue entry (recorded above, at the deeper path) is what
	// resolves it later.
	fillMissingLayerValues(dctx, processedInputs)

	// Perform normal merge (no type conflicts now that YAML functions are deferred).
	result, err := Merge(atmosConfig, processedInputs)
	if err != nil {
		return nil, nil, err
	}

	return result, dctx, nil
}

// fillMissingLayerValues adds a concrete DeferredValue entry, at that layer's own precedence, for
// every layer that has a value at a deferred path but hasn't otherwise contributed a DeferredValue
// there (i.e., its own value at that path isn't itself a deferred function). See MergeWithDeferred's
// call-site comment for why this is necessary.
func fillMissingLayerValues(dctx *DeferredMergeContext, processedInputs []map[string]any) {
	for pathKey, values := range dctx.deferredValues {
		if len(values) == 0 {
			continue
		}
		path := values[0].Path

		covered := make(map[int]bool, len(values))
		for _, dv := range values {
			covered[dv.Precedence] = true
		}

		for i, input := range processedInputs {
			if covered[i] {
				continue
			}
			if v, ok := GetValueAtPath(input, path); ok && v != nil {
				dctx.deferredValues[pathKey] = append(dctx.deferredValues[pathKey], &DeferredValue{
					Path:       slices.Clone(path),
					Value:      v,
					Precedence: i,
					IsFunction: false,
				})
			}
		}
	}
}

// processYAMLFunctions processes YAML functions in deferred values using the provided processor.
func processYAMLFunctions(deferredValues []*DeferredValue, processor YAMLFunctionProcessor, pathKey string) error {
	for _, dv := range deferredValues {
		if !dv.IsFunction {
			continue
		}

		// Process the YAML function string (e.g., "!template '{{ .settings.vpc_cidr }}'").
		valueStr, ok := dv.Value.(string)
		if !ok {
			// Not a string, keep as-is.
			continue
		}

		processedValue, err := processor.ProcessYAMLFunctionString(valueStr)
		if err != nil {
			return fmt.Errorf("failed to process YAML function at %s: %w", pathKey, err)
		}

		// Update the deferred value with the processed result.
		dv.Value = processedValue
		dv.IsFunction = false
	}

	return nil
}

// getConfigOrDefault returns the provided config or a default if nil.
func getConfigOrDefault(atmosConfig *schema.AtmosConfiguration) *schema.AtmosConfiguration {
	if atmosConfig == nil {
		return &schema.AtmosConfiguration{}
	}
	return atmosConfig
}

// processDeferredField processes a single deferred field and applies it to the result.
func processDeferredField(pathKey string, deferredValues []*DeferredValue, result map[string]interface{}, cfgPtr *schema.AtmosConfiguration, processor YAMLFunctionProcessor) error {
	// Every layer's contribution at this path — deferred function or concrete value — was already
	// collected into deferredValues by MergeWithDeferred (see fillMissingLayerValues), each at its
	// own layer's precedence. Sort by precedence (lower first, so higher precedence wins in merge).
	sort.Slice(deferredValues, func(i, j int) bool {
		return deferredValues[i].Precedence < deferredValues[j].Precedence
	})

	// Process YAML functions to get actual values if processor is provided.
	if processor != nil {
		if err := processYAMLFunctions(deferredValues, processor, pathKey); err != nil {
			return err
		}
	}

	// Merge all values for this path (respects list_merge_strategy).
	merged, err := MergeDeferredValues(deferredValues, cfgPtr)
	if err != nil {
		return fmt.Errorf("failed to merge deferred values at %s: %w", pathKey, err)
	}

	// Apply to result at the correct path.
	if err := SetValueAtPath(result, deferredValues[0].Path, merged); err != nil {
		return fmt.Errorf("failed to set value at %s: %w", pathKey, err)
	}

	return nil
}

// ApplyDeferredMerges processes all deferred YAML functions and applies them to the result.
// This function is called after the initial merge to handle YAML functions that were deferred
// to avoid type conflicts during merging.
//
// If processor is nil, YAML function strings are kept as-is (for testing or when processing is not needed).
// If processor is provided, YAML functions are processed to their actual values before merging.
//
// When processor is non-nil, this function resolves against a clone of dctx rather than dctx
// itself: processYAMLFunctions mutates DeferredValue.Value/IsFunction in place, and dctx may be a
// reference into shared/cached storage (e.g. a caller recovered it from a FindStacksMap cache
// entry) that other concurrent callers still hold an unresolved view of. Cloning here is
// defense-in-depth on top of any cloning callers do themselves — it makes "processor != nil never
// mutates the caller's dctx" a guarantee of this function, not a caller discipline requirement.
func ApplyDeferredMerges(dctx *DeferredMergeContext, result map[string]interface{}, atmosConfig *schema.AtmosConfiguration, processor YAMLFunctionProcessor) error {
	defer perf.Track(atmosConfig, "merge.ApplyDeferredMerges")()

	if dctx == nil || !dctx.HasDeferredValues() {
		return nil
	}

	if processor != nil {
		dctx = dctx.Clone()
	}

	cfgPtr := getConfigOrDefault(atmosConfig)

	// Process ancestor paths before descendant paths (e.g. "vars.combo" before
	// "vars.combo.nested"). Each pathKey's resolution ends in an unconditional
	// SetValueAtPath call that replaces whatever currently exists at that exact path — if a
	// descendant were processed before its ancestor, the ancestor's later wholesale replace of
	// the shared parent map would silently discard the descendant's already-resolved value.
	// dctx.GetDeferredValues() is a plain Go map, and Go randomizes map iteration order per
	// range, so without this explicit order the outcome was nondeterministic (confirmed via
	// live field-testing: the same command produced a correct result, a silently dropped value,
	// or an unresolved function string leaking into output, all from the same input).
	allDeferred := dctx.GetDeferredValues()
	pathKeys := make([]string, 0, len(allDeferred))
	for pathKey, deferredValues := range allDeferred {
		if len(deferredValues) == 0 {
			continue
		}
		pathKeys = append(pathKeys, pathKey)
	}
	sort.Slice(pathKeys, func(i, j int) bool {
		return len(allDeferred[pathKeys[i]][0].Path) < len(allDeferred[pathKeys[j]][0].Path)
	})

	// Process each deferred field.
	for _, pathKey := range pathKeys {
		deferredValues := allDeferred[pathKey]

		// Single-contribution reuse: when a deferred path has exactly one contributing layer
		// (no concrete override and no competing deferred function to deep-merge against) and
		// `result` already holds a fully resolved value at that path, re-resolving here would
		// execute the same function a SECOND time. In Atmos, exec.processStacks runs a
		// document-wide YAML-function pass (ProcessCustomYamlTags) before this Stage 3 resolver,
		// so the surviving (highest-precedence) function at a no-collision path is already
		// resolved in `result` by the time we get here. Re-running it is a harmless no-op for
		// pure/cached functions (!template, !terraform.output/state, !labels, !tags, !env), but
		// doubles the side effects and cost of UNCACHED functions such as !exec (runs the shell
		// again) and !store (an extra backend read). Reuse the value the earlier pass produced.
		// See docs/fixes/2026-08-13-deferred-merge-double-execution.md.
		//
		// Scope of the guard:
		//   - processor != nil restricts this to the Stage 3 resolution pass; the nil-processor
		//     structural-writeback callers never resolve functions and must keep placing the raw
		//     string, so they must not skip.
		//   - len == 1 restricts this to no-collision paths; a genuine collision (concrete
		//     override, or multiple competing functions) still needs the full resolve-and-merge.
		//   - isResolvedNonFunctionValue guards a real-processor caller that has NOT had a prior
		//     resolution pass (result still holds a nil placeholder or the raw function string):
		//     that caller must still resolve the function here rather than skip it.
		if processor != nil && len(deferredValues) == 1 {
			if existing, ok := GetValueAtPath(result, deferredValues[0].Path); ok && isResolvedNonFunctionValue(existing) {
				continue
			}
		}

		if err := processDeferredField(pathKey, deferredValues, result, cfgPtr, processor); err != nil {
			return err
		}
	}

	return nil
}

// isResolvedNonFunctionValue reports whether v is a concrete, already-resolved value — i.e. neither
// a nil placeholder (left by WalkAndDeferYAMLFunctions) nor a still-unresolved Atmos YAML function
// string. ApplyDeferredMerges uses it to decide whether a single-contribution deferred path was
// already resolved by an earlier pass and can be reused instead of re-executed.
func isResolvedNonFunctionValue(v interface{}) bool {
	if v == nil {
		return false
	}
	if s, ok := v.(string); ok && isAtmosYAMLFunction(s) {
		return false
	}
	return true
}
