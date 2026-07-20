package tflint

import (
	"context"
	"errors"
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
			*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager,
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
	require.ErrorIs(t, Execute(context.Background(), runtime, nil, nil), errUtils.ErrNilParam)
	require.ErrorIs(t, Execute(context.Background(), nil, &schema.ConfigAndStacksInfo{}, nil), errUtils.ErrNilParam)
	runtime.AffectedComponents = nil
	require.ErrorIs(t, Execute(context.Background(), runtime, &schema.ConfigAndStacksInfo{}, &AffectedOptions{}), errUtils.ErrNilParam)
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
	runTarget = func(_ context.Context, _ *Runtime, _ *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, target *dependency.Node, _ auth.AuthManager) error {
		assert.Equal(t, "requested", info.ComponentFromArg)
		linted = append(linted, target.Component+":"+target.Stack)
		return nil
	}
	t.Cleanup(func() { runTarget = originalRun })

	require.NoError(t, Execute(context.Background(), testRuntime(), &schema.ConfigAndStacksInfo{ComponentFromArg: "requested", Stack: "dev"}, nil))
	assert.Equal(t, []string{"app:dev", "vpc:dev"}, linted)
}

func TestExecuteReturnsSetupErrors(t *testing.T) {
	t.Run("config initialization", func(t *testing.T) {
		original := initCLIConfig
		want := errors.New("config failed")
		initCLIConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, want
		}
		t.Cleanup(func() { initCLIConfig = original })
		require.ErrorIs(t, Execute(context.Background(), testRuntime(), &schema.ConfigAndStacksInfo{}, nil), want)
	})

	t.Run("authentication", func(t *testing.T) {
		stubInitCLIConfig(t)
		want := errors.New("authentication failed")
		runtime := testRuntime()
		runtime.SetupAuth = func(*schema.AtmosConfiguration, *schema.ConfigAndStacksInfo) (auth.AuthManager, error) {
			return nil, want
		}
		require.ErrorIs(t, Execute(context.Background(), runtime, &schema.ConfigAndStacksInfo{}, nil), want)
	})

	t.Run("describe stacks", func(t *testing.T) {
		stubInitCLIConfig(t)
		want := errors.New("describe failed")
		runtime := testRuntime()
		runtime.DescribeStacks = func(
			*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager,
		) (map[string]any, error) {
			return nil, want
		}
		require.ErrorIs(t, Execute(context.Background(), runtime, &schema.ConfigAndStacksInfo{}, nil), want)
	})
}

func TestExecuteAffectedFiltersAndDeduplicatesTargets(t *testing.T) {
	stubInitCLIConfig(t)
	runtime := testRuntime()
	runtime.AffectedComponents = func(_ *schema.AtmosConfiguration, options *AffectedOptions, _ auth.AuthManager) ([]schema.Affected, error) {
		assert.True(t, options.AuthDisabled)
		return []schema.Affected{
			{Component: "vpc", Stack: "prod", ComponentType: cfg.TerraformComponentType},
			{Component: "vpc", Stack: "dev", ComponentType: cfg.TerraformComponentType},
			{Component: "helm", Stack: "dev", ComponentType: "helmfile"},
			{Component: "deleted", Stack: "dev", ComponentType: cfg.TerraformComponentType, Deleted: true},
		}, nil
	}

	originalRun := runTarget
	var linted []string
	runTarget = func(_ context.Context, _ *Runtime, _ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo, target *dependency.Node, _ auth.AuthManager) error {
		linted = append(linted, target.Component+":"+target.Stack)
		return nil
	}
	t.Cleanup(func() { runTarget = originalRun })

	info := &schema.ConfigAndStacksInfo{Identity: cfg.IdentityFlagDisabledValue}
	require.NoError(t, Execute(context.Background(), runtime, info, &AffectedOptions{}))
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
	runTarget = func(_ context.Context, _ *Runtime, _ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo, target *dependency.Node, _ auth.AuthManager) error {
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

	err := executeTargets(context.Background(), testRuntime(), &schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, []*dependency.Node{
		{Component: "first", Stack: "dev"},
		{Component: "second", Stack: "dev"},
		{Component: "third", Stack: "dev"},
	}, nil)

	assert.Equal(t, []string{"first", "second", "third"}, linted)
	require.ErrorIs(t, err, firstErr)
	require.ErrorIs(t, err, secondErr)
}
