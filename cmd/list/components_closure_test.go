package list

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tests"
)

// listComponentsClosureFixturePath is a vpc -> eks/cluster -> eks/istio/*
// dependency chain tagged network/eks/istio, plus an unrelated eks/karpenter
// component (tagged eks but outside the istio chain) that a --tags=istio
// closure preview must not pull in.
var listComponentsClosureFixturePath = filepath.Join("..", "..", "tests", "fixtures", "scenarios", "list-components-closure-tags")

func chdirToListComponentsClosureFixture(t *testing.T) {
	t.Helper()
	tests.RequireFilePath(t, listComponentsClosureFixturePath, "test fixture directory")
	t.Chdir(listComponentsClosureFixturePath)
}

// TestInitAndExtractComponents_ClosurePreview exercises
// extractComponentsViaScopedClosure end to end: seeding the closure by
// --tags=istio and expanding with --include-dependencies must return exactly
// the istio components plus their forward prerequisites (eks/cluster, vpc) —
// never an unrelated team's component (eks/karpenter) that merely shares the
// stack and a tag prefix but is outside the istio dependency chain.
func TestInitAndExtractComponents_ClosurePreview(t *testing.T) {
	initExecutorTestIO(t)
	chdirToListComponentsClosureFixture(t)

	cmd := newCmdWithListParser("components", componentsParser.RegisterFlags)
	// See TestSettingsCmd_RunE_CoverageIntegration: other tests in this package
	// leak a viper "identity" value via the shared viper.GetViper() singleton;
	// this fixture configures no auth, so identity resolution must be disabled
	// explicitly to keep this test's outcome independent of test ordering.
	require.NoError(t, cmd.Flags().Set("identity", "false"))
	opts := &ComponentsOptions{
		Format:              "json",
		Stack:               "prod",
		Tags:                []string{"istio"},
		IncludeDependencies: -1,
		ProcessTemplates:    true,
		ProcessFunctions:    false,
	}

	result, err := initAndExtractComponents(cmd, []string{}, opts)
	require.NoError(t, err)

	names := make([]string, 0, len(result.components))
	for _, component := range result.components {
		name, _ := component["component"].(string)
		names = append(names, name)
	}

	assert.ElementsMatch(t, []string{
		"vpc",
		"eks/cluster",
		"eks/istio/base",
		"eks/istio/istiod",
	}, names)
}

// TestInitAndExtractComponents_ClosurePreviewNoMatch proves an empty closure
// (a tag that matches nothing) returns zero components rather than falling
// back to the unscoped set.
func TestInitAndExtractComponents_ClosurePreviewNoMatch(t *testing.T) {
	initExecutorTestIO(t)
	chdirToListComponentsClosureFixture(t)

	cmd := newCmdWithListParser("components", componentsParser.RegisterFlags)
	require.NoError(t, cmd.Flags().Set("identity", "false"))
	opts := &ComponentsOptions{
		Format:              "json",
		Stack:               "prod",
		Tags:                []string{"nonexistent-team"},
		IncludeDependencies: -1,
		ProcessTemplates:    true,
		ProcessFunctions:    false,
	}

	result, err := initAndExtractComponents(cmd, []string{}, opts)
	require.NoError(t, err)
	assert.Empty(t, result.components)
}

// TestInitAndExtractComponents_ClosurePreviewPropagatesError proves
// extractComponentsViaScopedClosure surfaces a genuine evaluation error from
// dependencies.ResolveScopedClosure instead of swallowing it: seeding by the
// `broken-tag` tag pulls in the fixture's `broken` component, whose template
// unconditionally fails to render once evaluated. Mirrors
// TestListStacksWithOptions_ClosurePreviewPropagatesError
// (stacks_closure_test.go) for the components command's own closure wiring.
func TestInitAndExtractComponents_ClosurePreviewPropagatesError(t *testing.T) {
	initExecutorTestIO(t)
	chdirToListComponentsClosureFixture(t)

	cmd := newCmdWithListParser("components", componentsParser.RegisterFlags)
	require.NoError(t, cmd.Flags().Set("identity", "false"))
	opts := &ComponentsOptions{
		Format:              "json",
		ErrorMode:           "strict", // Force a hard failure instead of the default warn-mode degradation.
		Tags:                []string{"broken-tag"},
		IncludeDependencies: -1,
		ProcessTemplates:    true,
		ProcessFunctions:    false,
	}

	_, err := initAndExtractComponents(cmd, []string{}, opts)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broken component must only be evaluated when explicitly seeded")
}
