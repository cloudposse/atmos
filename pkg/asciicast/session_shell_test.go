package asciicast

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

// sessionHelperShell returns the test binary as a deterministic cross-platform
// "shell" (see runAsciicastSessionHelper).
func sessionHelperShell(t *testing.T) string {
	t.Helper()
	shell, err := os.Executable()
	require.NoError(t, err)
	t.Setenv(asciicastSessionHelperEnv, "1")
	t.Setenv(asciicastSessionBannerEnv, "1")
	return shell
}

func writeRCFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "demo.rc")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func TestResolveSessionShellInitClean(t *testing.T) {
	tests := []struct {
		shell    string
		wantArgs []string
		wantEnv  map[string]string
	}{
		{shell: "/bin/bash", wantArgs: []string{"--noprofile", "--norc"}},
		{shell: "/opt/homebrew/bin/zsh", wantArgs: []string{"-f", "-o", "NO_PROMPT_SP"}},
		{shell: "/usr/bin/fish", wantArgs: []string{"--no-config"}},
		{shell: "bash.exe", wantArgs: []string{"--noprofile", "--norc"}},
		{shell: "/bin/sh", wantEnv: map[string]string{"ENV": ""}},
		{shell: "/usr/bin/dash", wantEnv: map[string]string{"ENV": ""}},
	}
	for _, tt := range tests {
		for _, mode := range []string{"", "clean", " CLEAN "} {
			t.Run(tt.shell+"/"+mode, func(t *testing.T) {
				launch, err := resolveSessionShellInit(tt.shell, mode)
				require.NoError(t, err)
				assert.Equal(t, tt.wantArgs, launch.Args)
				assert.Equal(t, tt.wantEnv, launch.Env)
			})
		}
	}
}

func TestResolveSessionShellInitUserKeepsNormalStartup(t *testing.T) {
	for _, shell := range []string{"/bin/bash", "/bin/zsh", "/bin/sh"} {
		launch, err := resolveSessionShellInit(shell, "user")
		require.NoError(t, err)
		assert.Empty(t, launch.Args, shell)
		assert.Empty(t, launch.Env, shell)
	}
}

func TestSessionShellLaunchEnvironmentSilencesBashBannerWithoutMutatingInput(t *testing.T) {
	launch, err := resolveSessionShellInit("/bin/sh", "user")
	require.NoError(t, err)

	base := map[string]string{"A": "1"}
	merged := launch.environment(base)
	assert.Equal(t, "1", merged["A"])
	assert.Equal(t, "1", merged["BASH_SILENCE_DEPRECATION_WARNING"])
	assert.NotContains(t, base, "BASH_SILENCE_DEPRECATION_WARNING", "the caller's map must not change")

	// An explicit caller value wins for the banner switch.
	merged = launch.environment(map[string]string{"BASH_SILENCE_DEPRECATION_WARNING": "0"})
	assert.Equal(t, "0", merged["BASH_SILENCE_DEPRECATION_WARNING"])

	// Functional overrides always win over the caller's environment.
	clean, err := resolveSessionShellInit("/bin/sh", "clean")
	require.NoError(t, err)
	assert.Equal(t, "", clean.environment(map[string]string{"ENV": "/home/me/.shrc"})["ENV"])
}

func TestResolveSessionShellInitRCFile(t *testing.T) {
	rc := writeRCFile(t, "export DEMO=1\n")

	t.Run("bash", func(t *testing.T) {
		launch, err := resolveSessionShellInit("/bin/bash", rc)
		require.NoError(t, err)
		assert.Equal(t, []string{"--noprofile", "--rcfile", rc}, launch.Args)
		assert.Nil(t, launch.cleanup)
	})

	t.Run("fish", func(t *testing.T) {
		launch, err := resolveSessionShellInit("/usr/bin/fish", rc)
		require.NoError(t, err)
		assert.Equal(t, []string{"--no-config", "-C", "source " + fishQuote(rc)}, launch.Args)
	})

	t.Run("zsh uses a temporary ZDOTDIR that sources the file and is cleaned up", func(t *testing.T) {
		launch, err := resolveSessionShellInit("/bin/zsh", rc)
		require.NoError(t, err)
		dir := launch.Env["ZDOTDIR"]
		require.NotEmpty(t, dir)
		content, err := os.ReadFile(filepath.Join(dir, ".zshrc"))
		require.NoError(t, err)
		assert.Equal(t, "source '"+rc+"'\n", string(content))

		launch.release()
		_, statErr := os.Stat(dir)
		assert.ErrorIs(t, statErr, os.ErrNotExist)
		assert.NotPanics(t, launch.release, "release must be idempotent")
	})
}

func TestResolveSessionShellInitRCFileErrors(t *testing.T) {
	rc := writeRCFile(t, "")

	t.Run("missing file names the path", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "nope.rc")
		_, err := resolveSessionShellInit("/bin/bash", missing)
		require.ErrorIs(t, err, errUtils.ErrInvalidCastShellInit)
		assert.Contains(t, err.Error(), missing)
	})

	t.Run("directory", func(t *testing.T) {
		_, err := resolveSessionShellInit("/bin/zsh", t.TempDir())
		require.ErrorIs(t, err, errUtils.ErrInvalidCastShellInit)
	})

	t.Run("unsupported shell", func(t *testing.T) {
		_, err := resolveSessionShellInit("/bin/sh", rc)
		require.ErrorIs(t, err, errUtils.ErrInvalidCastShellInit)
		assert.Contains(t, err.Error(), "bash, zsh, or fish")
	})
}

func TestQuotingHelpers(t *testing.T) {
	assert.Equal(t, `'/tmp/it'\''s here'`, posixQuote("/tmp/it's here"))
	assert.Equal(t, `'/tmp/it\'s\\here'`, fishQuote(`/tmp/it's\here`))
	assert.Equal(t, `'C:\\Users\\demo\\init.fish'`, fishQuote(`C:\Users\demo\init.fish`))
}

func TestStartSessionShellRejectsInvalidShellInitBeforeStarting(t *testing.T) {
	_, err := startSessionShell(context.Background(), &SessionOptions{
		Shell:     sessionHelperShell(t),
		ShellInit: filepath.Join(t.TempDir(), "missing.rc"),
	})
	require.ErrorIs(t, err, errUtils.ErrInvalidCastShellInit)
}

// TestRunSessionShellInitReachesTheShellProcess drives the real session path
// with the test binary as the shell, so the environment the launch resolver
// computed is observable from inside the process.
func TestRunSessionShellInitReachesTheShellProcess(t *testing.T) {
	require.NoError(t, iolib.Initialize())
	shell := sessionHelperShell(t)

	tests := []struct {
		name      string
		shellInit string
		want      string
	}{
		{name: "default is clean", shellInit: "", want: `shellinit env_set=true env="" silence="1" args=[]`},
		{name: "clean", shellInit: "clean", want: `shellinit env_set=true env="" silence="1" args=[]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
			defer cancel()
			require.NoError(t, RunSession(ctx, &SessionOptions{
				Shell:     shell,
				ShellInit: tt.shellInit,
				Env:       map[string]string{"ENV": "/should/be/overridden"},
				Actions: []SessionAction{
					{Type: "wait", Text: sessionHelperBanner, Timeout: "10s"},
					{Type: "write", Text: "print shellinit", Rate: "0"},
					{Type: "key", Key: "enter"},
					{Type: "wait", Text: tt.want, Timeout: "10s"},
				},
			}))
		})
	}

	t.Run("user keeps the caller environment", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		require.NoError(t, RunSession(ctx, &SessionOptions{
			Shell:     shell,
			ShellInit: "user",
			Env:       map[string]string{"ENV": "/kept"},
			Actions: []SessionAction{
				{Type: "wait", Text: sessionHelperBanner, Timeout: "10s"},
				{Type: "write", Text: "print shellinit", Rate: "0"},
				{Type: "key", Key: "enter"},
				{Type: "wait", Text: `env="/kept" silence="1"`, Timeout: "10s"},
			},
		}))
	})
}

func TestSplitSessionShell(t *testing.T) {
	spaced := filepath.Join(t.TempDir(), "dir with space", "my shell")
	require.NoError(t, os.MkdirAll(filepath.Dir(spaced), 0o755))
	require.NoError(t, os.WriteFile(spaced, []byte("x"), 0o600))

	tests := []struct {
		name     string
		in       string
		wantExe  string
		wantArgs []string
	}{
		{name: "bare name", in: "bash", wantExe: "bash"},
		{name: "name with arguments", in: "bash -o vi", wantExe: "bash", wantArgs: []string{"-o", "vi"}},
		{name: "quoted argument", in: `zsh -c 'echo a b'`, wantExe: "zsh", wantArgs: []string{"-c", "echo a b"}},
		{name: "existing path with spaces is used as-is", in: spaced, wantExe: spaced},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exe, args, err := splitSessionShell(tt.in)
			require.NoError(t, err)
			assert.Equal(t, tt.wantExe, exe)
			assert.Equal(t, tt.wantArgs, args)
		})
	}

	t.Run("unterminated quote", func(t *testing.T) {
		_, _, err := splitSessionShell(`bash "-o vi`)
		require.ErrorIs(t, err, errUtils.ErrInvalidCastShell)
	})
	t.Run("whitespace only", func(t *testing.T) {
		_, _, err := splitSessionShell("   ")
		require.ErrorIs(t, err, errUtils.ErrInvalidCastShell)
	})
}

// TestSessionShellCommandAppendsAuthorArgsAfterShellInit verifies the author's
// own shell arguments follow the shell_init arguments, so `shell: bash -o vi`
// keeps the clean startup, and shell_init "user" runs the command line verbatim.
func TestSessionShellCommandAppendsAuthorArgsAfterShellInit(t *testing.T) {
	launch, err := sessionShellCommand("bash -o vi", ShellInitClean)
	require.NoError(t, err)
	defer launch.release()
	assert.Equal(t, "bash", launch.Executable)
	assert.Equal(t, []string{"--noprofile", "--norc", "-o", "vi"}, launch.Args)

	verbatim, err := sessionShellCommand("bash --rcfile demo.rc", ShellInitUser)
	require.NoError(t, err)
	defer verbatim.release()
	assert.Equal(t, "bash", verbatim.Executable)
	assert.Equal(t, []string{"--rcfile", "demo.rc"}, verbatim.Args)
}

// TestRunSessionPassesShellCommandLineArguments runs a real session whose shell
// setting carries an argument and confirms the shell process receives it.
func TestRunSessionPassesShellCommandLineArguments(t *testing.T) {
	shell := sessionHelperShell(t)
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	require.NoError(t, RunSession(ctx, &SessionOptions{
		Shell: posixQuote(shell) + " --demo-arg",
		Actions: []SessionAction{
			{Type: "wait", Text: sessionHelperBanner, Timeout: "10s"},
			{Type: "write", Text: "print shellinit", Rate: "0"},
			{Type: "key", Key: "enter"},
			{Type: "wait", Text: `args=["--demo-arg"]`, Timeout: "10s"},
		},
	}))
}

func TestMarkPromptEscapes(t *testing.T) {
	colored := "\x1b[1;38;2;0;163;224m> \x1b[0m"
	tests := []struct {
		name   string
		shell  string
		prompt string
		want   string
	}{
		{name: "bash wraps escapes", shell: "bash", prompt: colored, want: "\\[\x1b[1;38;2;0;163;224m\\]> \\[\x1b[0m\\]"},
		{name: "zsh wraps escapes", shell: "zsh", prompt: colored, want: "%{\x1b[1;38;2;0;163;224m%}> %{\x1b[0m%}"},
		{name: "already marked bash prompt is kept", shell: "bash", prompt: "\\[\x1b[0m\\]$ ", want: "\\[\x1b[0m\\]$ "},
		{name: "unknown shell keeps an author prompt", shell: "sh", prompt: "\x1b[31m$ \x1b[0m", want: "\x1b[31m$ \x1b[0m"},
		{name: "plain prompt is unchanged", shell: "bash", prompt: "$ ", want: "$ "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, markPromptEscapes(tt.shell, tt.prompt))
		})
	}

	t.Run("unknown shell drops the default prompt's colors", func(t *testing.T) {
		assert.Equal(t, ansi.Strip(defaultSessionPrompt()), markPromptEscapes("sh", defaultSessionPrompt()))
	})
}

func TestFirstOutputLines(t *testing.T) {
	assert.Equal(t, "no output", firstOutputLines("\r\n  \n"))
	assert.Equal(t, "sh: --x: invalid option; Usage: sh", firstOutputLines("\r\nsh: --x: invalid option\r\nUsage: sh\r\nmore\r\n"))
}

// TestRunSessionReportsShellThatExitsAtStartup verifies a shell that exits
// before any action (for example on an unknown option) fails with
// ErrCastSessionShellExited naming the exit status and the shell's own output,
// instead of a PTY write error.
func TestRunSessionReportsShellThatExitsAtStartup(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	t.Setenv(asciicastExecHelperEnv, "fail")

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	err = RunSession(ctx, &SessionOptions{
		Shell:   exe,
		Actions: []SessionAction{{Type: "write", Text: "echo hi", Rate: "0"}},
	})
	require.ErrorIs(t, err, errUtils.ErrCastSessionShellExited)
	assert.Contains(t, err.Error(), "exit status 3")
	assert.Contains(t, err.Error(), "about to fail")
}

func TestFinishSessionWithDiagnosticsPreservesCauseAndOutput(t *testing.T) {
	waitErr := errors.New("exit status 3")
	proc := newFakeSessionProcess(t, waitErr)
	t.Cleanup(func() { _ = proc.input.Close() })
	state := newWaitTestState()
	appendSessionOutput(state, "\x1b[31mbad shell option\x1b[0m\r\n")
	state.done <- nil

	err := finishSessionWithDiagnostics(context.Background(), proc, state, "test-shell")
	require.ErrorIs(t, err, errUtils.ErrCastSessionShellExited)
	require.ErrorIs(t, err, waitErr)
	assert.Contains(t, err.Error(), "test-shell")
	assert.Contains(t, err.Error(), "bad shell option")
	assert.NotContains(t, err.Error(), "\x1b[")
}
