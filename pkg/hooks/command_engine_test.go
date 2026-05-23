package hooks

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// testExePath returns the running test binary, used as a cross-platform
// stand-in for arbitrary subprocess behavior (see TestMain).
func testExePath(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	return exe
}

// runEngine builds an ExecContext from a Hook and runs the CommandEngine.
func runEngine(t *testing.T, hook *Hook) (*Output, error) {
	t.Helper()
	kind := &Kind{
		Name:      "command",
		OnFailure: OnFailureWarn,
		Engine:    &CommandEngine{},
	}
	resolved := kind.ResolveDefaults(hook)
	ctx := &ExecContext{
		Hook: resolved,
		Kind: kind,
		AtmosConfig: &schema.AtmosConfiguration{
			TerraformDirAbsolutePath: t.TempDir(),
		},
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "test-stack",
			ComponentFromArg: "test-component",
		},
	}
	return kind.Engine.Run(ctx)
}

func TestCommandEngine_RejectsEmptyCommand(t *testing.T) {
	_, err := runEngine(t, &Hook{Kind: "command"})
	require.Error(t, err)
	// Error builder wraps ErrInvalidConfig; explanation/context is attached
	// as hints (not in Error() string) — assertion checks the sentinel.
	assert.ErrorIs(t, err, errUtils.ErrInvalidConfig)
}

func TestCommandEngine_SuccessNoOutput(t *testing.T) {
	// Use the test binary itself as a no-op command via os.Executable(). The
	// _ATMOS_TEST_WRITE_OUTPUT gate is not set, so it just runs the test
	// suite again (without filtering) — that's fine; we don't assert on
	// its stdout. But to avoid recursive test execution, we use a
	// non-matching test filter via the -run flag.
	exe := testExePath(t)
	hook := &Hook{
		Kind:    "command",
		Command: exe,
		Args:    []string{"-test.run", "^$"}, // run no tests, exit cleanly
	}
	out, err := runEngine(t, hook)
	require.NoError(t, err)
	// No structured output produced; artifact is nil.
	assert.Nil(t, out.Artifact)
}

func TestCommandEngine_CapturesOutputFile(t *testing.T) {
	exe := testExePath(t)
	hook := &Hook{
		Kind:    "command",
		Command: exe,
		Args:    []string{"-test.run", "^$"},
		Env: map[string]string{
			"_ATMOS_TEST_WRITE_OUTPUT": "1",
			"_ATMOS_TEST_OUTPUT_BODY":  "hello from tool",
		},
	}
	out, err := runEngine(t, hook)
	require.NoError(t, err)
	require.NotNil(t, out.Artifact)
	assert.Equal(t, "hello from tool", string(out.Artifact.Body))
	assert.Equal(t, "command", out.Artifact.Metadata["kind"])
	assert.Equal(t, "test-stack", out.Artifact.Metadata["stack"])
	assert.Equal(t, "test-component", out.Artifact.Metadata["component"])
}

func TestCaptureOutput_AllowsNilInfo(t *testing.T) {
	outputFile := filepath.Join(t.TempDir(), "output")
	require.NoError(t, os.WriteFile(outputFile, []byte("hello"), 0o600))

	out := captureOutput(&ExecContext{
		Hook: &Hook{Kind: "command"},
		Kind: &Kind{Name: "command"},
	}, outputFile)

	require.NotNil(t, out.Artifact)
	assert.Equal(t, "command", out.Artifact.Metadata["kind"])
	assert.NotContains(t, out.Artifact.Metadata, "stack")
	assert.NotContains(t, out.Artifact.Metadata, "component")
}

func TestCommandEngine_OnFailureFailPropagates(t *testing.T) {
	exe := testExePath(t)
	hook := &Hook{
		Kind:      "command",
		Command:   exe,
		Args:      []string{"-test.run", "^$"},
		Env:       map[string]string{"_ATMOS_TEST_EXIT_ONE": "1"},
		OnFailure: OnFailureFail,
	}
	_, err := runEngine(t, hook)
	require.Error(t, err)
}

func TestCommandEngine_OnFailureWarnDoesNotPropagate(t *testing.T) {
	exe := testExePath(t)
	hook := &Hook{
		Kind:      "command",
		Command:   exe,
		Args:      []string{"-test.run", "^$"},
		Env:       map[string]string{"_ATMOS_TEST_EXIT_ONE": "1"},
		OnFailure: OnFailureWarn,
	}
	_, err := runEngine(t, hook)
	require.NoError(t, err, "warn mode should not propagate the subprocess error")
}

func TestCommandEngine_OnFailureIgnoreDoesNotPropagate(t *testing.T) {
	exe := testExePath(t)
	hook := &Hook{
		Kind:      "command",
		Command:   exe,
		Args:      []string{"-test.run", "^$"},
		Env:       map[string]string{"_ATMOS_TEST_EXIT_ONE": "1"},
		OnFailure: OnFailureIgnore,
	}
	_, err := runEngine(t, hook)
	require.NoError(t, err)
}

func TestCommandEngine_ExpandsAtmosVarsInArgs(t *testing.T) {
	// Capture which file path was passed in args via $ATMOS_OUTPUT_FILE
	// expansion by having the helper subprocess write the value back.
	exe := testExePath(t)
	hook := &Hook{
		Kind:    "command",
		Command: exe,
		// The args themselves don't affect the helper, but we exercise
		// the expander on a tag we know exists.
		Args: []string{"-test.run", "^$", "--", "$ATMOS_COMPONENT_PATH", "$ATMOS_STACK"},
		Env: map[string]string{
			"_ATMOS_TEST_WRITE_OUTPUT": "1",
			"_ATMOS_TEST_OUTPUT_BODY":  "$ATMOS_STACK",
		},
	}
	out, err := runEngine(t, hook)
	require.NoError(t, err)
	require.NotNil(t, out.Artifact)
	// The Env-value `$ATMOS_STACK` should NOT be expanded inside the
	// subprocess body (we write the literal env value, which already had
	// the engine substitute $ATMOS_STACK → "test-stack" before exec).
	assert.Equal(t, "test-stack", string(out.Artifact.Body))
}

func TestCommandEngine_InvokesResultHandler(t *testing.T) {
	exe := testExePath(t)
	handlerCalled := false
	kind := &Kind{
		Name:   "command-with-handler",
		Engine: &CommandEngine{},
		ResultHandler: func(ctx *ExecContext) (*Summary, error) {
			handlerCalled = true
			return &Summary{
				Kind:   ctx.Hook.Kind,
				Status: StatusSuccess,
				Title:  "ran",
				Body:   "**done**",
			}, nil
		},
	}
	hook := &Hook{
		Kind:    "command-with-handler",
		Command: exe,
		Args:    []string{"-test.run", "^$"},
		Env: map[string]string{
			"_ATMOS_TEST_WRITE_OUTPUT": "1",
			"_ATMOS_TEST_OUTPUT_BODY":  "ignored",
		},
	}
	ctx := &ExecContext{
		Hook:        kind.ResolveDefaults(hook),
		Kind:        kind,
		AtmosConfig: &schema.AtmosConfiguration{TerraformDirAbsolutePath: t.TempDir()},
		Info:        &schema.ConfigAndStacksInfo{Stack: "s", ComponentFromArg: "c"},
	}
	out, err := kind.Engine.Run(ctx)
	require.NoError(t, err)
	assert.True(t, handlerCalled)
	require.NotNil(t, out.Summary)
	assert.Equal(t, "command-with-handler", out.Summary.Kind)
	assert.Equal(t, StatusSuccess, out.Summary.Status)
	assert.Equal(t, "ran", out.Summary.Title)
}

func TestCommandEngine_CommandKindIsRegistered(t *testing.T) {
	k, ok := GetKind("command")
	require.True(t, ok, "generic command kind must self-register via init()")
	require.NotNil(t, k.Engine)
	assert.Equal(t, OnFailureWarn, k.OnFailure)
}

func TestResolveBinaryOnPath_RejectsNonExecutableUnixFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix executable bits do not apply on Windows")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "tool")
	require.NoError(t, os.WriteFile(path, []byte("not executable"), 0o600))

	_, err := resolveBinaryOnPathWithEnv("tool", dir, "", "", runtime.GOOS)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCommandNotFound)
}

func TestResolveBinaryOnPath_UsesWindowsPATHEXTForToolchainPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trivy.EXE")
	require.NoError(t, os.WriteFile(path, []byte("exe"), 0o600))

	got, err := resolveBinaryOnPathWithEnv("trivy", dir, "", ".EXE;.BAT", "windows")
	require.NoError(t, err)
	assert.Equal(t, path, got)
}

func TestVerifyCommandAvailable_RejectsNonExecutableExplicitPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix executable bits do not apply on Windows")
	}

	path := filepath.Join(t.TempDir(), "tool")
	require.NoError(t, os.WriteFile(path, []byte("not executable"), 0o600))

	err := verifyCommandAvailable(path, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCommandNotFound)
}
