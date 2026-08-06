package list

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListStacksWithOptions_ComponentFilter exercises executeAndExtractStacks'
// non-closure branch with opts.Component set, covering the
// extract.StacksForComponent call (as opposed to extract.Stacks, used when no
// component filter is given — already covered by
// TestListStacksWithOptions_CoverageIntegration).
func TestListStacksWithOptions_ComponentFilter(t *testing.T) {
	initExecutorTestIO(t)
	chdirToCompleteFixture(t)

	cmd := newCmdWithListParser("stacks", stacksParser.RegisterFlags)
	// See TestSettingsCmd_RunE_CoverageIntegration: other tests in this package
	// leak a viper "identity" value via the shared viper.GetViper() singleton;
	// this fixture configures no auth, so identity resolution must be disabled
	// explicitly to keep this test's outcome independent of test ordering.
	require.NoError(t, cmd.Flags().Set("identity", "false"))
	opts := &StacksOptions{
		Format:           "json",
		Component:        "vpc",
		ProcessTemplates: true,
		ProcessFunctions: false,
	}

	require.NoError(t, listStacksWithOptions(cmd, []string{}, opts),
		"filtering by an existing component should list its stacks cleanly")
}

// TestListStacksWithOptions_ClosurePreview exercises extractStacksViaScopedClosure
// end to end: seeding the closure by --tags=istio and expanding with
// --include-dependencies must return only the stack the closure touches, with
// its stacksMap narrowed to the istio chain plus forward prerequisites — never
// eks/karpenter, which shares the stack but sits outside the chain.
func TestListStacksWithOptions_ClosurePreview(t *testing.T) {
	initExecutorTestIO(t)
	chdirToListComponentsClosureFixture(t)

	cmd := newCmdWithListParser("stacks", stacksParser.RegisterFlags)
	// See TestSettingsCmd_RunE_CoverageIntegration: other tests in this package
	// leak a viper "identity" value via the shared viper.GetViper() singleton;
	// this fixture configures no auth, so identity resolution must be disabled
	// explicitly to keep this test's outcome independent of test ordering.
	require.NoError(t, cmd.Flags().Set("identity", "false"))
	opts := &StacksOptions{
		Format:              "json",
		Component:           "eks/istio/istiod",
		Tags:                []string{"istio"},
		IncludeDependencies: -1,
		ProcessTemplates:    true,
		ProcessFunctions:    false,
	}

	atmosConfig, authManager, err := initStacksConfig(cmd, []string{}, opts)
	require.NoError(t, err)
	errOpts, _ := describeStacksErrorOptions(opts.ErrorMode)

	rows, stacksMap, err := executeAndExtractStacks(&atmosConfig, opts, authManager, errOpts)
	require.NoError(t, err)
	require.NotEmpty(t, rows)

	require.Contains(t, stacksMap, "prod")
	prodComponents := stacksMapTerraformComponentNames(t, stacksMap, "prod")
	assert.ElementsMatch(t, []string{
		"vpc",
		"eks/cluster",
		"eks/istio/base",
		"eks/istio/istiod",
	}, prodComponents)

	// The closure path must mirror extract.StacksForComponent's row shape:
	// with --component set, rows for stacks containing the component carry it
	// under "component" so `{{ .component }}` columns render identically with
	// and without closure flags.
	for _, row := range rows {
		if row["stack"] == "prod" {
			assert.Equal(t, "eks/istio/istiod", row["component"],
				"closure rows for stacks containing the selected component must carry it")
		}
	}
}

// TestListStacksWithOptions_ClosurePreviewPropagatesError proves
// extractStacksViaScopedClosure surfaces a genuine evaluation error from
// dependencies.ResolveScopedClosure instead of swallowing it: seeding by the
// `broken-tag` tag pulls in the fixture's `broken` component, whose template
// unconditionally fails to render once evaluated.
func TestListStacksWithOptions_ClosurePreviewPropagatesError(t *testing.T) {
	initExecutorTestIO(t)
	chdirToListComponentsClosureFixture(t)

	cmd := newCmdWithListParser("stacks", stacksParser.RegisterFlags)
	require.NoError(t, cmd.Flags().Set("identity", "false"))
	opts := &StacksOptions{
		Format:              "json",
		ErrorMode:           "strict", // Force a hard failure instead of the default warn-mode degradation.
		Tags:                []string{"broken-tag"},
		IncludeDependencies: -1,
		ProcessTemplates:    true,
		ProcessFunctions:    false,
	}

	atmosConfig, authManager, err := initStacksConfig(cmd, []string{}, opts)
	require.NoError(t, err)
	errOpts, _ := describeStacksErrorOptions(opts.ErrorMode)

	_, _, err = executeAndExtractStacks(&atmosConfig, opts, authManager, errOpts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broken component must only be evaluated when explicitly seeded")
}

// stacksMapTerraformComponentNames returns the terraform component names
// present in the given stack's describe-stacks output.
func stacksMapTerraformComponentNames(t *testing.T, stacksMap map[string]any, stackName string) []string {
	t.Helper()

	stackSection, ok := stacksMap[stackName].(map[string]any)
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

// TestListStacksWithOptions_ClosurePreviewExcludesConservativelyEvaluatedStacks
// is the regression test for restricting `list stacks` output to the FINAL
// closure: in the dependents direction the scoped-closure engine conservatively
// evaluates components whose dependency declarations are templated (so their
// edges can materialize — see dependencies.UnresolvedDependencySources), which
// lands their stacks in the resolved stacks map even when the resolved edge
// points outside the closure. The fixture's `qa` stack resolves its `batch`
// dependency to eks/karpenter — outside the istio dependents closure — and
// must not be listed.
func TestListStacksWithOptions_ClosurePreviewExcludesConservativelyEvaluatedStacks(t *testing.T) {
	initExecutorTestIO(t)
	chdirToListComponentsClosureFixture(t)

	cmd := newCmdWithListParser("stacks", stacksParser.RegisterFlags)
	// See TestSettingsCmd_RunE_CoverageIntegration: other tests in this package
	// leak a viper "identity" value via the shared viper.GetViper() singleton;
	// this fixture configures no auth, so identity resolution must be disabled
	// explicitly to keep this test's outcome independent of test ordering.
	require.NoError(t, cmd.Flags().Set("identity", "false"))
	opts := &StacksOptions{
		Format:            "json",
		Tags:              []string{"istio"},
		IncludeDependents: -1,
		ProcessTemplates:  true,
		ProcessFunctions:  false,
	}

	atmosConfig, authManager, err := initStacksConfig(cmd, []string{}, opts)
	require.NoError(t, err)
	errOpts, _ := describeStacksErrorOptions(opts.ErrorMode)

	rows, stacksMap, err := executeAndExtractStacks(&atmosConfig, opts, authManager, errOpts)
	require.NoError(t, err)
	require.NotEmpty(t, rows)

	require.Contains(t, stacksMap, "prod")
	assert.NotContains(t, stacksMap, "qa",
		"a conservatively evaluated stack outside the final closure must not be listed")
	for _, row := range rows {
		assert.NotEqual(t, "qa", row["stack"],
			"row for a stack outside the final closure must not be rendered")
	}
}
