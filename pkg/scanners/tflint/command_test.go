package tflint

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/schema"
)

func testRuntime() *Runtime {
	return &Runtime{
		SetupAuth: func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
			return nil, nil
		},
		DescribeStacks: func(
			*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager, bool,
		) (map[string]any, error) {
			return map[string]any{}, nil
		},
		ProcessStacks: func(
			*schema.AtmosConfiguration, schema.ConfigAndStacksInfo, bool, bool, bool, []string, auth.AuthManager,
		) (schema.ConfigAndStacksInfo, error) {
			return schema.ConfigAndStacksInfo{}, nil
		},
		AffectedComponents: func(*schema.AtmosConfiguration, *AffectedOptions, auth.AuthManager) ([]schema.Affected, error) {
			return nil, nil
		},
	}
}

func stubInitCLIConfig(t *testing.T) {
	t.Helper()
	original := initCLIConfig
	initCLIConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	}
	t.Cleanup(func() { initCLIConfig = original })

	// checkTFLintAvailable hits the real toolchain/PATH; stub it so tests exercising
	// other behavior aren't at the mercy of whether tflint happens to be installed on
	// the machine running the test suite. TestCheckTFLintAvailable covers its real logic.
	originalCheck := checkTFLintAvailable
	checkTFLintAvailable = func(*schema.AtmosConfiguration) (string, error) { return "", nil }
	t.Cleanup(func() { checkTFLintAvailable = originalCheck })
}

// TestCheckTFLintAvailableImpl_NotOnPATH verifies the fast-fail check reports a
// well-formatted, hinted ErrCommandNotFound when tflint resolves nowhere (no
// .tool-versions pin, nothing on PATH) — the exact scenario that previously ran a full
// describe-stacks pass (and generated files for every component) before failing.
func TestCheckTFLintAvailableImpl_NotOnPATH(t *testing.T) {
	emptyPathDir := t.TempDir()
	t.Setenv("PATH", emptyPathDir)

	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	path, err := checkTFLintAvailableImpl(atmosConfig)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCommandNotFound)
	assert.ErrorContains(t, err, "tflint")
	assert.Empty(t, path)
}

// TestCheckTFLintAvailableImpl_OnPATH verifies the check passes (returns nil) when
// tflint resolves via the ambient PATH, so a genuinely-available tflint never blocks
// discovery. No .tool-versions pin is declared, so the resolved toolchain PATH is empty.
func TestCheckTFLintAvailableImpl_OnPATH(t *testing.T) {
	binDir := t.TempDir()
	toolName := Command
	if runtime.GOOS == "windows" {
		toolName += ".exe"
	}
	exe, err := os.Executable()
	require.NoError(t, err)
	require.NoError(t, os.Symlink(exe, filepath.Join(binDir, toolName)))
	t.Setenv("PATH", binDir)

	atmosConfig := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	path, err := checkTFLintAvailableImpl(atmosConfig)
	require.NoError(t, err)
	assert.Empty(t, path)
}

// TestCombineToolchainPATH verifies the project-wide (.tool-versions-resolved) toolchain
// PATH is combined with a target's own component-resolved PATH, taking priority over it,
// so a tool pinned solely via .tool-versions (with no matching stack/component
// `dependencies:` block) is still found by dependencies.ForComponent's PATH-unaware
// resolution. See checkTFLintAvailableImpl's doc comment for the full rationale.
func TestCombineToolchainPATH(t *testing.T) {
	sep := string(os.PathListSeparator)

	tests := []struct {
		name          string
		projectPATH   string
		componentPATH string
		want          string
	}{
		{name: "both empty", projectPATH: "", componentPATH: "", want: ""},
		{name: "project only", projectPATH: "/project/bin", componentPATH: "", want: "/project/bin"},
		{name: "component only", projectPATH: "", componentPATH: "/component/bin", want: "/component/bin"},
		{name: "both set, project takes priority", projectPATH: "/project/bin", componentPATH: "/component/bin", want: "/project/bin" + sep + "/component/bin"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, combineToolchainPATH(tt.projectPATH, tt.componentPATH))
		})
	}
}

// TestScopeToTFLint verifies the check only ever hands tflint's own .tool-versions
// entry to dependencies.NewEnvironmentFromDeps, never the other pinned tools — those
// would each trigger a real install attempt (see NewEnvironmentFromDeps/EnsureTools),
// turning a fast-fail check into a multi-minute one across an unrelated toolchain.
func TestScopeToTFLint(t *testing.T) {
	deps := map[string]string{
		"terraform": "1.9.0",
		"kubectl":   "1.31.0",
		Command:     "0.38.1",
	}

	scoped := scopeToTFLint(deps)

	assert.Equal(t, map[string]string{Command: "0.38.1"}, scoped)
}

// TestScopeToTFLint_NotDeclared verifies scoping returns an empty map (not the whole
// input) when tflint isn't declared in .tool-versions at all.
func TestScopeToTFLint_NotDeclared(t *testing.T) {
	deps := map[string]string{"terraform": "1.9.0"}

	scoped := scopeToTFLint(deps)

	assert.Empty(t, scoped)
}

func TestTargetsForDeduplicatesComponentsDeterministically(t *testing.T) {
	targets := targetsFor(nil, []*dependency.Node{
		nil,
		{Component: "", Stack: "dev"},
		{Component: "missing-stack"},
		{Component: "vpc", Stack: "prod"},
		{Component: "account", Stack: "prod"},
		{Component: "vpc", Stack: "dev"},
	})

	require.Len(t, targets, 2)
	assert.Equal(t, "account", targets[0].Component)
	assert.Equal(t, "prod", targets[0].Stack)
	assert.Equal(t, "vpc", targets[1].Component)
	assert.Equal(t, "dev", targets[1].Stack)
}

func TestExecuteRejectsMissingInputs(t *testing.T) {
	runtime := testRuntime()
	require.ErrorIs(t, Execute(context.Background(), runtime, nil, nil, 0), errUtils.ErrNilParam)
	require.ErrorIs(t, Execute(context.Background(), nil, &schema.ConfigAndStacksInfo{}, nil, 0), errUtils.ErrNilParam)
	runtime.AffectedComponents = nil
	require.ErrorIs(t, Execute(context.Background(), runtime, &schema.ConfigAndStacksInfo{}, &AffectedOptions{}, 0), errUtils.ErrNilParam)
}

func TestExecuteRoutesSortedUniqueTargets(t *testing.T) {
	stubInitCLIConfig(t)

	originalGraph := buildTerraformGraph
	buildTerraformGraph = func(map[string]any) (*dependency.Graph, error) {
		return &dependency.Graph{Nodes: map[string]*dependency.Node{
			"vpc-prod": {Component: "vpc", Stack: "prod"},
			"vpc-dev":  {Component: "vpc", Stack: "dev"},
			"app-dev":  {Component: "app", Stack: "dev"},
		}}, nil
	}
	t.Cleanup(func() { buildTerraformGraph = originalGraph })

	originalRun := runTarget
	var linted []string
	runTarget = func(_ context.Context, exec *targetExecution, target *dependency.Node) error {
		assert.Equal(t, "requested", exec.BaseInfo.ComponentFromArg)
		linted = append(linted, target.Component+":"+target.Stack)
		return nil
	}
	t.Cleanup(func() { runTarget = originalRun })

	require.NoError(t, Execute(context.Background(), testRuntime(), &schema.ConfigAndStacksInfo{ComponentFromArg: "requested", Stack: "dev"}, nil, 0))
	assert.Equal(t, []string{"app:dev", "vpc:dev"}, linted)
}

func TestExecuteDisablesComponentAuthDuringStackDiscovery(t *testing.T) {
	stubInitCLIConfig(t)

	originalGraph := buildTerraformGraph
	buildTerraformGraph = func(map[string]any) (*dependency.Graph, error) {
		return &dependency.Graph{}, nil
	}
	t.Cleanup(func() { buildTerraformGraph = originalGraph })

	runtime := testRuntime()
	runtime.SetupAuth = func(_ *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
		assert.True(t, info.AuthDisabled)
		assert.Equal(t, cfg.IdentityFlagDisabledValue, info.Identity)
		return nil, nil
	}
	called := false
	runtime.DescribeStacks = func(
		_ *schema.AtmosConfiguration, _ string, _ []string, _ []string, _ []string, _ bool, _ bool, processFunctions bool, _ bool, _ []string, _ auth.AuthManager, authDisabled bool,
	) (map[string]any, error) {
		called = true
		assert.True(t, authDisabled)
		// With auth disabled there's no AuthManager to reach a real backend
		// with, so !terraform.state/!terraform.output must not be evaluated —
		// they'd otherwise hit AWS unauthenticated instead of being skipped.
		assert.False(t, processFunctions, "ProcessFunctions must be forced off when auth is disabled")
		return map[string]any{}, nil
	}

	info := &schema.ConfigAndStacksInfo{Identity: cfg.IdentityFlagDisabledValue, ProcessFunctions: true}
	require.NoError(t, Execute(context.Background(), runtime, info, nil, 0))
	assert.True(t, called)
}

func TestExecuteReturnsSetupErrors(t *testing.T) {
	t.Run("config initialization", func(t *testing.T) {
		original := initCLIConfig
		want := errors.New("config failed")
		initCLIConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, want
		}
		t.Cleanup(func() { initCLIConfig = original })
		require.ErrorIs(t, Execute(context.Background(), testRuntime(), &schema.ConfigAndStacksInfo{}, nil, 0), want)
	})

	t.Run("authentication", func(t *testing.T) {
		stubInitCLIConfig(t)
		want := errors.New("authentication failed")
		runtime := testRuntime()
		runtime.SetupAuth = func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
			return nil, want
		}
		require.ErrorIs(t, Execute(context.Background(), runtime, &schema.ConfigAndStacksInfo{}, nil, 0), want)
	})

	t.Run("describe stacks", func(t *testing.T) {
		stubInitCLIConfig(t)
		want := errors.New("describe failed")
		runtime := testRuntime()
		runtime.DescribeStacks = func(
			*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager, bool,
		) (map[string]any, error) {
			return nil, want
		}
		require.ErrorIs(t, Execute(context.Background(), runtime, &schema.ConfigAndStacksInfo{}, nil, 0), want)
	})
}

func TestExecuteAffectedFiltersAndDeduplicatesTargets(t *testing.T) {
	stubInitCLIConfig(t)
	runtime := testRuntime()
	runtime.AffectedComponents = func(_ *schema.AtmosConfiguration, options *AffectedOptions, _ auth.AuthManager) ([]schema.Affected, error) {
		assert.True(t, options.AuthDisabled)
		assert.False(t, options.ProcessYamlFunctions, "ProcessYamlFunctions must be forced off when auth is disabled")
		return []schema.Affected{
			{Component: "vpc", Stack: "prod", ComponentType: cfg.TerraformComponentType},
			{Component: "vpc", Stack: "dev", ComponentType: cfg.TerraformComponentType},
			{Component: "helm", Stack: "dev", ComponentType: "helmfile"},
			{Component: "deleted", Stack: "dev", ComponentType: cfg.TerraformComponentType, Deleted: true},
		}, nil
	}

	originalRun := runTarget
	var linted []string
	runTarget = func(_ context.Context, _ *targetExecution, target *dependency.Node) error {
		linted = append(linted, target.Component+":"+target.Stack)
		return nil
	}
	t.Cleanup(func() { runTarget = originalRun })

	info := &schema.ConfigAndStacksInfo{Identity: cfg.IdentityFlagDisabledValue}
	require.NoError(t, Execute(context.Background(), runtime, info, &AffectedOptions{ProcessYamlFunctions: true}, 0))
	assert.Equal(t, []string{"vpc:dev"}, linted)
	assert.True(t, info.AuthDisabled)
}

func TestTargetsForReadsAndDeduplicatesGraphs(t *testing.T) {
	graph := &dependency.Graph{Nodes: map[string]*dependency.Node{
		"vpc-prod": {Component: "vpc", Stack: "prod"},
		"app-prod": {Component: "app", Stack: "prod"},
		"vpc-dev":  {Component: "vpc", Stack: "dev"},
	}}

	targets := targetsFor(graph, []*dependency.Node{{Component: "ignored", Stack: "ignored"}})
	require.Len(t, targets, 2)
	assert.Equal(t, "app", targets[0].Component)
	assert.Equal(t, "vpc", targets[1].Component)
	assert.Equal(t, "dev", targets[1].Stack)
}

func TestTargetErrorIncludesTargetAndCause(t *testing.T) {
	cause := errors.New("scanner failed")
	err := targetError(&dependency.Node{Component: "vpc", Stack: "prod"}, "running TFLint", cause)
	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, errUtils.ErrTerraformLint)
	require.ErrorContains(t, err, `component "vpc" in stack "prod"`)
	require.ErrorContains(t, err, "running TFLint")
}

func TestExecuteTargetsContinuesAfterFailures(t *testing.T) {
	firstErr := errors.New("first lint failure")
	secondErr := errors.New("second lint failure")
	originalRun := runTarget
	var linted []string
	runTarget = func(_ context.Context, _ *targetExecution, target *dependency.Node) error {
		linted = append(linted, target.Component)
		switch target.Component {
		case "first":
			return firstErr
		case "second":
			return secondErr
		default:
			return nil
		}
	}
	t.Cleanup(func() { runTarget = originalRun })

	err := executeTargets(context.Background(), &targetExecution{Runtime: testRuntime(), AtmosConfig: &schema.AtmosConfiguration{}, BaseInfo: &schema.ConfigAndStacksInfo{}}, []*dependency.Node{
		{Component: "first", Stack: "dev"},
		{Component: "second", Stack: "dev"},
		{Component: "third", Stack: "dev"},
	})

	assert.Equal(t, []string{"first", "second", "third"}, linted)
	require.ErrorIs(t, err, firstErr)
	require.ErrorIs(t, err, secondErr)

	// The returned error's own Error() must stay a short, count-based summary — not
	// every joined per-target message concatenated — since the top-level CLI error
	// formatter can't traverse a joined multi-error and would otherwise print the
	// whole concatenation verbatim. See lintAggregateError's doc comment.
	assert.Equal(t, "2 of 3 component(s) failed to lint", err.Error())
	assert.NotContains(t, err.Error(), firstErr.Error())
	assert.NotContains(t, err.Error(), secondErr.Error())
}

// TestLintAggregateError_UnwrapsForErrorsIsWithoutLongText verifies
// lintAggregateError directly: Unwrap() []error makes errors.Is/errors.As traverse
// into every wrapped cause, while Error() itself never grows with the number or
// length of those causes.
func TestLintAggregateError_UnwrapsForErrorsIsWithoutLongText(t *testing.T) {
	causeA := errors.New("cause A: " + strings.Repeat("x", 200))
	causeB := errors.New("cause B: " + strings.Repeat("y", 200))
	err := &lintAggregateError{errs: []error{causeA, causeB}, failed: 2, total: 5}

	require.ErrorIs(t, err, causeA)
	require.ErrorIs(t, err, causeB)
	assert.Equal(t, "2 of 5 component(s) failed to lint", err.Error())
	assert.Less(t, len(err.Error()), 60)
}
