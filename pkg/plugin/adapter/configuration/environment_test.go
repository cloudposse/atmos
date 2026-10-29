package configuration

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestPreparationEnvironmentPrecedenceAndSanitization(t *testing.T) {
	t.Setenv("ATMOS_ENV_AMBIENT_PROBE", "ambient")
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "must-not-return")
	for _, test := range []struct {
		name      string
		sanitized []string
		overrides []string
		present   bool
		want      string
	}{
		{name: "ambient without identity", present: true, want: "ambient"},
		{name: "sanitized identity", sanitized: []string{"AWS_PROFILE=selected", "ATMOS_ENV_AMBIENT_PROBE=identity"}, want: "identity"},
		{name: "explicit empty environment", sanitized: []string{}},
		{name: "component overrides", sanitized: []string{"ATMOS_ENV_AMBIENT_PROBE=identity"}, overrides: []string{"ATMOS_ENV_AMBIENT_PROBE=component"}, want: "component"},
		{name: "explicit empty override", sanitized: []string{"ATMOS_ENV_AMBIENT_PROBE=identity"}, overrides: []string{"ATMOS_ENV_AMBIENT_PROBE="}},
		{name: "last path wins", sanitized: []string{"PATH=ambient"}, overrides: []string{"PATH=component", "PATH=toolchain"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			info := &schema.ConfigAndStacksInfo{SanitizedEnv: test.sanitized, ComponentEnvList: test.overrides, ComponentEnvSection: map[string]any{"AWS_WEB_IDENTITY_TOKEN_FILE": "stale-section"}}
			got := preparationEnvironment(info)
			require.Equal(t, test.want, got["ATMOS_ENV_AMBIENT_PROBE"])
			_, present := got["AWS_WEB_IDENTITY_TOKEN_FILE"]
			require.Equal(t, test.present, present)
			if test.name == "last path wins" {
				require.Equal(t, "toolchain", got["PATH"])
			}
			got["AWS_PROFILE"] = "mutated"
			require.NotEqual(t, "mutated", preparationEnvironment(info)["AWS_PROFILE"])
			if len(info.SanitizedEnv) > 0 {
				info.SanitizedEnv[0] = "SOURCE_CHANGED=yes"
				require.NotContains(t, got, "SOURCE_CHANGED")
			}
			require.Equal(t, "must-not-return", os.Getenv("AWS_WEB_IDENTITY_TOKEN_FILE"))
		})
	}
}
