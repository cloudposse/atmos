package config

import (
	"testing"

	fntag "github.com/cloudposse/atmos/pkg/function/tag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	yaml "go.yaml.in/yaml/v3"
)

// Exercise the actual configuration dispatcher. A registration-only test would
// miss a supported catalog name whose consumer silently ignores it.
func TestConfigFunctionScopeMatchesDispatcher(t *testing.T) {
	t.Setenv("ATMOS_SCOPE_TEST", "resolved")
	cases := []struct {
		name, source, path string
		context            fntag.Context
		ok                 bool
	}{
		{"env", "value: !env ATMOS_SCOPE_TEST", "value", fntag.ConfigContext, true},
		{"store", "value: !store account key", "value", fntag.ConfigContext, false},
		{"version", "ai: {skills: {example: {ref: !version tool}}}", "ai.skills.example.ref", fntag.SkillRefContext, true},
		{"version", "value: !version tool", "value", fntag.ConfigContext, false},
		{"append", "value: !append [one, two]", "value", fntag.ConfigContext, true},
	}
	for _, tc := range cases {
		t.Run(tc.path+tc.name, func(t *testing.T) {
			var n yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte(tc.source), &n))
			v := viper.New()
			err := processNode(&n, v, "")
			require.Equal(t, tc.ok, err == nil, "%v", err)
			require.Equal(t, tc.ok, fntag.Scope(tc.name, tc.context).NativeTag)
			if tc.ok {
				require.NotNil(t, v.Get(tc.path))
			}
			if tc.name == "env" {
				require.Equal(t, "resolved", v.Get(tc.path))
			}
		})
	}
}
