package starlark

import (
	"os"
	"path/filepath"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestDialectAllowsTopLevelControlFlow(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, source, want string }{
		{"for", "acc = []\nfor i in range(3):\n    acc.append(i)\noutput = acc", `[0,1,2]`},
		{"if", "acc = []\nif True:\n    acc.append(1)\nelse:\n    acc.append(2)\noutput = acc", `[1]`},
		{"while", "items = [1, 2, 3]\nwhile items:\n    items.pop()\noutput = items", `[]`},
		{"set", "output = len(set([1, 1, 2]))", `2`},
		{"recursion", "def fact(n):\n    return 1 if n <= 1 else n * fact(n - 1)\noutput = fact(5)", `120`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			result, err := runSource(t, tc.source)
			require.NoError(t, err)
			assert.JSONEq(t, tc.want, result.Value)
		})
	}
}

func TestDialectKeepsGlobalsSingleAssignmentWithHint(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, source string }{
		{"plain reassignment", "x = 1\nx = 2"},
		{"conditional output", "if True:\n    output = 1\nelse:\n    output = 2"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runSource(t, tc.source)
			require.ErrorIs(t, err, errUtils.ErrStarlark)
			assert.Contains(t, err.Error(), "cannot reassign global")
			hints := cockroach.GetAllHints(err)
			require.Len(t, hints, 1)
			assert.Contains(t, hints[0], "Starlark globals are assigned once")
			assert.Contains(t, hints[0], "`x = a if cond else b`")
			assert.Contains(t, hints[0], "`output = main()`")
		})
	}
}

func TestDialectAppliesToLoadedModules(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	files := map[string]string{
		"loop.star": "values = []\nfor i in range(2):\n    values.append(i)\n",
		"bad.star":  "y = 1\ny = 2\n",
	}
	for name, source := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(source), 0o600))
	}
	result, err := New().Execute(t.Context(), script.Spec{
		WorkingDirectory: dir,
		Source:           "load(\"loop.star\", \"values\")\noutput = values",
	})
	require.NoError(t, err)
	assert.JSONEq(t, `[0,1]`, result.Value)

	_, err = New().Execute(t.Context(), script.Spec{WorkingDirectory: dir, Source: "load(\"bad.star\", \"y\")"})
	require.ErrorIs(t, err, errUtils.ErrStarlark)
	hints := cockroach.GetAllHints(err)
	require.NotEmpty(t, hints)
	assert.Contains(t, hints[0], "assigned once")
}

func TestDryRunReportsSyntaxErrorsWithoutExecuting(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, source string
		wantErr      string
	}{
		{"valid", "for i in range(2):\n    exec.run([\"never\"])", ""},
		{"syntax error", "def broken(:\n    pass", "got ':'"},
		{"reassigned global", "x = 1\nx = 2", "cannot reassign global"},
		{"undefined name", "print(undefined_name)", "undefined"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runner := NewMockRunner(gomock.NewController(t))
			result, err := New(WithProcessRunner(runner)).Execute(t.Context(), script.Spec{
				DryRun: true, Name: "dry.star", Source: tc.source,
			})
			assert.Equal(t, script.Result{}, result)
			if tc.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrStarlark)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestDryRunDoesNotLoadModules(t *testing.T) {
	t.Parallel()
	read := func(string) ([]byte, error) {
		t.Error("dry run must not read modules")
		return nil, nil
	}
	_, err := New(WithReadFile(read)).Execute(t.Context(), script.Spec{DryRun: true, Source: "load(\"/lib/x.star\", \"y\")"})
	require.NoError(t, err)
}
