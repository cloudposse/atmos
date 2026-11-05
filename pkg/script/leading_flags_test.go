package script_test

import (
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/script"
)

// globalFlags is a stand-in for Atmos's global flags: --chdir/-C takes a value, --profile and
// --identity take only --flag=value, --no-color is boolean.
func globalFlags(name string, short bool) (takes, found bool) {
	if short {
		switch name {
		case "C":
			return true, true
		case "i":
			return false, true
		}
		return false, false
	}
	switch name {
	case "chdir", "logs-level":
		return true, true
	case "profile", "identity", "no-color":
		return false, true
	}
	return false, false
}

func optionalValue(name string, short bool) bool {
	return (!short && (name == "profile" || name == "identity")) || (short && name == "i")
}

func TestDiagnoseLeadingFlagsNamesTheMistake(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		args []string
		want string
		hint string
	}{
		{"profile with a space", []string{"--profile", "dev", "./tool.star", "api"}, `"--profile dev" before a script path is ambiguous`, "Write --profile=dev."},
		{"identity with a space", []string{"--no-color", "--identity", "admin", "tool.star"}, `"--identity admin" before a script path is ambiguous`, "Write --identity=admin."},
		{"short optional flag", []string{"-i", "admin", "./tool.star"}, `"-i admin" before a script path is ambiguous`, "Write -i=admin."},
		{"unknown flag before a script", []string{"--bogus", "./tool.star", "api"}, `unknown flag "--bogus" before a script path`, "belong to the script"},
		{"unknown flag after a known one", []string{"--chdir", "dir", "--bogus", "tool.star"}, `unknown flag "--bogus"`, "belong to the script"},
		{"unknown flag with a possible value", []string{"--bogus", "value", "./tool.star"}, `unknown flag "--bogus"`, "belong to the script"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := script.DiagnoseLeadingFlags(tc.args, globalFlags, optionalValue)
			require.ErrorIs(t, err, errUtils.ErrScriptUsage)
			assert.ErrorContains(t, err, tc.want)
			assert.Contains(t, cockroach.FlattenHints(err), tc.hint)
			assert.Equal(t, 2, errUtils.GetExitCode(err))
		})
	}
}

func TestDiagnoseLeadingFlagsLeavesOtherCommandLinesAlone(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{
		nil,
		{"./tool.star", "api"},
		{"--profile=dev", "./tool.star"},
		{"--profile", "./tool.star"},
		{"--no-color", "./tool.star"},
		{"--no-color", "describe", "./tool.star"},
		{"--chdir", "dir", "describe", "./tool.star"},
		{"--profile=dev", "dev", "./tool.star"},
		{"--profile", "dev", "describe", "stacks"},
		{"describe", "stacks", "--bogus", "./tool.star"},
		{"--bogus", "describe", "stacks"},
		{"--bogus"},
		{"--bogus", "terraform", "plan", "./component"},
		{"--"},
		{"--bogus", "--", "./tool.star"},
		{"--chdir"},
	} {
		assert.NoError(t, script.DiagnoseLeadingFlags(args, globalFlags, optionalValue), "%v", args)
	}
}

func TestSelectionEnv(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		profiles []string
		identity string
		want     map[string]string
	}{
		{"nothing selected", nil, "", map[string]string{}},
		{"one profile", []string{"dev"}, "", map[string]string{"ATMOS_PROFILE": "dev"}},
		{"several profiles keep their order", []string{"base", "dev"}, "", map[string]string{"ATMOS_PROFILE": "base,dev"}},
		{"blank and interactive profiles are dropped", []string{" ", "__SELECT__"}, "", map[string]string{}},
		{"identity", nil, "admin", map[string]string{"ATMOS_IDENTITY": "admin"}},
		{"interactive identity is not forwarded", nil, "__SELECT__", map[string]string{}},
		{"disabled identity", nil, "__DISABLED__", map[string]string{"ATMOS_IDENTITY": "false"}},
		{"both", []string{"dev"}, "admin", map[string]string{"ATMOS_PROFILE": "dev", "ATMOS_IDENTITY": "admin"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, script.SelectionEnv(tc.profiles, tc.identity))
		})
	}

	base := []string{"KEEP=1", "ATMOS_PROFILE=stale", "ATMOS_PROFILE=staler"}
	got := script.ProcessEnvironment(base, script.SelectionEnv([]string{"dev"}, ""))
	assert.Equal(t, []string{"KEEP=1", "ATMOS_PROFILE=dev"}, got, "the selection replaces every inherited value")
	assert.Equal(t, []string{"KEEP=1", "ATMOS_PROFILE=stale", "ATMOS_PROFILE=staler"}, base, "the inherited slice is not modified")
}
