package exec

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
)

// sortMapKeys recursively sorts map keys for consistent comparison.
func sortMapKeys(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Get all keys
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}

	// Sort keys
	sort.Strings(keys)

	// Process each key in sorted order
	for _, k := range keys {
		v := m[k]
		result[k] = processValue(v)
	}

	return result
}

// processValue recursively processes a value, sorting map keys and handling slices.
func processValue(v interface{}) interface{} {
	// Handle maps
	if nestedMap, ok := v.(map[string]interface{}); ok {
		return sortMapKeys(nestedMap)
	}

	// Handle slices
	if nestedSlice, ok := v.([]interface{}); ok {
		processedSlice := make([]interface{}, len(nestedSlice))
		for i, item := range nestedSlice {
			processedSlice[i] = processValue(item)
		}
		return processedSlice
	}

	// Return unchanged for other types
	return v
}

// generatePlanDiff generates a diff between two terraform plans.
func generatePlanDiff(origPlan, newPlan map[string]interface{}, opts ...planDiffOption) (string, bool) {
	var diff strings.Builder
	hasDiff := false

	// Compare variables
	if varsDiff, varsHasDiff := compareVariables(origPlan, newPlan); varsHasDiff {
		hasDiff = true
		diff.WriteString(varsDiff)
	}

	// Compare resources
	if resourcesDiff, resourcesHasDiff := compareResourceSections(origPlan, newPlan, opts...); resourcesHasDiff {
		hasDiff = true
		diff.WriteString(resourcesDiff)
	}

	// Compare outputs
	if outputsDiff, outputsHasDiff := compareOutputSections(origPlan, newPlan); outputsHasDiff {
		hasDiff = true
		diff.WriteString(outputsDiff)
	}

	return diff.String(), hasDiff
}

// compareVariables compares variables between two plans and returns the diff.
func compareVariables(origPlan, newPlan map[string]interface{}) (string, bool) {
	origVars, newVars := getVariables(origPlan), getVariables(newPlan)
	if reflect.DeepEqual(origVars, newVars) {
		return "", false
	}

	var diff strings.Builder
	diff.WriteString("Variables:\n")
	diff.WriteString("----------\n")

	// Find added variables
	for k, v := range newVars {
		if _, exists := origVars[k]; !exists {
			diff.WriteString(fmt.Sprintf("+ %s: %v\n", k, formatValue(v)))
		}
	}

	// Find removed variables
	for k, v := range origVars {
		if _, exists := newVars[k]; !exists {
			diff.WriteString(fmt.Sprintf("- %s: %v\n", k, formatValue(v)))
		}
	}

	// Find changed variables
	for k, origV := range origVars {
		if newV, exists := newVars[k]; exists && !reflect.DeepEqual(origV, newV) {
			diff.WriteString(fmt.Sprintf("~ %s: %v => %v\n", k, formatValue(origV), formatValue(newV)))
		}
	}

	diff.WriteString("\n")
	return diff.String(), true
}

// compareResourceSections compares resource sections between two plans and returns the diff.
func compareResourceSections(origPlan, newPlan map[string]interface{}, opts ...planDiffOption) (string, bool) {
	origResources := normalizePlanDiffResources(getResources(origPlan), opts...)
	newResources := normalizePlanDiffResources(getResources(newPlan), opts...)
	if reflect.DeepEqual(origResources, newResources) {
		return "", false
	}

	var diff strings.Builder
	diff.WriteString("Resources:\n")
	diff.WriteString("-----------\n")
	diff.WriteString("\n")

	resourceDiff := compareResources(origResources, newResources, opts...)
	diff.WriteString(resourceDiff)
	diff.WriteString("\n")

	return diff.String(), true
}

// compareOutputSections compares output sections between two plans and returns the diff.
func compareOutputSections(origPlan, newPlan map[string]interface{}) (string, bool) {
	origOutputs, newOutputs := getOutputs(origPlan), getOutputs(newPlan)
	if reflect.DeepEqual(origOutputs, newOutputs) {
		return "", false
	}

	var diff strings.Builder
	diff.WriteString("Outputs:\n")
	diff.WriteString("--------\n")

	outputDiff := compareOutputs(origOutputs, newOutputs)
	diff.WriteString(outputDiff)

	return diff.String(), true
}

// processPriorStateResources extracts resources from prior_state.
func processPriorStateResources(plan map[string]interface{}, result map[string]interface{}) {
	priorState, ok := plan["prior_state"].(map[string]interface{})
	if !ok {
		return
	}

	values, ok := priorState["values"].(map[string]interface{})
	if !ok {
		return
	}

	rootModule, ok := values["root_module"].(map[string]interface{})
	if !ok {
		return
	}

	processRootModuleResources(rootModule, result)
}

// processPlannedValuesResources extracts resources from planned_values.
func processPlannedValuesResources(plan map[string]interface{}, result map[string]interface{}) {
	plannedValues, ok := plan["planned_values"].(map[string]interface{})
	if !ok {
		return
	}

	rootModule, ok := plannedValues["root_module"].(map[string]interface{})
	if !ok {
		return
	}

	processRootModuleResources(rootModule, result)
}

// processRootModuleResources processes resources from a root_module.
func processRootModuleResources(rootModule map[string]interface{}, result map[string]interface{}) {
	resources, ok := rootModule["resources"].([]interface{})
	if !ok {
		return
	}

	for _, res := range resources {
		resMap, ok := res.(map[string]interface{})
		if !ok {
			continue
		}

		addressVal, ok := resMap["address"]
		if !ok {
			continue
		}

		modeVal, ok := resMap["mode"]
		if !ok {
			continue
		}

		mode, ok := modeVal.(string)
		if !ok {
			continue
		}

		if mode == "data" {
			continue
		}

		address, ok := addressVal.(string)
		if !ok {
			continue
		}

		result[address] = resMap
	}
}

// processResourceChanges extracts resources from resource_changes.
func processResourceChanges(plan map[string]interface{}, result map[string]interface{}) {
	resourceChanges, ok := plan["resource_changes"].([]interface{})
	if !ok {
		return
	}

	for _, change := range resourceChanges {
		changeMap, ok := change.(map[string]interface{})
		if !ok {
			continue
		}

		addressVal, ok := changeMap["address"]
		if !ok {
			continue
		}

		modeVal, ok := changeMap["mode"]
		if !ok {
			continue
		}

		mode, ok := modeVal.(string)
		if !ok {
			continue
		}

		if mode == "data" {
			continue
		}

		address, ok := addressVal.(string)
		if !ok {
			continue
		}

		result[address] = changeMap
	}
}

// getVariables extracts variables from a terraform plan.
func getVariables(plan map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	if vars, ok := plan["variables"].(map[string]interface{}); ok {
		for k, v := range vars {
			if varMap, ok := v.(map[string]interface{}); ok {
				if value, exists := varMap["value"]; exists {
					result[k] = value
				}
			}
		}
	}

	return result
}

// getResources extracts resources from a terraform plan.
func getResources(plan map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Process resources from different sections
	processPriorStateResources(plan, result)
	processPlannedValuesResources(plan, result)
	processResourceChanges(plan, result)

	return result
}

// getOutputs extracts outputs from a terraform plan.
func getOutputs(plan map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Check planned_values for outputs
	if plannedValues, ok := plan["planned_values"].(map[string]interface{}); ok {
		if outputs, ok := plannedValues["outputs"].(map[string]interface{}); ok {
			for k, v := range outputs {
				result[k] = v
			}
		}
	}

	// Check output_changes
	if outputChanges, ok := plan["output_changes"].(map[string]interface{}); ok {
		for k, v := range outputChanges {
			if _, exists := result[k]; !exists {
				result[k] = v
			}
		}
	}

	return result
}

// compareOutputs compares outputs between two terraform plans.
func compareOutputs(origOutputs, newOutputs map[string]interface{}) string {
	var diff strings.Builder

	// Find added outputs
	for k, v := range newOutputs {
		if _, exists := origOutputs[k]; !exists {
			diff.WriteString(fmt.Sprintf("+ %s: %v\n", k, formatValue(v)))
		}
	}

	// Find removed outputs
	for k, v := range origOutputs {
		if _, exists := newOutputs[k]; !exists {
			diff.WriteString(fmt.Sprintf("- %s: %v\n", k, formatValue(v)))
		}
	}

	// Find changed outputs
	for k, origV := range origOutputs {
		if newV, exists := newOutputs[k]; exists && !reflect.DeepEqual(origV, newV) {
			diff.WriteString(formatOutputChange(k, origV, newV))
		}
	}

	return diff.String()
}

// compareResources compares resources between two terraform plans.
func compareResources(origResources, newResources map[string]interface{}, opts ...planDiffOption) string {
	var diff strings.Builder

	// Process resource additions and removals
	processResourceAdditionsAndRemovals(&diff, origResources, newResources)

	// Process resource changes
	processChangedResources(&diff, origResources, newResources, opts...)

	return diff.String()
}

// processResourceAdditionsAndRemovals adds information about added and removed resources to the diff.
func processResourceAdditionsAndRemovals(diff *strings.Builder, origResources, newResources map[string]interface{}) {
	// Find added resources
	for k := range newResources {
		if _, exists := origResources[k]; !exists {
			diff.WriteString(fmt.Sprintf("+ %s\n", k))
		}
	}

	// Find removed resources
	for k := range origResources {
		if _, exists := newResources[k]; !exists {
			diff.WriteString(fmt.Sprintf("- %s\n", k))
		}
	}
}

// processChangedResources processes resources that exist in both but have changes.
func processChangedResources(diff *strings.Builder, origResources, newResources map[string]interface{}, opts ...planDiffOption) {
	for k, origV := range origResources {
		newV, exists := newResources[k]
		if !exists || reflect.DeepEqual(origV, newV) {
			continue
		}

		diff.WriteString(fmt.Sprintf("%s\n", k))

		// Compare resource attributes
		origAttrs := getResourceAttributes(origV)
		newAttrs := getResourceAttributes(newV)

		// Attributes that are unknown until apply are absent from the attribute maps,
		// so carry them separately to avoid rendering them as removals or additions.
		attrOpts := append(slices.Clone(opts), withPlanDiffUnknownAttributes(getUnknownAttributes(origV), getUnknownAttributes(newV)))

		// Process attribute differences
		processAttributeDifferences(diff, origAttrs, newAttrs, attrOpts...)
	}
}

// processAttributeDifferences handles comparing and generating diff for resource attributes.
func processAttributeDifferences(diff *strings.Builder, origAttrs, newAttrs map[string]interface{}, opts ...planDiffOption) {
	options := planDiffOptions{}
	for _, opt := range opts {
		opt(&options)
	}
	// Important attributes to always show first if they exist
	priorityAttrs := []string{"id", "url", "content"}

	// Process priority attributes first
	processPriorityAttributes(diff, origAttrs, newAttrs, priorityAttrs, options)

	// Explicit exclusions have already been applied to the compared resource records.
	// Older editions additionally retain the historical display-only suppression.
	processRegularAttributeChanges(diff, origAttrs, newAttrs, priorityAttrs, options)

	// Find added attributes (that weren't in the priority list)
	processAddedAttributes(diff, origAttrs, newAttrs, priorityAttrs, options)
}

// processPriorityAttributes handles high-priority attributes that should be shown first.
func processPriorityAttributes(diff *strings.Builder, origAttrs, newAttrs map[string]interface{}, priorityAttrs []string, options planDiffOptions) {
	for _, attrK := range priorityAttrs {
		origAttrV, origExists := origAttrs[attrK]
		newAttrV, newExists := newAttrs[attrK]

		switch {
		case origExists && newExists && !reflect.DeepEqual(origAttrV, newAttrV):
			printAttributeDiff(diff, attrK, origAttrV, newAttrV)
		case origExists && !newExists:
			diff.WriteString(options.removedAttributeLine(attrK, origAttrV))
		case !origExists && newExists:
			diff.WriteString(options.addedAttributeLine(attrK, newAttrV))
		}
	}
}

// processRegularAttributeChanges handles changed and removed attributes.
func processRegularAttributeChanges(diff *strings.Builder, origAttrs, newAttrs map[string]interface{}, priorityAttrs []string, options planDiffOptions) {
	for attrK, origAttrV := range origAttrs {
		// Skip priority attributes (already processed).
		if slices.Contains(priorityAttrs, attrK) || (options.legacyDisplay && legacyHiddenAttribute(attrK)) {
			continue
		}

		if newAttrV, exists := newAttrs[attrK]; exists && !reflect.DeepEqual(origAttrV, newAttrV) {
			printAttributeDiff(diff, attrK, origAttrV, newAttrV)
		} else if !exists {
			diff.WriteString(options.removedAttributeLine(attrK, origAttrV))
		}
	}
}

// processAddedAttributes handles new attributes that didn't exist before.
func processAddedAttributes(diff *strings.Builder, origAttrs, newAttrs map[string]interface{}, priorityAttrs []string, options planDiffOptions) {
	for attrK, newAttrV := range newAttrs {
		if _, exists := origAttrs[attrK]; !exists && !slices.Contains(priorityAttrs, attrK) && (!options.legacyDisplay || !legacyHiddenAttribute(attrK)) {
			diff.WriteString(options.addedAttributeLine(attrK, newAttrV))
		}
	}
}

// getResourceAttributes extracts attributes from a resource.
func getResourceAttributes(resource interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	resMap, ok := resource.(map[string]interface{})
	if !ok {
		return result
	}

	// Extract values from the "values" field
	extractValuesField(resMap, result)

	// Extract values from the "change.after" field
	extractChangeAfterField(resMap, result)

	return result
}

// extractValuesField extracts attributes from the "values" field of a resource.
func extractValuesField(resMap map[string]interface{}, result map[string]interface{}) {
	values, ok := resMap["values"].(map[string]interface{})
	if !ok {
		return
	}

	for k, v := range values {
		result[k] = v
	}
}

// extractChangeAfterField extracts attributes from the "change.after" field of a resource.
func extractChangeAfterField(resMap map[string]interface{}, result map[string]interface{}) {
	change, ok := resMap["change"].(map[string]interface{})
	if !ok {
		return
	}

	after, ok := change["after"].(map[string]interface{})
	if !ok {
		return
	}

	for k, v := range after {
		result[k] = v
	}
}

// getUnknownAttributes returns the top-level attributes that Terraform reports as unknown until
// apply (change.after_unknown[attr] == true). Only a boolean true counts; nested unknown
// structures are not attribute-level unknowns.
func getUnknownAttributes(resource interface{}) map[string]bool {
	result := make(map[string]bool)

	resMap, ok := resource.(map[string]interface{})
	if !ok {
		return result
	}

	change, ok := resMap["change"].(map[string]interface{})
	if !ok {
		return result
	}

	unknown, ok := change["after_unknown"].(map[string]interface{})
	if !ok {
		return result
	}

	for k, v := range unknown {
		if isUnknown, ok := v.(bool); ok && isUnknown {
			result[k] = true
		}
	}

	return result
}
