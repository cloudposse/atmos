package merge

import (
	"fmt"
	"sort"

	"github.com/cloudposse/atmos/pkg/schema"
)

// ExampleMergeWithDeferred demonstrates the complete deferred merge workflow.
// This example shows how YAML functions are detected, deferred during merge,
// and then applied after merging to avoid type conflicts.
func ExampleMergeWithDeferred() {
	// Create Atmos configuration with replace strategy.
	cfg := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "replace",
		},
	}

	// Simulate configuration from multiple imports.
	// In a real scenario, these would come from different YAML files
	// in an import chain (e.g., catalog/base.yaml, stacks/dev.yaml, stacks/prod.yaml).
	inputs := []map[string]any{
		{
			// Base catalog with YAML function.
			"template_config": "!template '{{ .settings.base }}'",
			"regular_value":   "from-base",
			"nested": map[string]interface{}{
				"yaml_func": "!terraform.output vpc.id",
				"static":    "base-static",
			},
		},
		{
			// Dev environment override.
			"template_config": "!template '{{ .settings.dev }}'",
			"regular_value":   "from-dev",
			"nested": map[string]interface{}{
				"static": "dev-static",
			},
		},
		{
			// Prod environment with concrete value.
			// Without deferred merge, this would cause a type conflict
			// if the YAML function returns a different type.
			"regular_value": "from-prod",
		},
	}

	// Perform merge with deferred YAML functions.
	result, dctx, err := MergeWithDeferred(cfg, inputs)
	if err != nil {
		fmt.Printf("Merge error: %v\n", err)
		return
	}

	// Show what was deferred (sorted for consistent output).
	fmt.Println("Deferred YAML functions:")
	deferredValues := dctx.GetDeferredValues()
	paths := make([]string, 0, len(deferredValues))
	for path := range deferredValues {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		values := deferredValues[path]
		fmt.Printf("  %s: %d values\n", path, len(values))
		for i, v := range values {
			fmt.Printf("    [%d] precedence=%d value=%v\n", i, v.Precedence, v.Value)
		}
	}

	// Show merged result (with nil placeholders for YAML functions).
	fmt.Println("\nMerged result (before applying deferred):")
	fmt.Printf("  template_config: %v\n", result["template_config"])
	fmt.Printf("  regular_value: %v\n", result["regular_value"])
	if nested, ok := result["nested"].(map[string]interface{}); ok {
		fmt.Printf("  nested.yaml_func: %v\n", nested["yaml_func"])
		fmt.Printf("  nested.static: %v\n", nested["static"])
	}

	// Apply deferred merges.
	// Note: In production, this would process YAML functions.
	// For this example, they remain as strings since processing is TODO.
	err = ApplyDeferredMerges(dctx, result, cfg)
	if err != nil {
		fmt.Printf("Apply error: %v\n", err)
		return
	}

	fmt.Println("\nFinal result (after applying deferred):")
	fmt.Printf("  template_config: %v\n", result["template_config"])
	fmt.Printf("  regular_value: %v\n", result["regular_value"])
	if nested, ok := result["nested"].(map[string]interface{}); ok {
		fmt.Printf("  nested.yaml_func: %v\n", nested["yaml_func"])
		fmt.Printf("  nested.static: %v\n", nested["static"])
	}

	// Output:
	// Deferred YAML functions:
	//   nested.yaml_func: 1 values
	//     [0] precedence=0 value=!terraform.output vpc.id
	//   template_config: 2 values
	//     [0] precedence=0 value=!template '{{ .settings.base }}'
	//     [1] precedence=1 value=!template '{{ .settings.dev }}'
	//
	// Merged result (before applying deferred):
	//   template_config: <nil>
	//   regular_value: from-prod
	//   nested.yaml_func: <nil>
	//   nested.static: dev-static
	//
	// Final result (after applying deferred):
	//   template_config: !template '{{ .settings.dev }}'
	//   regular_value: from-prod
	//   nested.yaml_func: !terraform.output vpc.id
	//   nested.static: dev-static
}

// ExampleMergeWithDeferred_listMergeStrategies demonstrates how list merge
// strategies work with deferred YAML functions.
func ExampleMergeWithDeferred_listMergeStrategies() {
	// Test with append strategy.
	cfg := &schema.AtmosConfiguration{
		Settings: schema.AtmosSettings{
			ListMergeStrategy: "append",
		},
	}

	inputs := []map[string]any{
		{
			"items": []interface{}{"item1", "item2"},
		},
		{
			"items": []interface{}{"item3"},
		},
	}

	result, _, err := MergeWithDeferred(cfg, inputs)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Append strategy result: %v\n", result["items"])

	// Test with merge strategy.
	cfg.Settings.ListMergeStrategy = "merge"

	inputs = []map[string]any{
		{
			"configs": []interface{}{
				map[string]interface{}{"name": "config1", "enabled": true},
				map[string]interface{}{"name": "config2", "enabled": false},
			},
		},
		{
			"configs": []interface{}{
				map[string]interface{}{"enabled": false, "timeout": 30},
			},
		},
	}

	result, _, err = MergeWithDeferred(cfg, inputs)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Println("Merge strategy result:")
	if configs, ok := result["configs"].([]interface{}); ok {
		for i, config := range configs {
			fmt.Printf("  [%d]: %v\n", i, config)
		}
	}

	// Output:
	// Append strategy result: [item1 item2 item3]
	// Merge strategy result:
	//   [0]: map[enabled:false name:config1 timeout:30]
	//   [1]: map[enabled:false name:config2]
}
