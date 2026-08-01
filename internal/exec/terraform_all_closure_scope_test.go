package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestDescribeTerraformStacksForExecution_ClosureRequested exercises the
// closure-requested branches of describeTerraformStacksForExecution that
// ExecuteTerraformAll's default (--all, no closure flags) tests never reach:
// the unbounded-selection fallback and the bounded scoped-closure path backed
// by pkg/list/dependencies.ResolveScopedClosure. Both run against the real
// terraform-apply-affected fixture (vpc -> eks/cluster -> eks/istio/base ->
// eks/istio/istiod -> eks/istio/test-app dependency chain) so the closure
// math itself is asserted, not just which branch was taken.
func TestDescribeTerraformStacksForExecution_ClosureRequested(t *testing.T) {
	t.Setenv("ATMOS_BASE_PATH", "")
	t.Setenv("ATMOS_CLI_CONFIG_PATH", "")
	os.Unsetenv("ATMOS_BASE_PATH")
	os.Unsetenv("ATMOS_CLI_CONFIG_PATH")

	t.Chdir(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "terraform-apply-affected"))

	t.Run("unbounded selection with closure requested falls back to a full describe", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentType:       "terraform",
			SubCommand:          "plan",
			IncludeDependencies: -1, // Unlimited forward closure requested, but no stack/components/tags/labels seed it.
		}
		atmosConfig, err := cfg.InitCliConfig(*info, true)
		require.NoError(t, err)

		stacks, err := describeTerraformStacksForExecution(&atmosConfig, info, nil, nil)
		require.NoError(t, err)

		// Unbounded: every stack the fixture defines comes back, matching the
		// non-closure describe path exactly -- the "not bounded" escape hatch
		// avoids paying for ResolveScopedClosure when it would only redo the
		// same full-repo work.
		require.Contains(t, stacks, "prod")
		require.Contains(t, stacks, "nonprod")
		prodComponents := terraformComponentNames(t, stacks, "prod")
		require.Contains(t, prodComponents, "vpc")
		require.Contains(t, prodComponents, "eks/istio/test-app")
	})

	t.Run("bounded selection resolves only the reachable dependency closure", func(t *testing.T) {
		info := &schema.ConfigAndStacksInfo{
			ComponentType:       "terraform",
			SubCommand:          "plan",
			Stack:               "prod",
			IncludeDependencies: -1, // Unlimited forward (dependencies) closure.
			ProcessTemplates:    true,
		}
		components := []string{"eks/istio/test-app"}
		atmosConfig, err := cfg.InitCliConfig(*info, true)
		require.NoError(t, err)

		stacks, err := describeTerraformStacksForExecution(&atmosConfig, info, nil, components)
		require.NoError(t, err)

		// Bounded + evaluation requested + eager off routes through
		// ResolveScopedClosure: only "prod" (the seed stack) comes back, and
		// only the seed plus its forward dependency chain -- vpc, eks/cluster,
		// eks/istio/base, eks/istio/istiod -- not unrelated siblings like
		// eks/karpenter or eks/external-dns.
		require.Len(t, stacks, 1)
		prodComponents := terraformComponentNames(t, stacks, "prod")
		require.ElementsMatch(t, []string{
			"vpc",
			"eks/cluster",
			"eks/istio/base",
			"eks/istio/istiod",
			"eks/istio/test-app",
		}, prodComponents)
	})
}

// TestDescribeTerraformStacksForExecution_ClosureErrorPropagates proves
// describeTerraformStacksForExecution surfaces a genuine evaluation error from
// listdeps.ResolveScopedClosure instead of swallowing it: seeding by the
// `broken-tag` tag (list-components-closure-tags fixture) pulls in a
// component whose template unconditionally fails to render once evaluated.
func TestDescribeTerraformStacksForExecution_ClosureErrorPropagates(t *testing.T) {
	t.Setenv("ATMOS_BASE_PATH", "")
	t.Setenv("ATMOS_CLI_CONFIG_PATH", "")
	os.Unsetenv("ATMOS_BASE_PATH")
	os.Unsetenv("ATMOS_CLI_CONFIG_PATH")

	t.Chdir(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "list-components-closure-tags"))

	info := &schema.ConfigAndStacksInfo{
		ComponentType:       "terraform",
		SubCommand:          "plan",
		Stack:               "broken",
		IncludeDependencies: -1,
		ProcessTemplates:    true,
	}
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	require.NoError(t, err)

	_, err = describeTerraformStacksForExecution(&atmosConfig, info, nil, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "broken component must only be evaluated when explicitly seeded")
}

// terraformComponentNames returns the terraform component names present in
// the given stack's describe-stacks output.
func terraformComponentNames(t *testing.T, stacks map[string]any, stackName string) []string {
	t.Helper()

	stackSection, ok := stacks[stackName].(map[string]any)
	require.True(t, ok, "stack %q missing from describe output", stackName)
	componentsSection, ok := stackSection["components"].(map[string]any)
	require.True(t, ok, "stack %q missing components section", stackName)
	terraformSection, ok := componentsSection["terraform"].(map[string]any)
	require.True(t, ok, "stack %q missing terraform components", stackName)

	names := make([]string, 0, len(terraformSection))
	for name := range terraformSection {
		names = append(names, name)
	}
	return names
}
