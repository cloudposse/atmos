package stack

import (
	"os"
	"path/filepath"
	"testing"

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
