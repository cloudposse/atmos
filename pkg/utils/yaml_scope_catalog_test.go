package utils

import (
	"testing"

	fntag "github.com/cloudposse/atmos/pkg/function/tag"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestScaffoldScopeMatchesActualPolicy(t *testing.T) {
	t.Setenv("ATMOS_SCOPE_TEST", "resolved")
	for _, tc := range []struct {
		name, args string
		allowed    bool
	}{
		{"env", "ATMOS_SCOPE_TEST", true},
		{"cwd", "", true},
		{"random", "1 2", true},
		{"exec", "echo forbidden", false},
		{"store", "account key", false},
		{"starlark", "return 1", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var n yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte("value: !"+tc.name+" "+tc.args+"\n"), &n))
			err := WalkYAMLTags(&schema.AtmosConfiguration{}, &n, "scaffold.yaml", ScaffoldTagPolicy(nil))
			require.Equal(t, tc.allowed, err == nil, "%v", err)
			require.Equal(t, tc.allowed, fntag.Scope(tc.name, fntag.ScaffoldContext).NativeTag)
			if tc.name == "env" {
				require.Equal(t, "resolved", n.Content[0].Content[1].Value)
			}
		})
	}
}
