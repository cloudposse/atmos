package exec

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestLintTargetsDeduplicatesComponentsDeterministically(t *testing.T) {
	targets := lintTargets(nil, []*dependency.Node{
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
