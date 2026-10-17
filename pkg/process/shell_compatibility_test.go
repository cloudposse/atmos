package process

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	errUtils "github.com/cloudposse/atmos/errors"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// Exercise both the step runner and the installed middleware used by workflows, commands,
// Git hooks, and YAML !exec. Neither entry point may change the shell's executable lookup.
func compatibilityRunners() map[string]func(*ShellInterpreterSpec) error {
	return map[string]func(*ShellInterpreterSpec) error{
		"step": func(spec *ShellInterpreterSpec) error { return RunShellInterpreter(context.Background(), spec) },
		"shared": func(spec *ShellInterpreterSpec) error {
			return u.ShellRunnerWithWriters(&u.ShellRunnerSpec{
				Command: spec.Command, Dir: spec.Dir, Env: spec.Env, Stdout: spec.Stdout, Stderr: spec.Stderr,
			})
		},
	}
}

func TestShellCompatibilityUnexportedPATH(t *testing.T) {
	command, _, env := processHelperCommand(t, "argv-env")
	dir := t.TempDir()
	name := "atmos-shell-helper"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	data, err := os.ReadFile(command)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o755))
	env = append(env, "HELPER_DIR="+dir)
	for nameRunner, run := range compatibilityRunners() {
		t.Run(nameRunner, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run(&ShellInterpreterSpec{
				Command: `unset PATH; PATH="$HELPER_DIR"; ` + name + ` -test.run=TestHelperProcess -- argv-env`,
				Env:     env, Stdout: &stdout, Stderr: &stderr,
			})
			require.NoError(t, err, stderr.String())
			assert.Equal(t, "argv0="+name+" path-is-shell-local=false", stdout.String())
		})
	}
}

func TestShellCompatibilityNoShebang(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "helper"), []byte(`printf '%s|%s|%s\n' "$1" "$EXPORTED" "${LOCAL-unset}"; exit 17`), 0o755))
	for name, run := range compatibilityRunners() {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run(&ShellInterpreterSpec{
				Command: `LOCAL=private; export EXPORTED=public; ./helper 'two words' || printf 'recovered:%s\n' "$?"; printf 'following\n'`,
				Dir:     dir, Env: os.Environ(), Stdout: &stdout, Stderr: &stderr,
			})
			require.NoError(t, err, stderr.String())
			assert.Equal(t, "two words|public|unset\nrecovered:17\nfollowing\n", stdout.String())
		})
	}
}

func TestShellCompatibilityFallbackFailures(t *testing.T) {
	skipOnWindows(t)
	for _, test := range []struct {
		name   string
		body   string
		status string
	}{
		{name: "binary", body: "not\x00a script\n", status: "126"},
		{name: "syntax", body: "if then\n", status: "2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "helper"), []byte(test.body), 0o755))
			var stdout, stderr bytes.Buffer
			err := RunShellInterpreter(context.Background(), &ShellInterpreterSpec{
				Command: `./helper || printf 'status:%s\n' "$?"`,
				Dir:     dir, Env: os.Environ(), Stdout: &stdout, Stderr: &stderr,
			})
			require.NoError(t, err)
			assert.Equal(t, "status:"+test.status+"\n", stdout.String())
			assert.NotEmpty(t, stderr.String())
		})
	}
}

func TestShellCompatibilityFallbackNestedScriptAndStdin(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "outer"), []byte(`./inner "$@"`), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "inner"), []byte(`read -r line; printf '%s|%s\n' "$1" "$line"`), 0o755))
	var stdout bytes.Buffer
	err := RunShellInterpreter(context.Background(), &ShellInterpreterSpec{
		Command: `./outer -e`, Dir: dir, Env: os.Environ(), Stdin: strings.NewReader("payload\n"), Stdout: &stdout,
	})
	require.NoError(t, err)
	assert.Equal(t, "-e|payload\n", stdout.String())
}

func TestShellCompatibilityFallbackMatchesDefaultHandler(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "nested"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "nested", "helper"), []byte(`printf 'zero=%s|one=%s\n' "$0" "$1"; exit 17`), 0o755))
	for _, command := range []string{
		`./nested/helper 'two words'`,
		`exec ./nested/helper -x`,
		`PATH=nested; helper -e`,
		`cd nested; ./helper --`,
	} {
		t.Run(command, func(t *testing.T) {
			var baseline, stdout bytes.Buffer
			shell, err := interp.New(interp.Dir(dir), interp.Env(expand.ListEnviron(os.Environ()...)), interp.StdIO(nil, &baseline, nil))
			require.NoError(t, err)
			file, err := syntax.NewParser().Parse(strings.NewReader(command), "comparison")
			require.NoError(t, err)
			var baselineExit interp.ExitStatus
			require.ErrorAs(t, shell.Run(context.Background(), file), &baselineExit)
			var actualExit errUtils.ExitCodeError
			err = RunShellInterpreter(context.Background(), &ShellInterpreterSpec{Command: command, Dir: dir, Env: os.Environ(), Stdout: &stdout})
			require.ErrorAs(t, err, &actualExit)
			assert.Equal(t, int(baselineExit), actualExit.Code)
			assert.Equal(t, baseline.String(), stdout.String())
		})
	}
}

func TestShellCompatibilityMissingInterpreterDoesNotFallback(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "helper"), []byte("#!/atmos-nonexistent-interpreter\nprintf SHOULD_NOT_RUN\n"), 0o755))
	var stdout bytes.Buffer
	err := RunShellInterpreter(context.Background(), &ShellInterpreterSpec{Command: "./helper", Dir: dir, Env: os.Environ(), Stdout: &stdout})
	require.ErrorIs(t, err, syscall.ENOENT)
	assert.Empty(t, stdout.String())
}

func TestShellCompatibilityFallbackCancellationKillsDescendants(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	killAtCleanup(t, dir, "parent", "child")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "helper"), []byte(`"$BIN"`), 0o755))
	env := interpreterEnv(t, helperModeEnv+"="+modeParent, helperPIDDirEnv+"="+dir)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- RunShellInterpreter(ctx, &ShellInterpreterSpec{Command: "./helper", Dir: dir, Env: env})
	}()
	parent := waitForPID(t, dir, "parent")
	child := waitForPID(t, dir, "child")
	cancel()
	select {
	case err := <-result:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(goneWait + 5*time.Second):
		t.Fatal("shell fallback did not return after cancellation")
	}
	requireGone(t, parent, "fallback child")
	requireGone(t, child, "fallback grandchild")
}
