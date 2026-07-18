package exec

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/dependency"
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
