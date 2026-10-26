package starlark

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.starlark.net/starlark"
	"go.uber.org/mock/gomock"

	atmosgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/git/sandbox"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/script"
	"github.com/cloudposse/atmos/pkg/script/starlark/internal/threadcontext"
)

type sandboxGitRunner func(context.Context, string, []string, atmosgit.RunOptions) (atmosgit.RunResult, error)

//nolint:gocritic // Match the Runner interface's value parameter.
func (f sandboxGitRunner) Run(ctx context.Context, command string, args []string, opts atmosgit.RunOptions) (atmosgit.RunResult, error) {
	return f(ctx, command, args, opts)
}

func TestGitSandboxScopesAtmosEnvironmentAndSupportFiles(t *testing.T) {
	root := sandboxRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "support"), []byte("tool"), 0o644))
	runner := NewMockRunner(gomock.NewController(t))
	var checkout string
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, task process.TaskSpec) process.Result {
		checkout = task.Dir
		require.NotEqual(t, root, checkout)
		values := map[string]string{}
		for _, entry := range task.Env {
			key, value, _ := strings.Cut(entry, "=")
			values[key] = value
		}
		require.Equal(t, "child", values["SANDBOX_TEST"])
		require.Equal(t, checkout, values["ATMOS_BASE_PATH"])
		require.Equal(t, checkout, values["ATMOS_CLI_CONFIG_PATH"])
		require.Equal(t, "false", values["ATMOS_VERSION_CHECK_ENABLED"])
		require.NotContains(t, values, "GIT_AUTHOR_NAME")
		require.True(t, strings.HasPrefix(values["PATH"], filepath.Join(root, "tools")+string(os.PathListSeparator)))
		data, err := os.ReadFile(filepath.Join(checkout, "support"))
		require.NoError(t, err)
		require.Equal(t, "tool", string(data))
		return process.Result{}
	})
	_, err := New(WithProcessRunner(runner), WithGitRunner(atmosgit.NewExecRunner())).Execute(t.Context(), script.Spec{
		WorkingDirectory: root, AtmosWorkingDirectory: t.TempDir(),
		ProcessEnv: append(os.Environ(), "GIT_AUTHOR_NAME=source", "SANDBOX_TEST=parent"),
		Source: `
def check(box):
    box.copy(["support"])
    box.track(["generated"])
    fs.write_file("generated", "result\n")
    atmos.run(["version"], output="capture")
git.sandbox(run=check, env={"SANDBOX_TEST": "child"}, path_prepend=["tools"], fixes="unstaged", fail_on_changes=False)
`,
	})
	require.NoError(t, err)
	require.NoDirExists(t, checkout)
	data, err := os.ReadFile(filepath.Join(root, "generated"))
	require.NoError(t, err)
	require.Equal(t, "result\n", string(data))
	require.Equal(t, "file.txt\n", sandboxGit(t, root, "ls-files"))
}

func TestGitSandboxInvalidEnvironmentCleansCheckout(t *testing.T) {
	root := sandboxRepo(t)
	var checkout string
	runner := sandboxGitRunner(func(ctx context.Context, command string, args []string, opts atmosgit.RunOptions) (atmosgit.RunResult, error) {
		if args[0] == "init" {
			checkout = opts.Dir
		}
		return atmosgit.NewExecRunner().Run(ctx, command, args, opts)
	})
	_, err := New(WithGitRunner(runner)).Execute(t.Context(), script.Spec{WorkingDirectory: root, Source: `git.sandbox(run=lambda s: fail("callback must not run"), env={"KEY": 1})`})
	require.ErrorContains(t, err, "process env must contain strings")
	require.NotEmpty(t, checkout)
	require.NoDirExists(t, checkout)
}

func TestGitSandboxValidatesPathSequences(t *testing.T) {
	for _, source := range []string{
		`git.sandbox(run=lambda s: None, path_prepend="tools")`,
		`git.sandbox(run=lambda s: None, path_prepend=1)`,
		`git.sandbox(run=lambda s: None, path_prepend=[1])`,
	} {
		_, err := New().Execute(t.Context(), script.Spec{Source: source})
		require.Error(t, err, source)
	}
	for _, name := range []string{"copy", "track"} {
		for _, args := range []starlark.Tuple{nil, {starlark.String("file")}, {starlark.MakeInt(1)}, {starlark.NewList([]starlark.Value{starlark.MakeInt(1)})}} {
			value := sandboxValue(&sandbox.Sandbox{})
			method, err := value.(starlark.HasAttrs).Attr(name)
			require.NoError(t, err)
			thread := &starlark.Thread{Name: "invalid sandbox arguments"}
			threadcontext.Set(t.Context(), thread)
			_, err = starlark.Call(thread, method, args, nil)
			require.Error(t, err, name)
		}
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		thread := &starlark.Thread{Name: "cancelled sandbox"}
		threadcontext.Set(ctx, thread)
		method, err := sandboxValue(&sandbox.Sandbox{}).(starlark.HasAttrs).Attr(name)
		require.NoError(t, err)
		_, err = starlark.Call(thread, method, starlark.Tuple{starlark.NewList(nil)}, nil)
		require.ErrorIs(t, err, context.Canceled)
	}
}

func TestSandboxEnvironmentWithoutInheritedPath(t *testing.T) {
	env, err := sandboxEnvironment([]string{"BASE=kept"}, "checkout", &sandboxArguments{paths: []string{"tools"}})
	require.NoError(t, err)
	require.Contains(t, env, "PATH=tools")
	require.Contains(t, env, "BASE=kept")
}

func TestGitSandboxScopesOSChanges(t *testing.T) {
	root := sandboxRepo(t)
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(func(_ context.Context, task process.TaskSpec) process.Result {
		require.NotEqual(t, root, task.Dir)
		require.Equal(t, ".git", filepath.Base(task.Dir))
		require.Contains(t, task.Env, "SANDBOX_OS_TEST=inner")
		return process.Result{Started: true}
	})
	result, err := New(WithProcessRunner(runner)).Execute(t.Context(), script.Spec{
		WorkingDirectory: root,
		Source: `
os.setenv("SANDBOX_OS_TEST", "outer")
before = os.getcwd()
def check(box):
    if os.getenv("SANDBOX_OS_TEST") != "outer" or os.getcwd() != box.path:
        fail("sandbox must inherit the effective environment and directory")
    os.chdir(".git")
    os.setenv("SANDBOX_OS_TEST", "inner")
    fs.write_file("scoped.txt", "inside")
    exec.run(["tool"], output="capture")
    atmos.run(["version"], output="capture")
    return [fs.read_file("scoped.txt"), os.environ()["SANDBOX_OS_TEST"], os.getcwd() == fs.resolve(".")]
inside = git.sandbox(run=check)
output = [inside, os.getenv("SANDBOX_OS_TEST"), os.getcwd() == before]
`,
	})
	require.NoError(t, err)
	require.JSONEq(t, `[["inside","inner",true],"outer",true]`, result.Value)
	require.NoFileExists(t, filepath.Join(root, ".git", "scoped.txt"))
}
