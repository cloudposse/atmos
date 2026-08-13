package tests

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestYAMLFunctionsInLists tests the yaml-functions-in-lists scenario.
// This verifies that YAML functions work correctly in lists without type conflicts.
func TestYAMLFunctionsInLists(t *testing.T) {
	t.Chdir("./fixtures/scenarios/yaml-functions-in-lists")

	t.Setenv("TEST_ENV_VAR", "test-env-value")
	// Set AWS_REGION to test the auth env region export feature
	// This simulates what 'atmos auth env' would export when region is configured
	t.Setenv("AWS_REGION", "us-west-2")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	require.NotNil(t, atmosConfig)

	t.Run("stack config with AWS_REGION env reference loads successfully", func(t *testing.T) {
		// This test verifies that stack configurations can reference AWS_REGION
		// using !env AWS_REGION without causing parsing errors.
		// Note: !env functions are resolved at execution time (terraform apply),
		// not during describe. This test ensures the stack with !env AWS_REGION loads.
		componentName := "test-yaml-functions-in-lists"
		stack := "test"

		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: componentName,
				Stack:     stack,
			},
		)

		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// Verify aws_region key exists in the stack config
		// The value will be !env AWS_REGION (deferred until execution)
		assert.Contains(t, vars, "aws_region", "aws_region should be present in vars")
	})

	t.Run("loads stack with yaml functions in lists", func(t *testing.T) {
		// Test that we can describe a component that uses YAML functions in lists
		componentName := "test-yaml-functions-in-lists"
		stack := "test"

		// This would fail with type conflicts if deferred merge wasn't working
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: componentName,
				Stack:     stack,
			},
		)

		require.NoError(t, err)
		require.NotNil(t, componentSection)

		// Verify the component loaded successfully
		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")
		require.NotNil(t, vars)
	})

	t.Run("handles list concatenation scenario", func(t *testing.T) {
		// This test verifies the original bug report scenario
		// Multiple terraform.state/terraform.output calls in a list
		componentName := "test-list-concatenation"
		stack := "test"

		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: componentName,
				Stack:     stack,
			},
		)

		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// Verify function_results_list exists (contains YAML functions)
		require.Contains(t, vars, "function_results_list")

		// Verify string_list exists (contains YAML functions)
		require.Contains(t, vars, "string_list")
	})

	t.Run("processes multiple components with yaml functions", func(t *testing.T) {
		// Verify all three test components can be loaded
		components := []string{
			"test-component-1",
			"test-component-2",
			"test-yaml-functions-in-lists",
		}

		for _, componentName := range components {
			t.Run(componentName, func(t *testing.T) {
				componentSection, err := e.ExecuteDescribeComponent(
					&e.ExecuteDescribeComponentParams{
						Component: componentName,
						Stack:     "test",
					},
				)

				require.NoError(t, err)
				require.NotNil(t, componentSection)

				// Each component should have vars
				vars, ok := componentSection["vars"].(map[string]interface{})
				require.True(t, ok, "vars should be a map")
				require.NotEmpty(t, vars)
			})
		}
	})
}

// TestYAMLFunctionsDeferredMerge tests the atmos-yaml-functions-merge scenario.
// This verifies that YAML functions can be merged with concrete values without type conflicts.
func TestYAMLFunctionsDeferredMerge(t *testing.T) {
	t.Chdir("./fixtures/scenarios/atmos-yaml-functions-merge")

	t.Setenv("ATMOS_STAGE", "test")

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	require.NotNil(t, atmosConfig)

	t.Run("overrides yaml function with concrete value", func(t *testing.T) {
		// Test Case 1: Override !template with concrete map
		// Without deferred merge, this would cause a type conflict and fail
		componentName := "test-yaml-to-concrete"
		stack := "test"

		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: componentName,
				Stack:     stack,
			},
		)

		// The key test: no error means deferred merge prevented type conflict
		require.NoError(t, err, "deferred merge should prevent type conflicts between YAML functions and concrete values")
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")
		require.NotEmpty(t, vars, "vars should not be empty")

		// Verify template_config exists (value doesn't matter - we just need no errors)
		assert.Contains(t, vars, "template_config")
	})

	t.Run("overrides yaml function with another yaml function", func(t *testing.T) {
		// Test Case 2: Override !template with different !template
		componentName := "test-yaml-to-yaml"
		stack := "test"

		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: componentName,
				Stack:     stack,
			},
		)

		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// template_config should exist (contains YAML function result)
		require.Contains(t, vars, "template_config")
	})

	t.Run("overrides list yaml function with concrete list", func(t *testing.T) {
		// Test Case 3: Override !terraform.output list with concrete list
		// Without deferred merge, this would cause a type conflict and fail
		componentName := "test-list-yaml-to-concrete"
		stack := "test"

		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: componentName,
				Stack:     stack,
			},
		)

		// The key test: no error means deferred merge prevented type conflict
		require.NoError(t, err, "deferred merge should prevent type conflicts between YAML functions and concrete values")
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// Verify vpc_ids exists
		assert.Contains(t, vars, "vpc_ids")
	})

	t.Run("overrides map yaml function with concrete map", func(t *testing.T) {
		// Test Case 4: Override !template map with concrete map
		// Without deferred merge, this would cause a type conflict and fail
		componentName := "test-map-yaml-to-concrete"
		stack := "test"

		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: componentName,
				Stack:     stack,
			},
		)

		// The key test: no error means deferred merge prevented type conflict
		require.NoError(t, err, "deferred merge should prevent type conflicts between YAML functions and concrete values")
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// Verify network_config exists
		assert.Contains(t, vars, "network_config")
	})

	t.Run("deep merges with yaml functions", func(t *testing.T) {
		// Test Case 5: Deep merge with YAML function results
		componentName := "test-deep-merge"
		stack := "test"

		// ProcessTemplates/ProcessYamlFunctions must be explicitly opted into: describe component
		// defaults both to false (deferred functions are left unresolved in the output, e.g. for
		// dry inspection — see the "!env AWS_REGION (deferred until execution)" case above). This
		// deep-merge behavior is specifically about the RESOLVED-function case (matching the
		// defaults real callers like `atmos describe component`/`atmos terraform plan` use), so
		// the test must ask for both, or it never exercises the deferred-merge resolution path at
		// all — it would instead pass trivially on the raw, unresolved structural merge (which
		// happens to already produce a map here since the component's own override is a literal
		// map, not a function).
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component:            componentName,
				Stack:                stack,
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
			},
		)

		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// template_config must be deep-merged: keys unique to the !template result
		// (timeout, retries) must survive, keys the component overrides (enabled) must
		// win, and keys the component adds (new_key) must be present. Regression for the
		// broader gap behind https://github.com/cloudposse/atmos/issues/2888: today the
		// component's concrete map silently replaces the !template result outright
		// instead of deep-merging with it, dropping timeout/retries.
		templateConfig, ok := vars["template_config"].(map[string]interface{})
		require.True(t, ok, "vars.template_config should be a map")

		var expectedTemplateConfig map[string]interface{}
		require.NoError(t, json.Unmarshal(
			[]byte(`{"enabled":false,"timeout":30,"retries":3,"new_key":"new_value"}`),
			&expectedTemplateConfig,
		))
		assert.Equal(t, expectedTemplateConfig, templateConfig,
			"!template result must deep-merge with the component's own override, not be replaced by it")
	})

	t.Run("deep merges with yaml function at higher precedence (mirror of Test Case 5)", func(t *testing.T) {
		// Test Case 5b: the deferred function is the component-level (higher-precedence) layer and
		// the concrete map is the abstract-base (lower-precedence) layer — the reverse of "deep
		// merges with yaml functions" above. The deep-merge must be symmetric: it shouldn't matter
		// which side of the merge holds the unresolved function.
		componentName := "test-mirror-precedence"
		stack := "test"

		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component:            componentName,
				Stack:                stack,
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
			},
		)

		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		mirrorConfig, ok := vars["mirror_config"].(map[string]interface{})
		require.True(t, ok, "vars.mirror_config should be a map")

		assert.Equal(t, map[string]interface{}{
			"enabled":   true,
			"base_only": "yes",
			"new_key":   "new_value",
		}, mirrorConfig, "the higher-precedence !template result must deep-merge with the lower-precedence concrete base map, not replace it")
	})

	t.Run("deep merges with labels and tags functions (regression for #2888)", func(t *testing.T) {
		// https://github.com/cloudposse/atmos/issues/2888
		// !labels/!tags are fed by metadata.labels/metadata.tags and must deep-merge with
		// a concrete override at the same vars path, the same way !template already
		// claims to (see the "deep merges with yaml functions" subtest above).
		componentName := "test-labels-override"
		stack := "test"

		// See the ProcessTemplates/ProcessYamlFunctions comment on the "deep merges with yaml
		// functions" subtest above — describe component defaults both to false, and this
		// deep-merge behavior only exercises the deferred-merge resolution path when opted in.
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component:            componentName,
				Stack:                stack,
				ProcessYamlFunctions: true,
			},
		)

		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		tags, ok := vars["tags"].(map[string]interface{})
		require.True(t, ok, "vars.tags should be a map")
		assert.Equal(t, map[string]interface{}{
			"X": "1",
			"Y": "2",
			"Z": "override-value",
		}, tags, "!labels result must deep-merge with the component's own override, not be replaced by it")

		tagList, ok := vars["tag_list"].([]interface{})
		require.True(t, ok, "vars.tag_list should be a list")
		assert.Equal(t, []interface{}{"X", "Y", "Z"}, tagList,
			"!tags result must merge with the component's own override list, not be replaced by it, and must preserve precedence order")
	})

	t.Run("handles multiple yaml functions with precedence", func(t *testing.T) {
		// Test Case 6: Multiple YAML functions of same type
		// This component uses !env which should work without external dependencies
		componentName := "test-multiple-yaml-functions"
		stack := "test"

		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: componentName,
				Stack:     stack,
			},
		)

		// If component doesn't exist, skip this test
		if err != nil && strings.Contains(err.Error(), "Could not find the component") {
			t.Skip("test-multiple-yaml-functions component not found in fixture")
		}

		require.NoError(t, err, "deferred merge should handle multiple YAML functions")
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		// Verify vars contains expected fields
		assert.Contains(t, vars, "region")
	})

	t.Run("deep merges a deferred function nested inside a list item", func(t *testing.T) {
		// base-component-list-nested (list_merge_strategy: merge) sets servers[0] = {name: web,
		// stage: !env ATMOS_STAGE} and servers[1] = {name: db, stage: static}; the component
		// overrides servers[0] with {name: web, region: us-east-1} only. With merge strategy,
		// index 0 must deep-merge (env-resolved stage survives alongside the new region key)
		// and index 1, which has no override, must pass through unchanged.
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component:            "test-list-nested-function",
				Stack:                "test",
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
			},
		)
		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		servers, ok := vars["servers"].([]interface{})
		require.True(t, ok, "vars.servers should be a list")
		assert.Equal(t, []interface{}{
			map[string]interface{}{"name": "web", "stage": "test", "region": "us-east-1"},
			map[string]interface{}{"name": "db", "stage": "static"},
		}, servers, "the !env contribution at index 0 must survive the deep merge; index 1 must pass through untouched")
	})

	t.Run("resolves a 3-layer type flip to the highest-precedence layer", func(t *testing.T) {
		// base-component-typeflip (!template producing {a:1,b:2}) <- base-component-typeflip-mixin
		// (a plain scalar, via metadata.inherits) <- the component's own override (a map with
		// different keys). Pins current behavior for this precedence chain: the
		// highest-precedence map wins outright: the base layer's !template contribution does not
		// survive once an intermediate layer has type-flipped the path away from a map.
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component:            "test-typeflip-3layer",
				Stack:                "test",
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
			},
		)
		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		assert.Equal(t, map[string]interface{}{"c": "3", "highest": "wins"}, vars["flip_config"],
			"the component's own override must win the 3-layer type-flip chain")
	})

	t.Run("a plain scalar overrides a deferred function's own map-producing path", func(t *testing.T) {
		// base-component's !template-produced map at vars.template_config is overridden directly
		// (not at an ancestor path) by a plain string. A scalar always wins outright over a map —
		// there is nothing to deep-merge.
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component:            "test-scalar-overrides-map-function",
				Stack:                "test",
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
			},
		)
		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		assert.Equal(t, "just-a-plain-string", vars["template_config"],
			"a scalar override must replace the !template map outright")
	})

	t.Run("honors the default (replace) list_merge_strategy for !tags", func(t *testing.T) {
		// Mirror of "deep merges with labels and tags functions" but under the fixture-wide
		// DEFAULT list_merge_strategy (replace) instead of an explicit "append" opt-in: the
		// component's own tag_list must replace the !tags-produced list outright, not merge with
		// it. tags (a map) still deep-merges regardless of list_merge_strategy — the component
		// does not override it here, so the !labels result passes through unchanged.
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component:            "test-labels-override-default-strategy",
				Stack:                "test",
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
			},
		)
		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		assert.Equal(t, map[string]interface{}{"X": "1", "Y": "2"}, vars["tags"],
			"the unmodified !labels result must pass through")
		assert.Equal(t, []interface{}{"Z"}, vars["tag_list"],
			"under the default replace strategy, the override list must replace the !tags result, not merge with it")
	})

	t.Run("deep merges a parent-path function with a child-path function", func(t *testing.T) {
		// base-component-nested-collision's vars.combo is a !template-produced map ({a:1,b:2}); the
		// component overrides vars.combo.nested with its own !template and adds vars.combo.plain.
		// A parent-level deferred function and a child-level deferred function at the same
		// subtree must both resolve and deep-merge correctly, not clobber one another.
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component:            "test-nested-function-collision",
				Stack:                "test",
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
			},
		)
		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		assert.Equal(t, map[string]interface{}{
			"a":      "1",
			"b":      "2",
			"nested": "nested-value",
			"plain":  "concrete",
		}, vars["combo"], "the parent-path !template contribution and the child-path override (including its own !template) must all survive")
	})

	t.Run("an override still clobbers an untracked (non-deferred) function", func(t *testing.T) {
		// !git.root is deliberately NOT in the deferred-function allowlist (pkg/merge's
		// postMergeFunctions) — only 11 specific functions get the #2888 deep-merge protection.
		// This pins the known, by-design limitation: a concrete override at the same path as an
		// untracked function still replaces it outright, the original #2888 failure mode, rather
		// than deep-merging.
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component:            "test-untracked-function-override",
				Stack:                "test",
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
			},
		)
		require.NoError(t, err)
		require.NotNil(t, componentSection)

		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")

		assert.Equal(t, map[string]interface{}{"extra": "value"}, vars["git_info"],
			"an untracked function's contribution is not protected: the override replaces it outright")
	})

	t.Run("resolves and deep-merges a deferred function inside the backend section", func(t *testing.T) {
		// base-component-backend sets backend.s3.bucket to a !template expression and
		// backend.s3.key to a literal; the component adds backend.s3.region. backend is not one of
		// the sections resolveDeferredYamlFunctions is wired to, but since nothing overrides
		// `bucket` itself here, the document-wide Stage 2 pass (ProcessCustomYamlTags) alone
		// resolves it, and the ordinary structural merge combines the non-colliding keys.
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component:            "test-backend-override",
				Stack:                "test",
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
			},
		)
		require.NoError(t, err)
		require.NotNil(t, componentSection)

		backend, ok := componentSection["backend"].(map[string]interface{})
		require.True(t, ok, "backend should be a map")

		assert.Equal(t, "acme-us-east-1-tfstate", backend["bucket"], "the !template bucket expression must resolve")
		assert.Equal(t, "base/terraform.tfstate", backend["key"])
		assert.Equal(t, "us-east-1", backend["region"], "the component's own region addition must be present")
	})

	t.Run("loads all test cases without errors", func(t *testing.T) {
		// Verify all test components can be loaded without type conflicts
		components := []string{
			"test-yaml-to-concrete",
			"test-yaml-to-yaml",
			"test-list-yaml-to-concrete",
			"test-map-yaml-to-concrete",
			"test-deep-merge",
			"test-mirror-precedence",
			"test-labels-override",
			"test-list-nested-function",
			"test-typeflip-3layer",
			"test-scalar-overrides-map-function",
			"test-labels-override-default-strategy",
			"test-nested-function-collision",
			"test-untracked-function-override",
			"test-backend-override",
		}

		for _, componentName := range components {
			t.Run(componentName, func(t *testing.T) {
				componentSection, err := e.ExecuteDescribeComponent(
					&e.ExecuteDescribeComponentParams{
						Component: componentName,
						Stack:     "test",
					},
				)

				// Key test: no error means deferred merge prevented type conflicts
				require.NoError(t, err, "component %s should load without errors (deferred merge prevents type conflicts)", componentName)
				require.NotNil(t, componentSection)

				// Each component should have vars
				vars, ok := componentSection["vars"].(map[string]interface{})
				require.True(t, ok, "vars should be a map for component %s", componentName)
				require.NotEmpty(t, vars)
			})
		}
	})
}

// TestYAMLFunctionsDeferredMergeCacheCorrectness guards against a leaked/shared deferred-merge
// context: FindStacksMap caches its result (including the per-component ComponentDeferredContexts
// bundle) across calls within a process, and the Stage 3 resolution path
// (internal/exec/utils.go, ProcessStacks) must clone a section's context before resolving it —
// resolving one call's copy must not mutate the shared cached context that a later, independent
// call for the same (or a different) component will read.
func TestYAMLFunctionsDeferredMergeCacheCorrectness(t *testing.T) {
	t.Chdir("./fixtures/scenarios/atmos-yaml-functions-merge")
	t.Setenv("ATMOS_STAGE", "test")

	_, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	describeDeepMerge := func(t *testing.T) map[string]interface{} {
		t.Helper()
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component:            "test-deep-merge",
				Stack:                "test",
				ProcessTemplates:     true,
				ProcessYamlFunctions: true,
			},
		)
		require.NoError(t, err)
		require.NotNil(t, componentSection)
		vars, ok := componentSection["vars"].(map[string]interface{})
		require.True(t, ok, "vars should be a map")
		templateConfig, ok := vars["template_config"].(map[string]interface{})
		require.True(t, ok, "vars.template_config should be a map")
		return templateConfig
	}

	expected := map[string]interface{}{"enabled": false, "timeout": float64(30), "retries": float64(3), "new_key": "new_value"}

	t.Run("two independent describe-component calls for the same component resolve identically", func(t *testing.T) {
		first := describeDeepMerge(t)
		second := describeDeepMerge(t)

		assert.Equal(t, expected, first, "first call must resolve the full deep-merge")
		assert.Equal(t, expected, second, "second call must resolve identically — not corrupted or short-circuited by the first call's resolution")
	})

	t.Run("resolving a different component in between does not corrupt a shared cached context", func(t *testing.T) {
		before := describeDeepMerge(t)

		// Resolve a completely different component sharing the same cached stacksMap/deferredContexts.
		_, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component:            "test-labels-override",
				Stack:                "test",
				ProcessYamlFunctions: true,
			},
		)
		require.NoError(t, err)

		after := describeDeepMerge(t)

		assert.Equal(t, expected, before)
		assert.Equal(t, expected, after, "resolving a sibling component must not leak state into this component's cached deferred context")
	})
}

// TestDeferredMergeTypeConflictResolution tests that type conflicts are resolved.
// This is the core functionality that deferred merge provides.
func TestDeferredMergeTypeConflictResolution(t *testing.T) {
	t.Chdir("./fixtures/scenarios/atmos-yaml-functions-merge")

	_, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)

	t.Run("string to map conflict resolved", func(t *testing.T) {
		// Base catalog has !template (string)
		// Override has concrete map
		// Without deferred merge, this would fail
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: "test-yaml-to-concrete",
				Stack:     "test",
			},
		)

		require.NoError(t, err, "should resolve string->map conflict")
		require.NotNil(t, componentSection)
	})

	t.Run("string to list conflict resolved", func(t *testing.T) {
		// Base catalog has !terraform.output (string)
		// Override has concrete list
		// Without deferred merge, this would fail
		componentSection, err := e.ExecuteDescribeComponent(
			&e.ExecuteDescribeComponentParams{
				Component: "test-list-yaml-to-concrete",
				Stack:     "test",
			},
		)

		require.NoError(t, err, "should resolve string->list conflict")
		require.NotNil(t, componentSection)
	})
}
