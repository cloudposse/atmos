package exec

import (
	"errors"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/dependency"
	scheduleradapters "github.com/cloudposse/atmos/pkg/scheduler/adapters"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestLintTargetsDeduplicatesComponentsDeterministically(t *testing.T) {
	targets := lintTargets(nil, []*dependency.Node{
		nil,
		{Component: "", Stack: "dev"},
		{Component: "missing-stack"},
		{Component: "vpc", Stack: "prod"},
		{Component: "account", Stack: "prod"},
		{Component: "vpc", Stack: "dev"},
	})

	require.Len(t, targets, 2)
	require.Equal(t, "account", targets[0].Component)
	require.Equal(t, "prod", targets[0].Stack)
	require.Equal(t, "vpc", targets[1].Component)
	require.Equal(t, "dev", targets[1].Stack)
}

func TestExecuteTerraformLintRejectsNilInfo(t *testing.T) {
	require.ErrorIs(t, ExecuteTerraformLint(nil), errUtils.ErrNilParam)
	require.ErrorIs(t, ExecuteTerraformLintAffected(nil, &schema.ConfigAndStacksInfo{}), errUtils.ErrNilParam)
	require.ErrorIs(t, ExecuteTerraformLintAffected(&DescribeAffectedCmdArgs{}, nil), errUtils.ErrNilParam)
}

func TestExecuteTerraformLintRoutesSortedUniqueTargets(t *testing.T) {
	skipGomonkeyOnDarwinARM64(t)

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyFunc(cfg.InitCliConfig, func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	})
	patches.ApplyFunc(ExecuteDescribeStacks, func(
		*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager,
	) (map[string]any, error) {
		return map[string]any{}, nil
	})
	patches.ApplyFunc(scheduleradapters.BuildTerraformGraph, func(map[string]any) (*dependency.Graph, error) {
		return &dependency.Graph{Nodes: map[string]*dependency.Node{
			"vpc-prod": {Component: "vpc", Stack: "prod"},
			"vpc-dev":  {Component: "vpc", Stack: "dev"},
			"app-dev":  {Component: "app", Stack: "dev"},
		}}, nil
	})

	originalFactory := authManagerFactory
	authManagerFactory = func(string, schema.AuthConfig, string, *schema.AtmosConfiguration) (auth.AuthManager, error) {
		return nil, nil
	}
	t.Cleanup(func() { authManagerFactory = originalFactory })
	originalRun := runTerraformLintTarget
	t.Cleanup(func() { runTerraformLintTarget = originalRun })
	var linted []string
	runTerraformLintTarget = func(_ *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, target *dependency.Node, _ auth.AuthManager) error {
		assert.Equal(t, "requested", info.ComponentFromArg)
		linted = append(linted, target.Component+":"+target.Stack)
		return nil
	}

	require.NoError(t, ExecuteTerraformLint(&schema.ConfigAndStacksInfo{ComponentFromArg: "requested", Stack: "dev"}))
	require.Equal(t, []string{"app:dev", "vpc:dev"}, linted)
}

func TestExecuteTerraformLintReturnsSetupErrors(t *testing.T) {
	skipGomonkeyOnDarwinARM64(t)

	t.Run("config initialization", func(t *testing.T) {
		patches := gomonkey.NewPatches()
		defer patches.Reset()
		want := errors.New("config failed")
		patches.ApplyFunc(cfg.InitCliConfig, func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, want
		})
		require.ErrorIs(t, ExecuteTerraformLint(&schema.ConfigAndStacksInfo{}), want)
	})

	t.Run("authentication", func(t *testing.T) {
		patches := gomonkey.NewPatches()
		defer patches.Reset()
		patches.ApplyFunc(cfg.InitCliConfig, func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		})
		want := errors.New("authentication failed")
		original := authManagerFactory
		authManagerFactory = func(string, schema.AuthConfig, string, *schema.AtmosConfiguration) (auth.AuthManager, error) {
			return nil, want
		}
		t.Cleanup(func() { authManagerFactory = original })
		require.ErrorIs(t, ExecuteTerraformLint(&schema.ConfigAndStacksInfo{}), want)
	})

	t.Run("describe stacks", func(t *testing.T) {
		patches := gomonkey.NewPatches()
		defer patches.Reset()
		patches.ApplyFunc(cfg.InitCliConfig, func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, nil
		})
		patches.ApplyFunc(ExecuteDescribeStacks, func(
			*schema.AtmosConfiguration, string, []string, []string, []string, bool, bool, bool, bool, []string, auth.AuthManager,
		) (map[string]any, error) {
			return nil, errors.New("describe failed")
		})
		original := authManagerFactory
		authManagerFactory = func(string, schema.AuthConfig, string, *schema.AtmosConfiguration) (auth.AuthManager, error) {
			return nil, nil
		}
		t.Cleanup(func() { authManagerFactory = original })
		require.ErrorContains(t, ExecuteTerraformLint(&schema.ConfigAndStacksInfo{}), "describe failed")
	})
}

func TestExecuteTerraformLintAffectedFiltersAndDeduplicatesTargets(t *testing.T) {
	skipGomonkeyOnDarwinARM64(t)

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyFunc(cfg.InitCliConfig, func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{}, nil
	})
	patches.ApplyFunc(getAffectedComponents, func(args *DescribeAffectedCmdArgs) ([]schema.Affected, error) {
		require.NotNil(t, args.CLIConfig)
		return []schema.Affected{
			{Component: "vpc", Stack: "prod", ComponentType: cfg.TerraformComponentType},
			{Component: "vpc", Stack: "dev", ComponentType: cfg.TerraformComponentType},
			{Component: "helm", Stack: "dev", ComponentType: "helmfile"},
			{Component: "deleted", Stack: "dev", ComponentType: cfg.TerraformComponentType, Deleted: true},
		}, nil
	})
	originalFactory := authManagerFactory
	authManagerFactory = func(string, schema.AuthConfig, string, *schema.AtmosConfiguration) (auth.AuthManager, error) {
		return nil, nil
	}
	t.Cleanup(func() { authManagerFactory = originalFactory })
	originalRun := runTerraformLintTarget
	t.Cleanup(func() { runTerraformLintTarget = originalRun })
	var linted []string
	runTerraformLintTarget = func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo, target *dependency.Node, _ auth.AuthManager) error {
		linted = append(linted, target.Component+":"+target.Stack)
		return nil
	}

	args := &DescribeAffectedCmdArgs{}
	info := &schema.ConfigAndStacksInfo{Identity: cfg.IdentityFlagDisabledValue}
	require.NoError(t, ExecuteTerraformLintAffected(args, info))
	require.Equal(t, []string{"vpc:dev"}, linted)
	require.True(t, args.AuthDisabled)
	require.True(t, info.AuthDisabled)
}

func TestLintTargetsReadsAndDeduplicatesGraphs(t *testing.T) {
	graph := &dependency.Graph{Nodes: map[string]*dependency.Node{
		"vpc-prod": {Component: "vpc", Stack: "prod"},
		"app-prod": {Component: "app", Stack: "prod"},
		"vpc-dev":  {Component: "vpc", Stack: "dev"},
	}}

	targets := lintTargets(graph, []*dependency.Node{{Component: "ignored", Stack: "ignored"}})
	require.Len(t, targets, 2)
	require.Equal(t, "app", targets[0].Component)
	require.Equal(t, "vpc", targets[1].Component)
	require.Equal(t, "dev", targets[1].Stack)
}

func TestTerraformLintTargetErrorIncludesTargetAndCause(t *testing.T) {
	cause := errors.New("scanner failed")
	err := terraformLintTargetError(&dependency.Node{Component: "vpc", Stack: "prod"}, "running TFLint", cause)
	require.ErrorIs(t, err, cause)
	require.ErrorIs(t, err, errUtils.ErrTerraformLint)
	require.ErrorContains(t, err, `component "vpc" in stack "prod"`)
	require.ErrorContains(t, err, "running TFLint")
}

func TestExecuteTerraformLintTargetsContinuesAfterFailures(t *testing.T) {
	firstErr := errors.New("first lint failure")
	secondErr := errors.New("second lint failure")
	original := runTerraformLintTarget
	t.Cleanup(func() { runTerraformLintTarget = original })

	var linted []string
	runTerraformLintTarget = func(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo, target *dependency.Node, _ auth.AuthManager) error {
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

	err := executeTerraformLintTargets(&schema.AtmosConfiguration{}, &schema.ConfigAndStacksInfo{}, []*dependency.Node{
		{Component: "first", Stack: "dev"},
		{Component: "second", Stack: "dev"},
		{Component: "third", Stack: "dev"},
	}, nil)

	require.Equal(t, []string{"first", "second", "third"}, linted)
	require.ErrorIs(t, err, firstErr)
	require.ErrorIs(t, err, secondErr)
}
