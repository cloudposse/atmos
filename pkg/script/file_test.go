package script_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	_, err = script.DetectFile([]string{t.TempDir()})
	require.ErrorContains(t, err, "regular file")
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
