package script_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/script"
	starlarkengine "github.com/cloudposse/atmos/pkg/script/starlark"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetectFilePreservesNormalCLI(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{nil, {"terraform", "plan"}, {"--help"}, {"deploy"}, {"--chdir=repo", "deploy.star"}} {
		file, err := script.DetectFile(args)
		require.NoError(t, err)
		assert.Nil(t, file)
	}
}

func TestDetectStandaloneFile(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, source string
		detected     bool
	}{
		{"deploy.star", `print("hello")`, true},
		{"deploy", "#!/usr/bin/env atmos\nprint('hello')", true},
		{"absolute", "#!/usr/bin/atmos\nprint('hello')", true},
		{"env-s", "#!/usr/bin/env -S atmos\nprint('hello')", true},
		{"bash", "#!/bin/sh\necho hello", false},
		{"not-atmos", "#!/usr/bin/env atmos-other\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), tc.name)
			require.NoError(t, os.WriteFile(path, []byte(tc.source), 0o600))
			file, err := script.DetectFile([]string{path, "--help", "--chdir=/not-atmos", "two words"})
			require.NoError(t, err)
			if !tc.detected {
				assert.Nil(t, file)
				return
			}
			assert.Equal(t, []string{"--help", "--chdir=/not-atmos", "two words"}, file.Args)
			physical, err := filepath.EvalSymlinks(path)
			require.NoError(t, err)
			assert.Equal(t, physical, file.Path)
			assert.Equal(t, "starlark", file.Interpreter)
		})
	}
	_, err := script.DetectFile([]string{filepath.Join(t.TempDir(), "missing.star")})
	require.Error(t, err)
	file, err := script.DetectFile([]string{t.TempDir()})
	require.NoError(t, err)
	require.Nil(t, file)
}

func TestRegisteredExtensionDispatch(t *testing.T) {
	t.Parallel()
	// This uses the real engine under another name, without introducing a second language.
	script.Register("file-test-engine", starlarkengine.New(), script.WithExtensions(".script-test"))
	script.Register("file-test-engine", starlarkengine.New())
	path := filepath.Join(t.TempDir(), "release.script-test")
	require.NoError(t, os.WriteFile(path, []byte("#!/usr/bin/env atmos\noutput = 42"), 0o600))
	args := []string{path, "two words"}
	file, err := script.DetectFile(args)
	require.NoError(t, err)
	require.NotNil(t, file)
	assert.Equal(t, "file-test-engine", file.Interpreter, "the extension takes precedence over the shebang")
	args[1] = "changed"
	assert.Equal(t, []string{"two words"}, file.Args)
	file.Args[0] = "also changed"
	assert.Equal(t, "changed", args[1])
	engine, ok := script.Get(file.Interpreter)
	require.True(t, ok)
	source, err := os.ReadFile(file.Path)
	require.NoError(t, err)
	result, err := engine.Execute(t.Context(), script.Spec{Source: string(source)})
	require.NoError(t, err)
	assert.Equal(t, script.Result{Value: "42", HasOutput: true}, result)
	_, err = script.DetectFile([]string{filepath.Join(t.TempDir(), "missing.script-test")})
	require.ErrorIs(t, err, os.ErrNotExist)
	// Merely adding the registry must not claim TypeScript or unrelated files.
	unregistered := filepath.Join(t.TempDir(), "release.ts")
	require.NoError(t, os.WriteFile(unregistered, []byte("export const value = 42"), 0o600))
	file, err = script.DetectFile([]string{unregistered})
	require.NoError(t, err)
	assert.Nil(t, file)
	_, ok = script.InterpreterForFile("release.SCRIPT-TEST")
	assert.False(t, ok)
}

func TestDetectStdin(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		argv, want []string
	}{
		{"default", []string{"-"}, []string{}},
		{"arguments", []string{"-", "foo", "bar"}, []string{"foo", "bar"}},
		{"separator", []string{"-", "--", "foo", "bar"}, []string{"foo", "bar"}},
		{"equals interpreter", []string{"--interpreter=starlark", "-", "--", "foo"}, []string{"foo"}},
		{"separate interpreter", []string{"--interpreter", "starlark", "-", "foo"}, []string{"foo"}},
		{"script flags", []string{"-", "--help", "--interpreter=script-owned", "--chdir=script-owned"}, []string{"--help", "--interpreter=script-owned", "--chdir=script-owned"}},
		{"literal separator", []string{"-", "--", "--", "--literal"}, []string{"--", "--literal"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			original := append([]string{}, tc.argv...)
			file, err := script.DetectFile(tc.argv)
			require.NoError(t, err)
			require.NotNil(t, file)
			assert.True(t, file.Stdin)
			assert.Equal(t, "-", file.Path)
			assert.Equal(t, "starlark", file.Interpreter)
			assert.Equal(t, tc.want, file.Args)
			if len(file.Args) > 0 {
				file.Args[0] = "mutated"
				assert.Equal(t, original, tc.argv)
				file.Args[0] = tc.want[0]
				tc.argv[len(tc.argv)-1] = "changed"
				assert.Equal(t, tc.want, file.Args)
			}
		})
	}
}

func TestDetectStdinRejectsInvalidSelection(t *testing.T) {
	t.Parallel()
	for _, argv := range [][]string{
		{"--interpreter"},
		{"--interpreter=starlark"},
		{"--interpreter=starlark", "script.star"},
		{"--interpreter=missing", "-"},
		{"--interpreter=", "-"},
		{"--interpreter=starlark", "--unknown", "-"},
	} {
		t.Run(strings.Join(argv, " "), func(t *testing.T) {
			t.Parallel()
			file, err := script.DetectFile(argv)
			require.Error(t, err)
			assert.Nil(t, file)
		})
	}
	// A separator alone is not an instruction to execute stdin.
	file, err := script.DetectFile([]string{"--", "foo", "bar"})
	require.NoError(t, err)
	assert.Nil(t, file)
}

func TestDetectFileOnlyReportsErrorsForStarNames(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	stacks := filepath.Join(root, "stacks")
	require.NoError(t, os.Mkdir(stacks, 0o700))
	starDir := filepath.Join(root, "tools.star")
	require.NoError(t, os.Mkdir(starDir, 0o700))
	plain := filepath.Join(root, "notes.txt")
	require.NoError(t, os.WriteFile(plain, []byte("hello"), 0o600))

	for _, tc := range []struct {
		name string
		arg  string
	}{
		{"existing directory with a slash", stacks},
		{"directory with a trailing slash", stacks + string(filepath.Separator)},
		{"missing path with a slash", filepath.Join(root, "missing", "tool")},
		{"regular file without the shebang", plain},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			file, err := script.DetectFile([]string{tc.arg, "extra"})
			require.NoError(t, err, "non-.star paths fall through to normal command handling")
			assert.Nil(t, file)
		})
	}

	t.Run("star directory keeps a clear error", func(t *testing.T) {
		t.Parallel()
		_, err := script.DetectFile([]string{starDir})
		require.ErrorIs(t, err, errUtils.ErrScript)
		require.ErrorContains(t, err, "regular file")
	})
	t.Run("missing star file keeps a clear error", func(t *testing.T) {
		t.Parallel()
		_, err := script.DetectFile([]string{filepath.Join(root, "missing.star")})
		require.ErrorIs(t, err, errUtils.ErrScript)
	})
}

func TestDetectFileRecordsHowTheScriptWasInvoked(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "tool.star")
	require.NoError(t, os.WriteFile(path, []byte("print(1)"), 0o600))
	file, err := script.DetectFile([]string{path})
	require.NoError(t, err)
	require.NotNil(t, file)
	assert.Equal(t, path, file.Invoked)
}

func rootFlags(name string, short bool) (takes, found bool) {
	switch {
	case short && name == "C":
		return true, true
	case short && name == "v":
		return false, true
	case !short && (name == "chdir" || name == "logs-level"):
		return true, true
	case !short && name == "no-color":
		return false, true
	case !short && name == "identity":
		return false, true // Optional value: only --identity=name.
	}
	return false, false
}

func TestSplitGlobalFlags(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		args    []string
		globals []string
		rest    []string
		ok      bool
	}{
		{"no flags", []string{"./x.star", "a"}, []string{}, []string{"./x.star", "a"}, true},
		{"equals form", []string{"--chdir=dir", "x.star", "a"}, []string{"--chdir=dir"}, []string{"x.star", "a"}, true},
		{"separate value", []string{"--logs-level", "Debug", "./x.star"}, []string{"--logs-level", "Debug"}, []string{"./x.star"}, true},
		{"shorthand value", []string{"-C", "dir", "x.star"}, []string{"-C", "dir"}, []string{"x.star"}, true},
		{"shorthand attached", []string{"-Cdir", "x.star"}, []string{"-Cdir"}, []string{"x.star"}, true},
		{"bool flag takes no value", []string{"--no-color", "x.star"}, []string{"--no-color"}, []string{"x.star"}, true},
		{"optional-value flag takes no next word", []string{"--identity", "x.star"}, []string{"--identity"}, []string{"x.star"}, true},
		{"several", []string{"--chdir=a", "--no-color", "-v", "x.star", "--chdir=b"}, []string{"--chdir=a", "--no-color", "-v"}, []string{"x.star", "--chdir=b"}, true},
		{"double dash ends globals", []string{"--chdir=a", "--", "x.star", "--"}, []string{"--chdir=a"}, []string{"x.star", "--"}, true},
		{"stdin interpreter after globals", []string{"--no-color", "--interpreter=starlark", "-", "--help"}, []string{"--no-color"}, []string{"--interpreter=starlark", "-", "--help"}, true},
		{"unknown flag", []string{"--bogus", "x.star"}, nil, []string{"--bogus", "x.star"}, false},
		{"missing value", []string{"--chdir"}, nil, []string{"--chdir"}, false},
		{"only flags", []string{"--no-color"}, []string{"--no-color"}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			globals, rest, ok := script.SplitGlobalFlags(tc.args, rootFlags)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.rest, rest)
			assert.Equal(t, tc.globals, globals)
		})
	}
}

func TestMayBeFile(t *testing.T) {
	t.Parallel()
	for arg, want := range map[string]bool{"x.star": true, "./tool": true, `dir\tool`: true, "terraform": false, "--x.star": false, "": false} {
		assert.Equal(t, want, script.MayBeFile([]string{arg}), arg)
	}
	assert.False(t, script.MayBeFile(nil))
}
