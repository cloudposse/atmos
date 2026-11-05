package localactions

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/script"
	"github.com/cloudposse/atmos/pkg/script/starlark"
)

type testCommand struct {
	Name     string
	Commands []testCommand
	Steps    []struct{ Type, Interpreter, Script string }
}

// Read the production steps so the tests cover the commands CI actually calls.
func commandSource(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("../../.atmos.d/test.yaml")
	require.NoError(t, err)
	var config struct{ Commands []testCommand }
	require.NoError(t, yaml.Unmarshal(data, &config))
	commands := config.Commands
	var selected testCommand
	for _, part := range []string{"test", "local-actions", name} {
		found := false
		for _, command := range commands {
			if command.Name == part {
				selected, commands, found = command, command.Commands, true
				break
			}
		}
		require.True(t, found, "missing command %s", part)
	}
	require.Len(t, selected.Steps, 1)
	require.Equal(t, "script", selected.Steps[0].Type)
	require.Equal(t, "starlark", selected.Steps[0].Interpreter)
	require.NotEmpty(t, selected.Steps[0].Script)
	return selected.Steps[0].Script
}

type recordingRunner struct {
	calls  [][]string
	failAt int
}

var _ process.Runner = (*recordingRunner)(nil)

func (r *recordingRunner) Run(_ context.Context, spec process.TaskSpec) process.Result { //nolint:gocritic // process.Runner requires a value parameter.
	r.calls = append(r.calls, append([]string{spec.Command}, spec.Args...))
	if len(r.calls) == r.failAt {
		return process.Result{ExitCode: 17}
	}
	return process.Result{}
}

func execute(t *testing.T, name string, files, directories []string, runner *recordingRunner) error {
	t.Helper()
	dir := t.TempDir()
	for _, path := range directories {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, path), 0o700))
	}
	for _, path := range files {
		path = filepath.Join(dir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, nil, 0o600))
	}
	_, err := starlark.New(starlark.WithProcessRunner(runner)).Execute(t.Context(), script.Spec{
		Source: commandSource(t, name), WorkingDirectory: dir,
	})
	return err
}

func TestDiscoveryAndExecution(t *testing.T) {
	for _, tc := range []struct {
		name        string
		files, dirs []string
		want        [][]string
	}{
		{
			name: "javascript",
			files: []string{
				".github/scripts/root.test.mjs", ".github/actions/action/deep/space name.test.cjs",
				".github/actions/action/dist/ignored.test.cjs", ".github/actions/action/node_modules/ignored.test.mjs",
				".github/actions/verify-sha-pinning/test.mjs", ".github/scripts/deep/deeper/nested.test.mjs",
			},
			dirs: []string{".github/scripts/directory.test.cjs"},
			want: [][]string{{
				"node", "--test", "--experimental-test-coverage",
				".github/actions/action/deep/space name.test.cjs", ".github/scripts/deep/deeper/nested.test.mjs",
				".github/scripts/root.test.mjs",
			}},
		},
		{
			name: "python",
			files: []string{
				".github/actions/a/test_one.py", ".github/actions/a/test_two.py",
				".github/actions/space name/test_three.py", ".github/actions/a/node_modules/test_ignored.py",
			},
			dirs: []string{".github/actions/empty/test_directory.py"},
			want: [][]string{
				{"python", "-m", "unittest", "discover", "-s", ".github/actions/a", "-p", "test_*.py", "-v"},
				{"python", "-m", "unittest", "discover", "-s", ".github/actions/space name", "-p", "test_*.py", "-v"},
			},
		},
		{
			name:  "packages",
			files: []string{".github/actions/space name/package.json", ".github/actions/space name/node_modules/package.json"},
			dirs:  []string{".github/actions/empty/package.json"},
			want: [][]string{
				{"npm", "ci", "--prefix", ".github/actions/space name"},
				{"npm", "test", "--prefix", ".github/actions/space name"},
				{"npm", "run", "typecheck", "--if-present", "--prefix", ".github/actions/space name"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &recordingRunner{}
			require.NoError(t, execute(t, tc.name, tc.files, tc.dirs, runner))
			require.Equal(t, tc.want, runner.calls)
			for failAt := 1; failAt <= len(tc.want); failAt++ {
				runner = &recordingRunner{failAt: failAt}
				err := execute(t, tc.name, tc.files, tc.dirs, runner)
				require.ErrorContains(t, err, "17")
				require.Equal(t, tc.want[:failAt], runner.calls, "must stop after a failed command")
			}
		})
	}
}

func TestEmptyDiscoveryFails(t *testing.T) {
	for _, name := range []string{"javascript", "python", "packages"} {
		t.Run(name, func(t *testing.T) {
			runner := &recordingRunner{}
			err := execute(t, name, []string{
				".github/actions/a/dist/ignored.test.cjs", ".github/actions/a/node_modules/ignored.test.mjs",
				".github/actions/a/test.mjs",
			}, nil, runner)
			require.ErrorContains(t, err, "test suites found")
			require.Empty(t, runner.calls)
		})
	}
}
