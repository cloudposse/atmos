// Package plandiff compares two Terraform JSON plans and renders the differences as text.
//
// The configured exclusions (plan_diff.ignore_changes) apply to both the verdict and the
// display, so an excluded attribute can neither fail a comparison nor appear in its output.
// The package only compares plan documents that have already been decoded; it does not run
// Terraform.
package plandiff

import (
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Compare compares two decoded Terraform JSON plans and returns the rendered differences and
// whether any difference remains after the exclusions given by the options are applied.
// Map keys of both plans are normalized first, so callers do not need to sort them.
func Compare(orig, fresh map[string]interface{}, opts ...Option) (string, bool) {
	defer perf.Track(nil, "plandiff.Compare")()

	return generatePlanDiff(sortMapKeys(orig), sortMapKeys(fresh), opts...)
}

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
func generatePlanDiff(origPlan, newPlan map[string]interface{}, opts ...Option) (string, bool) {
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
func compareResourceSections(origPlan, newPlan map[string]interface{}, opts ...Option) (string, bool) {
	origResources := normalizeResources(getResources(origPlan), opts...)
	newResources := normalizeResources(getResources(newPlan), opts...)
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
func compareResources(origResources, newResources map[string]interface{}, opts ...Option) string {
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
func processChangedResources(diff *strings.Builder, origResources, newResources map[string]interface{}, opts ...Option) {
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
		attrOpts := append(slices.Clone(opts), withUnknownAttributes(getUnknownAttributes(origV), getUnknownAttributes(newV)))

		// Process attribute differences
		processAttributeDifferences(diff, origAttrs, newAttrs, attrOpts...)
	}
}

// processAttributeDifferences handles comparing and generating diff for resource attributes.
func processAttributeDifferences(diff *strings.Builder, origAttrs, newAttrs map[string]interface{}, opts ...Option) {
	o := newOptions(opts...)
	// Important attributes to always show first if they exist
	priorityAttrs := []string{"id", "url", "content"}

	// Process priority attributes first
	processPriorityAttributes(diff, origAttrs, newAttrs, priorityAttrs, o)

	// Explicit exclusions have already been applied to the compared resource records.
	// Older editions additionally retain the historical display-only suppression.
	processRegularAttributeChanges(diff, origAttrs, newAttrs, priorityAttrs, o)

	// Find added attributes (that weren't in the priority list)
	processAddedAttributes(diff, origAttrs, newAttrs, priorityAttrs, o)
}

// processPriorityAttributes handles high-priority attributes that should be shown first.
func processPriorityAttributes(diff *strings.Builder, origAttrs, newAttrs map[string]interface{}, priorityAttrs []string, o options) {
	for _, attrK := range priorityAttrs {
		origAttrV, origExists := origAttrs[attrK]
		newAttrV, newExists := newAttrs[attrK]

		switch {
		case origExists && newExists && !reflect.DeepEqual(origAttrV, newAttrV):
			printAttributeDiff(diff, attrK, origAttrV, newAttrV)
		case origExists && !newExists:
			diff.WriteString(o.removedAttributeLine(attrK, origAttrV))
		case !origExists && newExists:
			diff.WriteString(o.addedAttributeLine(attrK, newAttrV))
		}
	}
}

// processRegularAttributeChanges handles changed and removed attributes.
func processRegularAttributeChanges(diff *strings.Builder, origAttrs, newAttrs map[string]interface{}, priorityAttrs []string, o options) {
	for attrK, origAttrV := range origAttrs {
		// Skip priority attributes (already processed).
		if slices.Contains(priorityAttrs, attrK) || (o.legacyDisplay && legacyHiddenAttribute(attrK)) {
			continue
		}

		if newAttrV, exists := newAttrs[attrK]; exists && !reflect.DeepEqual(origAttrV, newAttrV) {
			printAttributeDiff(diff, attrK, origAttrV, newAttrV)
		} else if !exists {
			diff.WriteString(o.removedAttributeLine(attrK, origAttrV))
		}
	}
}

// processAddedAttributes handles new attributes that didn't exist before.
func processAddedAttributes(diff *strings.Builder, origAttrs, newAttrs map[string]interface{}, priorityAttrs []string, o options) {
	for attrK, newAttrV := range newAttrs {
		if _, exists := origAttrs[attrK]; !exists && !slices.Contains(priorityAttrs, attrK) && (!o.legacyDisplay || !legacyHiddenAttribute(attrK)) {
			diff.WriteString(o.addedAttributeLine(attrK, newAttrV))
		}
	}
}
