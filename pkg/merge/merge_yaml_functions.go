package merge

import (
	"fmt"
	"sort"
	"strings"

	"dario.cat/mergo"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

func isAtmosYAMLFunction(s string) bool {
	if s == "" {
		return false
	}

	// YAML functions processed after merging.
	postMergeFunctions := []string{
		"!template",
		"!terraform.output",
		"!terraform.state",
		"!store.get",
		"!store",
		"!exec",
		"!env",
	}

	for _, fn := range postMergeFunctions {
		if strings.HasPrefix(s, fn) {
			return true
		}
	}

	return false
}

// WalkAndDeferYAMLFunctions walks through a map and defers any YAML functions.
// Returns a new map with YAML functions replaced by nil placeholders.
func WalkAndDeferYAMLFunctions(dctx *DeferredMergeContext, data map[string]interface{}, basePath []string) map[string]interface{} {
	if data == nil {
		return nil
	}

	result := make(map[string]interface{}, len(data))

	for key, value := range data {
		currentPath := make([]string, len(basePath)+1)
		copy(currentPath, basePath)
		currentPath[len(basePath)] = key

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
func SetValueAtPath(data map[string]interface{}, path []string, value interface{}) error {
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

// mergeSlicesAppendStrategy concatenates all slice values in precedence order.
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
		// Merge with source using mergo.
		if err := mergo.Merge(&mergedMap, srcMap, mergo.WithOverride); err != nil {
			return nil, err
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

		if err := mergo.Merge(&mergedMap, valueMap, mergo.WithOverride); err != nil {
			return nil, err
		}
	}

	return mergedMap, nil
}

// MergeDeferredValues merges all values for a single field path.
func MergeDeferredValues(values []*DeferredValue, atmosConfig *schema.AtmosConfiguration) (interface{}, error) {
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
//	// After all sections are merged, apply deferred merges:
//	if err := m.ApplyDeferredMerges(dctx, finalVars, atmosConfig); err != nil {
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

	// Walk each input and defer YAML functions.
	processedInputs := make([]map[string]any, len(inputs))
	for i, input := range inputs {
		processedInputs[i] = WalkAndDeferYAMLFunctions(dctx, input, []string{})
		dctx.IncrementPrecedence()
	}

	// Perform normal merge (no type conflicts now that YAML functions are deferred).
	result, err := Merge(atmosConfig, processedInputs)
	if err != nil {
		return nil, nil, err
	}

	return result, dctx, nil
}

// ApplyDeferredMerges processes all deferred YAML functions and applies them to the result.
// This function is called after the initial merge to handle YAML functions that were deferred
// to avoid type conflicts during merging.
func ApplyDeferredMerges(dctx *DeferredMergeContext, result map[string]interface{}, atmosConfig *schema.AtmosConfiguration) error {
	if dctx == nil || !dctx.HasDeferredValues() {
		return nil
	}

	// Default config if nil.
	var cfg schema.AtmosConfiguration
	cfgPtr := atmosConfig
	if atmosConfig == nil {
		cfgPtr = &cfg
	}

	// Process each deferred field.
	for pathKey, deferredValues := range dctx.GetDeferredValues() {
		if len(deferredValues) == 0 {
			continue
		}

		// Sort by precedence (lower first, so higher precedence wins in merge).
		sort.Slice(deferredValues, func(i, j int) bool {
			return deferredValues[i].Precedence < deferredValues[j].Precedence
		})

		// TODO: Process YAML functions to get actual values.
		// This requires integration with internal/exec YAML function processors:
		// - !template: processTagTemplate()
		// - !terraform.output: processTerraformOutput()
		// - !terraform.state: processTerraformState()
		// - !store.get, !store: processStore()
		// - !exec: processExec()
		// - !env: processEnv()
		//
		// For now, we keep the YAML function strings as-is.
		// In the next phase, we'll add a processor interface that can handle each type.

		// Merge all values for this path (respects list_merge_strategy).
		merged, err := MergeDeferredValues(deferredValues, cfgPtr)
		if err != nil {
			return fmt.Errorf("failed to merge deferred values at %s: %w", pathKey, err)
		}

		// Apply to result at the correct path.
		if err := SetValueAtPath(result, deferredValues[0].Path, merged); err != nil {
			return fmt.Errorf("failed to set value at %s: %w", pathKey, err)
		}
	}

	return nil
}
