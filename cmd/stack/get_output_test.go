package stack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStackGetJSON(t *testing.T) {
	resetEditFlags(t)
	chdirToValidAtmosProject(t)
	stdout := initStackConfigTestWriter(t)
	flagStack, flagComponent = "nonprod", "mycomponent"
	flagFile = filepath.Join(t.TempDir(), "stack.yaml")
	require.NoError(t, os.WriteFile(flagFile, []byte(`components:
  terraform:
    mycomponent:
      vars:
        text: "false"
        boolean: false
        items: [one, 2]
`), 0o600))
	for _, tc := range []struct{ path, expected string }{
		{"vars.text", `"false"`}, {"vars.boolean", `false`}, {"vars.items", `["one",2]`},
	} {
		stdout.Reset()
		require.NoError(t, runStackGetFormat([]string{tc.path}, "json"))
		assert.JSONEq(t, tc.expected, stdout.String())
	}
	stdout.Reset()
	flagFile = ""
	require.NoError(t, runStackGetFormat([]string{"vars.foo"}, "json"))
	assert.JSONEq(t, `"foo nonprod override"`, stdout.String())
}

// --format on both read commands follows the standard precedence: the flag, then the command's
// environment variable, then raw.
func TestStackGetFormatPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cmd      *cobra.Command
		envVar   string
		otherEnv string
	}{
		{"stack get", stackGetCmd, "ATMOS_STACK_GET_FORMAT", "ATMOS_STACK_CONFIG_GET_FORMAT"},
		{"stack config get", stackConfigGetCmd, "ATMOS_STACK_CONFIG_GET_FORMAT", "ATMOS_STACK_GET_FORMAT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetEditFlags(t)
			chdirToValidAtmosProject(t)
			stdout := initStackConfigTestWriter(t)
			flagStack, flagComponent = "nonprod", "mycomponent"
			flagFile = filepath.Join(t.TempDir(), "stack.yaml")
			require.NoError(t, os.WriteFile(flagFile, []byte("components:\n  terraform:\n    mycomponent:\n      vars:\n        text: \"false\"\n"), 0o600))
			reset := func() {
				viper.Reset()
				require.NoError(t, tc.cmd.Flags().Set("format", "raw"))
				tc.cmd.Flags().Lookup("format").Changed = false
			}
			t.Cleanup(reset)
			get := func() string {
				stdout.Reset()
				require.NoError(t, runStackGetCommand(tc.cmd, []string{"vars.text"}))
				return strings.TrimSpace(stdout.String())
			}

			reset()
			assert.Equal(t, "false", get(), "the default is raw")

			reset()
			t.Setenv(tc.otherEnv, "json")
			assert.Equal(t, "false", get(), "another command's environment variable does not change this command's output")

			reset()
			t.Setenv(tc.envVar, "json")
			assert.Equal(t, `"false"`, get(), "the environment variable selects the format")

			reset()
			t.Setenv(tc.envVar, "json")
			require.NoError(t, tc.cmd.Flags().Set("format", "raw"))
			assert.Equal(t, "false", get(), "the flag beats the environment variable")

			flag := tc.cmd.Flags().Lookup("format")
			require.NotNil(t, flag)
			assert.Equal(t, "f", flag.Shorthand)
		})
	}
}
