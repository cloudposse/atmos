package step

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/asciicast"
	"github.com/cloudposse/atmos/pkg/script"
	star "github.com/cloudposse/atmos/pkg/script/starlark"
)

func executeCastSource(t *testing.T, dir, source string) error {
	t.Helper()
	var output bytes.Buffer
	_, err := star.New().Execute(context.Background(), script.Spec{
		Name: "cast.star", Source: source, WorkingDirectory: dir,
		Steps: NewAutomationLibrary(nil, nil), Stdout: &output, Stderr: &output,
	})
	return err
}

func castTestSource(t *testing.T, body string) string {
	t.Helper()
	exe, err := os.Executable()
	require.NoError(t, err)
	return fmt.Sprintf(`rec = cast.recorder(output="demo.cast", defaults={"simulate":{"rate":"0s","duration":"0s","interval":"0s"}}, env={"_ATMOS_STEP_FAKE":"ok"})
command = %s
%s
`, strconv.Quote(strconv.Quote(exe)), body)
}

func TestDeclarativeCastGenerateAndValidate(t *testing.T) {
	initShellTestIO(t)
	dir := t.TempDir()
	body := `rec.comment("A command")
rec.run(command, expect="fake-atmos-output", reject=cast.regex("missing-[0-9]+"), interval="0s", prompt={"text":"$ ","style":"command"})
rec.generate()`
	require.NoError(t, executeCastSource(t, dir, castTestSource(t, body)))
	path := filepath.Join(dir, "demo.cast")
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, executeCastSource(t, dir, castTestSource(t, `rec.comment("A command")
rec.run(command, expect="fake-atmos-output", reject=cast.regex("missing-[0-9]+"), interval="0s", prompt={"text":"$ ","style":"command"})
rec.validate()`)))
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
	require.ErrorContains(t, executeCastSource(t, dir, castTestSource(t, `rec.run(command, expect="changed")
rec.validate()`)), "regenerate")
}

func TestDeclarativeCastFailurePreservesArtifactAndRunsCleanup(t *testing.T) {
	initShellTestIO(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.cast")
	require.NoError(t, os.WriteFile(path, []byte("previous recording"), 0o600))
	cleanup := filepath.Join(dir, "cleanup-me")
	require.NoError(t, os.WriteFile(cleanup, []byte("fixture"), 0o600))
	source := castTestSource(t, fmt.Sprintf(`def cleanup(c):
    c.run(command, env={"_ATMOS_STEP_FAKE":"rm-glob-and-fail", "_ATMOS_STEP_FAKE_RM_GLOB":%q}, exit_code=1, interval="0s")
rec.cleanup(cleanup)
rec.run(command, display="only-in-typed-command", expect="only-in-typed-command", interval="0s")
rec.generate()`, cleanup))
	require.ErrorContains(t, executeCastSource(t, dir, source), "missing")
	saved, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "previous recording", string(saved))
	require.NoFileExists(t, cleanup)
}

func TestDeclarativeCastExpectedFailureAndPolicy(t *testing.T) {
	initShellTestIO(t)
	dir := t.TempDir()
	source := castTestSource(t, `rec.run(command, env={"_ATMOS_STEP_FAKE":"fail"}, exit_code=3, expect="fake-atmos-error", interval="0s")
rec.generate()`)
	require.NoError(t, executeCastSource(t, dir, source))
	path := filepath.Join(dir, "demo.cast")
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	require.ErrorContains(t, executeCastSource(t, dir, `def policy(text):
    fail("policy refused recording")
rec = cast.recorder(output="demo.cast", policy=policy)
rec.prompt()
rec.generate()`), "policy refused")
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestDeclarativeCastTerminal(t *testing.T) {
	initShellTestIO(t)
	dir := t.TempDir()
	exe, err := os.Executable()
	require.NoError(t, err)
	source := fmt.Sprintf(`rec=cast.recorder(output="demo.cast", shell=%q, write_rate="0s", key_interval="0s", env={"_ATMOS_STEP_SESSION_SHELL":"1","_ATMOS_STEP_SESSION_BANNER":"1"})
rec.terminal.wait("shell-up", timeout="10s")
rec.terminal.send("printf ready")
rec.terminal.wait("ready", timeout="10s")
rec.terminal.close()
rec.prompt()
rec.generate()`, exe)
	require.NoError(t, executeCastSource(t, dir, source))
	_, events, err := asciicast.ReadEvents(filepath.Join(dir, "demo.cast"))
	require.NoError(t, err)
	require.NotEmpty(t, events)
}

// TestDeclarativeCastTerminalHiddenCommandDoesNotLeak proves a wait does not
// match the terminal's echo of the typed hidden command: show() must not
// re-enable recording until the hidden command has really finished, or its
// output is published in the cast.
func TestDeclarativeCastTerminalHiddenCommandDoesNotLeak(t *testing.T) {
	initShellTestIO(t)
	dir := t.TempDir()
	exe, err := os.Executable()
	require.NoError(t, err)
	// The fake shell prints "out:<word>" per word after a delay, so the bare
	// words below appear immediately only in the terminal's echo of the command.
	source := fmt.Sprintf(`rec=cast.recorder(output="demo.cast", shell=%q, write_rate="0s", key_interval="0s", env={"_ATMOS_STEP_SESSION_SHELL":"1","_ATMOS_STEP_SESSION_BANNER":"1"})
rec.terminal.wait("shell-up", timeout="10s")
rec.terminal.hide()
rec.terminal.send("delay-print HIDDEN-SENTINEL READY")
rec.terminal.wait("READY", timeout="10s")
rec.terminal.show()
rec.terminal.send("delay-print VISIBLE")
rec.terminal.wait("out:VISIBLE", timeout="10s")
rec.terminal.close()
rec.generate()`, exe)
	require.NoError(t, executeCastSource(t, dir, source))

	_, events, err := asciicast.ReadEvents(filepath.Join(dir, "demo.cast"))
	require.NoError(t, err)
	var recorded strings.Builder
	for _, event := range events {
		recorded.WriteString(event.Data)
	}
	require.NotContains(t, recorded.String(), "HIDDEN-SENTINEL")
	require.Contains(t, recorded.String(), "out:VISIBLE")
}

func TestDeclarativeCastValidatesBeforeSetup(t *testing.T) {
	initShellTestIO(t)
	dir := t.TempDir()
	source := castTestSource(t, `def setup(c):
    c.run(command, env={"_ATMOS_STEP_FAKE":"fail"}, interval="0s")
rec.setup(setup)
rec.terminal.key("not-a-key")
rec.generate()`)
	require.ErrorContains(t, executeCastSource(t, dir, source), "key")
	require.NoFileExists(t, filepath.Join(dir, "demo.cast"))
}

func TestDeclarativeCastValidationSkipsSetupAndCleanup(t *testing.T) {
	initShellTestIO(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "must-survive-validation")
	declaration := castTestSource(t, fmt.Sprintf(`def remove(c):
    c.run(command, env={"_ATMOS_STEP_FAKE":"rm-glob-and-fail", "_ATMOS_STEP_FAKE_RM_GLOB":%q}, exit_code=1, interval="0s")
rec.setup(remove)
rec.cleanup(remove)
rec.hidden(remove)
rec.run(command, expect="fake-atmos-output", interval="0s")
`, target))
	require.NoError(t, executeCastSource(t, dir, declaration+"rec.generate()"))
	require.NoError(t, os.WriteFile(target, []byte("keep"), 0o600))
	require.NoError(t, executeCastSource(t, dir, declaration+"rec.validate()"))
	require.FileExists(t, target)
}

func TestDeclarativeCastSetupFailureStillCleansUp(t *testing.T) {
	initShellTestIO(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "partial-setup")
	require.NoError(t, os.WriteFile(target, []byte("cleanup"), 0o600))
	source := castTestSource(t, fmt.Sprintf(`def setup(c):
    c.run(command, env={"_ATMOS_STEP_FAKE":"fail"}, interval="0s")
def cleanup(c):
    c.run(command, env={"_ATMOS_STEP_FAKE":"rm-glob-and-fail", "_ATMOS_STEP_FAKE_RM_GLOB":%q}, exit_code=1, interval="0s")
rec.setup(setup)
rec.cleanup(cleanup)
rec.prompt()
rec.generate()`, target))
	require.ErrorContains(t, executeCastSource(t, dir, source), "exit status 3")
	require.NoFileExists(t, target)
	require.NoFileExists(t, filepath.Join(dir, "demo.cast"))
}

func TestDeclarativeCastCleanupFailurePreservesOldCast(t *testing.T) {
	initShellTestIO(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.cast")
	require.NoError(t, os.WriteFile(path, []byte("keep"), 0o600))
	source := castTestSource(t, `def cleanup(c):
    c.run(command, env={"_ATMOS_STEP_FAKE":"fail"}, interval="0s")
rec.cleanup(cleanup)
rec.run(command, expect="fake-atmos-output", interval="0s")
rec.generate()`)
	require.ErrorContains(t, executeCastSource(t, dir, source), "exit status 3")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "keep", string(content))
}

func TestDeclarativeCastScreenshotAndHiddenOutput(t *testing.T) {
	initShellTestIO(t)
	dir := t.TempDir()
	source := castTestSource(t, `def hidden(c):
    c.run(command, env={"_ATMOS_STEP_FAKE":"fail"}, exit_code=3, interval="0s")
rec.hidden(hidden)
rec.run(command, expect="fake-atmos-output", interval="0s")
rec.screenshot("frame.png")
rec.generate()`)
	require.NoError(t, executeCastSource(t, dir, source))
	require.FileExists(t, filepath.Join(dir, "frame.png"))
	content, err := os.ReadFile(filepath.Join(dir, "demo.cast"))
	require.NoError(t, err)
	require.NotContains(t, string(content), "fake-atmos-error")
}

func TestDeclarativeCastParallelAndDryRun(t *testing.T) {
	initShellTestIO(t)
	dir := t.TempDir()
	require.ErrorContains(t, executeCastSource(t, dir, `def record():
    rec=cast.recorder(output="demo.cast")
    rec.prompt()
    rec.generate()
steps.parallel(functions=[record])`), "exclusive")
	_, err := star.New().Execute(t.Context(), script.Spec{Source: `rec=cast.recorder(output="never.cast");rec.run("must-not-execute");rec.generate()`, DryRun: true, WorkingDirectory: dir, Steps: NewAutomationLibrary(nil, nil)})
	require.NoError(t, err)
	require.NoFileExists(t, filepath.Join(dir, "never.cast"))
}

func TestDeclarativeCastCancellationRunsCleanup(t *testing.T) {
	initShellTestIO(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "cancel-cleanup")
	require.NoError(t, os.WriteFile(target, []byte("remove"), 0o600))
	source := castTestSource(t, fmt.Sprintf(`def cleanup(c):
    c.run(command, env={"_ATMOS_STEP_FAKE":"rm-glob-and-fail", "_ATMOS_STEP_FAKE_RM_GLOB":%q}, exit_code=1, interval="0s")
rec.cleanup(cleanup)
rec.run(command, env={"_ATMOS_STEP_FAKE":"sleep", "_ATMOS_STEP_FAKE_SLEEP_MS":"5000"}, interval="0s")
rec.generate()`, target))
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	_, err := star.New().Execute(ctx, script.Spec{Name: "cancel.star", Source: source, WorkingDirectory: dir, Steps: NewAutomationLibrary(nil, nil)})
	require.Error(t, err)
	require.NoFileExists(t, target)
	require.NoFileExists(t, filepath.Join(dir, "demo.cast"))
}
