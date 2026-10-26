package atmos

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestModuleStdin(t *testing.T) {
	t.Parallel()
	for _, call := range []string{
		`atmos.run(["version"]`, `atmos.vendor("pull"`, `atmos.toolchain("list"`,
		`atmos.terraform("plan","api","dev"`, `atmos.tf("plan","api","dev"`,
		`atmos.helm("template","api","dev"`,
	} {
		for _, tc := range []struct {
			name, arg, want string
			absent, invalid bool
		}{
			{name: "omitted", absent: true},
			{name: "none", arg: ", stdin=None", absent: true},
			{name: "empty", arg: `, stdin=""`},
			{name: "bytes", arg: `, stdin="secret\n\x00café"`, want: "secret\n\x00café"},
			{name: "integer", arg: ", stdin=42", invalid: true},
			{name: "boolean", arg: ", stdin=False", invalid: true},
			{name: "list", arg: `, stdin=["private-payload"]`, invalid: true},
		} {
			t.Run(call+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				calls := 0
				module := New("project", testCatalog(), func(_ *starlark.Thread, argv []string, opts Options, _ bool) (starlark.Value, error) {
					calls++
					assert.NotContains(t, argv, "--stdin", "input must not implicitly add a CLI flag")
					if tc.absent {
						assert.Nil(t, opts.Stdin)
					} else {
						require.NotNil(t, opts.Stdin)
						assert.Equal(t, tc.want, *opts.Stdin)
					}
					return starlark.None, nil
				})
				_, err := starlark.EvalOptions(&syntax.FileOptions{}, &starlark.Thread{}, "stdin.star", call+tc.arg+")", starlark.StringDict{"atmos": module})
				if tc.invalid {
					require.ErrorIs(t, err, errUtils.ErrStarlarkInvalidArgument)
					assert.Contains(t, err.Error(), "stdin must be a string or None")
					assert.NotContains(t, err.Error(), "private-payload")
					assert.Zero(t, calls)
				} else {
					require.NoError(t, err)
					assert.Equal(t, 1, calls)
				}
			})
		}
	}
}
