package ui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
)

func TestShouldUseStreamingUI_ExplicitlyDisabled(t *testing.T) {
	// --ui=false explicitly set.
	result := ShouldUseStreamingUI(true, false, true, "plan")
	assert.False(t, result)
}

func TestShouldUseStreamingUI_ConfigDisabled(t *testing.T) {
	// Neither flag nor config enabled.
	result := ShouldUseStreamingUI(false, false, false, "plan")
	assert.False(t, result)
}

func TestShouldUseStreamingUI_UnsupportedCommand(t *testing.T) {
	// Even if enabled, unsupported commands return false.
	unsupportedCommands := []string{"version", "workspace", "fmt", "validate", "import", "state", "output"}
	for _, cmd := range unsupportedCommands {
		result := ShouldUseStreamingUI(true, true, true, cmd)
		assert.False(t, result, "command %s should not support streaming UI", cmd)
	}
}

func TestShouldUseStreamingUI_SupportedCommands(t *testing.T) {
	// Test that supported commands (plan, apply, init, destroy) still need enablement.
	// Note: refresh is NOT supported due to poor -json streaming support.
	supportedCommands := []string{"plan", "apply", "init", "destroy"}
	for _, cmd := range supportedCommands {
		// In non-CI, non-TTY environment, this would still return false.
		// But we're testing the command filtering logic.
		result := ShouldUseStreamingUI(false, false, false, cmd)
		assert.False(t, result, "command %s needs enabled flag or config to use streaming UI", cmd)
	}
}

func TestUIRequestedButUnsupported(t *testing.T) {
	tests := []struct {
		name       string
		uiFlagSet  bool
		uiFlag     bool
		config     bool
		subCommand string
		expect     bool
	}{
		{name: "refresh with --ui=true", uiFlagSet: true, uiFlag: true, subCommand: "refresh", expect: true},
		{name: "refresh via config", config: true, subCommand: "refresh", expect: true},
		{name: "refresh with --ui=false", uiFlagSet: true, uiFlag: false, subCommand: "refresh", expect: false},
		{name: "refresh not requested at all", subCommand: "refresh", expect: false},
		{name: "apply with --ui=true (supported)", uiFlagSet: true, uiFlag: true, subCommand: "apply", expect: false},
		{name: "destroy with --ui=true (supported)", uiFlagSet: true, uiFlag: true, subCommand: "destroy", expect: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := UIRequestedButUnsupported(tt.uiFlagSet, tt.uiFlag, tt.config, tt.subCommand)
			assert.Equal(t, tt.expect, result)
		})
	}
}

func TestBuildArgsWithJSON_AddsFlagForPlan(t *testing.T) {
	args := []string{"plan", "-out", "plan.out"}
	result := buildArgsWithJSON(args)
	assert.Contains(t, result, "-json")
	assert.Contains(t, result, "-compact-warnings")
	// Flags should be after "plan".
	assert.Equal(t, "plan", result[0])
	assert.Equal(t, "-json", result[1])
	assert.Equal(t, "-compact-warnings", result[2])
}

func TestBuildArgsWithJSON_AddsFlagForApply(t *testing.T) {
	args := []string{"apply", "-auto-approve"}
	result := buildArgsWithJSON(args)
	assert.Contains(t, result, "-json")
	assert.Contains(t, result, "-compact-warnings")
	assert.Equal(t, "apply", result[0])
	assert.Equal(t, "-json", result[1])
	assert.Equal(t, "-compact-warnings", result[2])
}

func TestBuildArgsWithJSON_AddsFlagForInit(t *testing.T) {
	args := []string{"init", "-reconfigure"}
	result := buildArgsWithJSON(args)
	assert.Contains(t, result, "-json")
	assert.Contains(t, result, "-compact-warnings")
	assert.Equal(t, "init", result[0])
	assert.Equal(t, "-json", result[1])
	assert.Equal(t, "-compact-warnings", result[2])
}

func TestBuildArgsWithJSON_AddsFlagForRefresh(t *testing.T) {
	args := []string{"refresh"}
	result := buildArgsWithJSON(args)
	assert.Contains(t, result, "-json")
	assert.Contains(t, result, "-compact-warnings")
	assert.Equal(t, "refresh", result[0])
	assert.Equal(t, "-json", result[1])
	assert.Equal(t, "-compact-warnings", result[2])
}

// TestBuildArgsWithJSON_AddsFlagForDestroy is a regression test: isJSONSubCommand
// previously omitted "destroy", so -json/-compact-warnings were prepended BEFORE the
// subcommand instead of after it (e.g. "-json -compact-warnings destroy -auto-approve"),
// which terraform's CLI rejects outright ("Invalid flags before the subcommand").
func TestBuildArgsWithJSON_AddsFlagForDestroy(t *testing.T) {
	args := []string{"destroy", "-auto-approve"}
	result := buildArgsWithJSON(args)
	assert.Contains(t, result, "-json")
	assert.Contains(t, result, "-compact-warnings")
	assert.Equal(t, "destroy", result[0])
	assert.Equal(t, "-json", result[1])
	assert.Equal(t, "-compact-warnings", result[2])
	assert.Equal(t, "-auto-approve", result[3])
}

func TestBuildArgsWithJSON_DoesNotDuplicateFlag(t *testing.T) {
	args := []string{"plan", "-json", "-compact-warnings", "-out", "plan.out"}
	result := buildArgsWithJSON(args)
	// Count flag occurrences.
	jsonCount := 0
	compactCount := 0
	for _, arg := range result {
		if arg == "-json" {
			jsonCount++
		}
		if arg == "-compact-warnings" {
			compactCount++
		}
	}
	assert.Equal(t, 1, jsonCount, "-json should not be duplicated")
	assert.Equal(t, 1, compactCount, "-compact-warnings should not be duplicated")
}

func TestBuildArgsWithJSON_AddsCompactWarningsWhenOnlyJSONPresent(t *testing.T) {
	args := []string{"plan", "-json", "-out", "plan.out"}
	result := buildArgsWithJSON(args)
	assert.Contains(t, result, "-json")
	assert.Contains(t, result, "-compact-warnings")
	// -json should still be at position 1, -compact-warnings added after.
	assert.Equal(t, "plan", result[0])
}

func TestBuildArgsWithJSON_NoSubcommandAtStart(t *testing.T) {
	// Edge case: args don't start with a recognized subcommand.
	args := []string{"-var", "foo=bar"}
	result := buildArgsWithJSON(args)
	assert.Contains(t, result, "-json")
	assert.Contains(t, result, "-compact-warnings")
	// In this case, flags should be prepended.
	assert.Equal(t, "-json", result[0])
	assert.Equal(t, "-compact-warnings", result[1])
}

func TestDetectOutputContentType(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected outputContentType
	}{
		{"empty string", "", outputContentTypeDefault},
		{"sensitive marker", "<sensitive>", outputContentTypeSensitive},
		{"null value", "null", outputContentTypeNull},
		{"true boolean", "true", outputContentTypeBoolean},
		{"false boolean", "false", outputContentTypeBoolean},
		{"integer", "42", outputContentTypeNumber},
		{"negative integer", "-10", outputContentTypeNumber},
		{"float", "3.14", outputContentTypeNumber},
		{"string value", "hello world", outputContentTypeDefault},
		{"url string", "https://example.com", outputContentTypeDefault},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectOutputContentType(tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsNumericString(t *testing.T) {
	tests := []struct {
		value    string
		expected bool
	}{
		{"42", true},
		{"-10", true},
		{"3.14", true},
		{"0", true},
		{"1e10", true},
		{"hello", false},
		{"", false},
		{"true", false},
		{"false", false},
		{"null", false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			result := isNumericString(tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractOutFlag(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{"flag with separate value", []string{"plan", "-out", "myplan.tfplan"}, "myplan.tfplan"},
		{"flag with equals", []string{"plan", "-out=myplan.tfplan"}, "myplan.tfplan"},
		{"no flag", []string{"plan"}, ""},
		{"other flags only", []string{"plan", "-var", "foo=bar"}, ""},
		{"flag at end without value", []string{"plan", "-out"}, ""},
		{"empty args", []string{}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractOutFlag(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasFlag(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		flag     string
		expected bool
	}{
		{"flag present", []string{"apply", "-auto-approve"}, "-auto-approve", true},
		{"flag absent", []string{"apply"}, "-auto-approve", false},
		{"multiple flags with target", []string{"apply", "-var", "x=1", "-auto-approve"}, "-auto-approve", true},
		{"similar prefix not matched", []string{"apply", "-auto-approve-all"}, "-auto-approve", false},
		{"empty args", []string{}, "-auto-approve", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasFlag(tt.args, tt.flag)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractPlanFile(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{"--from-plan with separate value", []string{"apply", "--from-plan", "plan.tfplan"}, "plan.tfplan"},
		{"--from-plan with equals", []string{"apply", "--from-plan=plan.tfplan"}, "plan.tfplan"},
		{"--planfile with separate value", []string{"apply", "--planfile", "plan.tfplan"}, "plan.tfplan"},
		{"--planfile with equals", []string{"apply", "--planfile=plan.tfplan"}, "plan.tfplan"},
		{"positional planfile", []string{"apply", "myplan.tfplan"}, "myplan.tfplan"},
		{"positional planfile without .tfplan suffix", []string{"apply", "tfplan"}, "tfplan"},
		{"positional planfile with arbitrary name", []string{"apply", "my-saved-plan"}, "my-saved-plan"},
		{"no planfile", []string{"apply", "-auto-approve"}, ""},
		{"positional Terraform config file is not a planfile", []string{"apply", "config.tf"}, ""},
		{"positional tfvars file is not a planfile", []string{"apply", "extra.tfvars"}, ""},
		{"empty args", []string{}, ""},
		{"single arg apply only", []string{"apply"}, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPlanFile(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildPlanArgs(t *testing.T) {
	planFile := filepath.Join("tmp", "plan.tfplan")
	tests := []struct {
		name     string
		args     []string
		planFile string
		expected []string
	}{
		{
			name:     "basic apply to plan",
			args:     []string{"apply", "-var", "foo=bar"},
			planFile: planFile,
			expected: []string{"plan", "-var", "foo=bar", "-out=" + planFile},
		},
		{
			name:     "strips auto-approve",
			args:     []string{"apply", "-auto-approve", "-var", "x=1"},
			planFile: planFile,
			expected: []string{"plan", "-var", "x=1", "-out=" + planFile},
		},
		{
			name:     "apply only",
			args:     []string{"apply"},
			planFile: planFile,
			expected: []string{"plan", "-out=" + planFile},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildPlanArgs(tt.args, tt.planFile)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildApplyArgs(t *testing.T) {
	planFile := filepath.Join("tmp", "plan.tfplan")
	otherPlanFile := filepath.Join("var", "tmp", "my-plan.tfplan")
	tests := []struct {
		name     string
		planFile string
		expected []string
	}{
		{"basic planfile", planFile, []string{"apply", planFile}},
		{"different path", otherPlanFile, []string{"apply", otherPlanFile}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildApplyArgs(tt.planFile)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildDestroyPlanArgs(t *testing.T) {
	planFile := filepath.Join("tmp", "destroy.tfplan")
	tests := []struct {
		name     string
		args     []string
		planFile string
		expected []string
	}{
		{
			name:     "basic destroy to plan",
			args:     []string{"destroy", "-var", "foo=bar"},
			planFile: planFile,
			expected: []string{"plan", "-destroy", "-var", "foo=bar", "-out=" + planFile},
		},
		{
			name:     "strips auto-approve",
			args:     []string{"destroy", "-auto-approve"},
			planFile: planFile,
			expected: []string{"plan", "-destroy", "-out=" + planFile},
		},
		{
			name:     "destroy only",
			args:     []string{"destroy"},
			planFile: planFile,
			expected: []string{"plan", "-destroy", "-out=" + planFile},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildDestroyPlanArgs(tt.args, tt.planFile)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatOutputValue(t *testing.T) {
	tests := []struct {
		name     string
		value    any
		expected string
	}{
		{"string", "hello", "hello"},
		{"integer as float64", float64(42), "42"},
		{"float", 3.14, "3.14"},
		{"boolean true", true, "true"},
		{"boolean false", false, "false"},
		{"nil", nil, "null"},
		{"simple map", map[string]any{"key": "value"}, "{\n  \"key\": \"value\"\n}"},
		{"slice", []any{"a", "b"}, "[\n  \"a\",\n  \"b\"\n]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatOutputValue(tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestExecuteApply_FailsFastWhenStdinNotTTY is a regression test: previously ExecuteApply
// (without -auto-approve) always ran the full streaming plan phase before discovering, only
// at the confirmation step, that stdin wasn't a TTY - duplicating real plan/refresh work and
// then falling back to a plain re-run that also fails. In a `go test` process stdin is not a
// TTY, so this exercises the fast-fail path directly: it must return quickly with
// ErrStreamingNotSupported, never attempting to spawn a real terraform subprocess.
func TestExecuteApply_FailsFastWhenStdinNotTTY(t *testing.T) {
	opts := &ExecuteOptions{
		Command:    "terraform",
		Args:       []string{"apply"}, // no -auto-approve: confirmation would be required.
		WorkingDir: t.TempDir(),
		SubCommand: subCommandApply,
	}

	err := ExecuteApply(context.Background(), opts)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrStreamingNotSupported)
}

// TestExecuteDestroy_FailsFastWhenStdinNotTTY mirrors TestExecuteApply_FailsFastWhenStdinNotTTY
// for the destroy two-phase flow.
func TestExecuteDestroy_FailsFastWhenStdinNotTTY(t *testing.T) {
	opts := &ExecuteOptions{
		Command:    "terraform",
		Args:       []string{"destroy"}, // no -auto-approve: confirmation would be required.
		WorkingDir: t.TempDir(),
		SubCommand: subCommandDestroy,
	}

	err := ExecuteDestroy(context.Background(), opts)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrStreamingNotSupported)
}

// noCancelModel is a minimal tea.Model that does not implement cancellableModel, used to
// verify modelWasCancelled defaults safely to false for models that don't opt in.
type noCancelModel struct{}

func (noCancelModel) Init() tea.Cmd                       { return nil }
func (noCancelModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return noCancelModel{}, nil }
func (noCancelModel) View() string                        { return "" }

func TestModelWasCancelled(t *testing.T) {
	t.Run("cancelled via key press", func(t *testing.T) {
		m := NewModel("comp", "stack", "plan", strings.NewReader(""))
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
		assert.True(t, modelWasCancelled(updated))
	})

	t.Run("done on its own, not cancelled", func(t *testing.T) {
		m := NewModel("comp", "stack", "plan", strings.NewReader(""))
		updated, _ := m.Update(doneMsg{exitCode: 0})
		assert.False(t, modelWasCancelled(updated))
	})

	t.Run("model without Cancelled() defaults to false", func(t *testing.T) {
		assert.False(t, modelWasCancelled(noCancelModel{}))
	})
}

// TestKillIfCancelled_KillsRealProcess verifies a cancelled run actually terminates a
// long-running subprocess instead of leaving it running invisibly in the background - this
// is the fix for a real hang where Ctrl-C during a streaming apply never stopped terraform.
func TestKillIfCancelled_KillsRealProcess(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err)

	cmd := exec.Command(exePath, "-test.run=^$")
	cmd.Env = append(os.Environ(), "_ATMOS_TEST_SLEEP_SECONDS=30")
	require.NoError(t, cmd.Start())

	killIfCancelled(true, cmd)

	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	select {
	case err := <-waitDone:
		require.Error(t, err, "a killed process should report a non-nil wait error")
	case <-time.After(5 * time.Second):
		t.Fatal("subprocess (30s sleep) was not killed within 5s - killIfCancelled did not stop it")
	}
}

// TestKillIfCancelled_LeavesProcessRunningWhenNotCancelled verifies the normal (non-cancelled)
// path leaves the subprocess alone, since Execute() still needs to wait for it to finish.
func TestKillIfCancelled_LeavesProcessRunningWhenNotCancelled(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err)

	cmd := exec.Command(exePath, "-test.run=^$")
	cmd.Env = append(os.Environ(), "_ATMOS_TEST_SLEEP_SECONDS=2")
	require.NoError(t, cmd.Start())

	killIfCancelled(false, cmd)

	require.NoError(t, cmd.Wait(), "process should exit on its own after its 2s sleep, undisturbed")
}

// ptrInt returns a pointer to i, used by table-driven tests that need to distinguish
// "no expectation" (nil) from "expect exit code 0" (non-nil, pointing at 0).
func ptrInt(i int) *int {
	return &i
}

// TestStreamStderrToLog_LogsEachLine verifies streamStderrToLog forwards every line read from
// the given reader to the Atmos debug logger, so terraform's stderr diagnostics (backend
// errors, plugin crashes) aren't silently dropped.
func TestStreamStderrToLog_LogsEachLine(t *testing.T) {
	origLevel := log.GetLevel()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	log.SetLevel(log.DebugLevel)
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetLevel(origLevel)
	})

	streamStderrToLog(strings.NewReader("first diagnostic line\nsecond diagnostic line\n"))

	output := buf.String()
	assert.Contains(t, output, "first diagnostic line")
	assert.Contains(t, output, "second diagnostic line")
}

// TestNewStreamingCommand_StartsRealProcess verifies newStreamingCommand actually starts the
// subprocess and returns a readable stdout pipe, using the test binary itself (per this
// repo's self-re-exec pattern) instead of a real terraform binary.
func TestNewStreamingCommand_StartsRealProcess(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err)

	opts := &ExecuteOptions{
		Command:    exePath,
		WorkingDir: t.TempDir(),
		Env:        append(os.Environ(), "_ATMOS_TEST_SLEEP_SECONDS=0"),
	}

	cmd, stdout, err := newStreamingCommand(context.Background(), opts, []string{"-test.run=^$"})
	require.NoError(t, err)
	require.NotNil(t, cmd)
	require.NotNil(t, stdout)

	data, readErr := io.ReadAll(stdout)
	require.NoError(t, readErr)
	assert.Empty(t, data, "fake subprocess writes nothing to stdout")

	require.NoError(t, cmd.Wait(), "fake subprocess should exit 0")
}

// TestNewStreamingCommand_StartError verifies a nonexistent binary surfaces ErrCommandStart
// instead of a raw exec error, so callers can rely on the sentinel for classification.
func TestNewStreamingCommand_StartError(t *testing.T) {
	opts := &ExecuteOptions{
		Command:    filepath.Join(t.TempDir(), "nonexistent-terraform-binary"),
		WorkingDir: t.TempDir(),
	}

	cmd, stdout, err := newStreamingCommand(context.Background(), opts, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCommandStart)
	assert.Nil(t, cmd)
	assert.Nil(t, stdout)
}

// TestNewStreamingCommand_StdoutPipeError uses the execCommandContext DI seam to return a cmd
// whose Stdout is already set, which makes the real cmd.StdoutPipe() call fail - verifying
// that failure is wrapped in ErrStdoutPipe rather than surfacing raw.
func TestNewStreamingCommand_StdoutPipeError(t *testing.T) {
	orig := execCommandContext
	t.Cleanup(func() { execCommandContext = orig })
	execCommandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, name, arg...)
		cmd.Stdout = &bytes.Buffer{} // Pre-set so cmd.StdoutPipe() fails.
		return cmd
	}

	opts := &ExecuteOptions{Command: "unused-because-of-seam", WorkingDir: t.TempDir()}
	cmd, stdout, err := newStreamingCommand(context.Background(), opts, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrStdoutPipe)
	assert.Nil(t, cmd)
	assert.Nil(t, stdout)
}

// TestNewStreamingCommand_StderrPipeError mirrors TestNewStreamingCommand_StdoutPipeError for
// the stderr pipe, which is used to stream diagnostics to the logger.
func TestNewStreamingCommand_StderrPipeError(t *testing.T) {
	orig := execCommandContext
	t.Cleanup(func() { execCommandContext = orig })
	execCommandContext = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, name, arg...)
		cmd.Stderr = &bytes.Buffer{} // Pre-set so cmd.StderrPipe() fails.
		return cmd
	}

	opts := &ExecuteOptions{Command: "unused-because-of-seam", WorkingDir: t.TempDir()}
	cmd, stdout, err := newStreamingCommand(context.Background(), opts, nil)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrStderrPipe)
	assert.Nil(t, cmd)
	assert.Nil(t, stdout)
}

// TestNewInitCommand_StartsRealProcessAndMergesOutput verifies newInitCommand starts the
// subprocess, merges stdout/stderr into a single reader, and reports completion via the done
// channel and captured error pointer.
func TestNewInitCommand_StartsRealProcessAndMergesOutput(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err)

	opts := &ExecuteOptions{
		Command:    exePath,
		Args:       []string{"-test.run=^$"},
		WorkingDir: t.TempDir(),
		Env:        append(os.Environ(), "_ATMOS_TEST_SLEEP_SECONDS=0"),
	}

	res, err := newInitCommand(context.Background(), opts)
	require.NoError(t, err)
	require.NotNil(t, res)

	data, readErr := io.ReadAll(res.reader)
	require.NoError(t, readErr)
	assert.Empty(t, data, "fake subprocess writes nothing")

	<-res.done
	require.NoError(t, *res.err, "fake subprocess should exit 0")
}

// TestNewInitCommand_StartError verifies a nonexistent binary surfaces ErrCommandStart.
func TestNewInitCommand_StartError(t *testing.T) {
	opts := &ExecuteOptions{
		Command:    filepath.Join(t.TempDir(), "nonexistent-terraform-binary"),
		WorkingDir: t.TempDir(),
	}

	res, err := newInitCommand(context.Background(), opts)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCommandStart)
	assert.Nil(t, res)
}

// TestRunTUIProgram_TUIErrorKillsProcess verifies that when the injected tea.Program runner
// fails, runTUIProgram kills the still-running subprocess and wraps the error in ErrTUIRun -
// otherwise a TUI crash would leave terraform running invisibly in the background.
func TestRunTUIProgram_TUIErrorKillsProcess(t *testing.T) {
	origRun := runTeaProgram
	t.Cleanup(func() { runTeaProgram = origRun })
	sentinelErr := errors.New("tui crashed")
	runTeaProgram = func(p *tea.Program) (tea.Model, error) {
		return nil, sentinelErr
	}

	exePath, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.Command(exePath, "-test.run=^$")
	cmd.Env = append(os.Environ(), "_ATMOS_TEST_SLEEP_SECONDS=30")
	require.NoError(t, cmd.Start())

	finalModel, cancelled, runErr := runTUIProgram(noCancelModel{}, cmd)

	require.Error(t, runErr)
	assert.ErrorIs(t, runErr, errUtils.ErrTUIRun)
	assert.ErrorIs(t, runErr, sentinelErr)
	assert.Nil(t, finalModel)
	assert.False(t, cancelled)

	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	select {
	case werr := <-waitDone:
		require.Error(t, werr, "process should have been killed after the TUI run failed")
	case <-time.After(5 * time.Second):
		t.Fatal("subprocess was not killed within 5s after TUI run error")
	}
}

// TestRunTUIProgram_CancelledKillsProcess verifies that when the final model reports the user
// cancelled (Ctrl-C/q), runTUIProgram kills the still-running subprocess and reports cancelled.
func TestRunTUIProgram_CancelledKillsProcess(t *testing.T) {
	origRun := runTeaProgram
	t.Cleanup(func() { runTeaProgram = origRun })
	runTeaProgram = func(p *tea.Program) (tea.Model, error) {
		return Model{cancelled: true}, nil
	}

	exePath, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.Command(exePath, "-test.run=^$")
	cmd.Env = append(os.Environ(), "_ATMOS_TEST_SLEEP_SECONDS=30")
	require.NoError(t, cmd.Start())

	finalModel, cancelled, runErr := runTUIProgram(NewModel("comp", "stack", "plan", strings.NewReader("")), cmd)

	require.NoError(t, runErr)
	assert.True(t, cancelled)
	m, ok := finalModel.(Model)
	require.True(t, ok)
	assert.True(t, m.Cancelled())

	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	select {
	case werr := <-waitDone:
		require.Error(t, werr, "process should have been killed after cancellation")
	case <-time.After(5 * time.Second):
		t.Fatal("subprocess was not killed within 5s after cancellation")
	}
}

// TestRunTUIProgram_SuccessLeavesProcessRunning verifies the normal (non-cancelled,
// non-error) path does not kill the subprocess - Execute() still needs to Wait() on it.
func TestRunTUIProgram_SuccessLeavesProcessRunning(t *testing.T) {
	origRun := runTeaProgram
	t.Cleanup(func() { runTeaProgram = origRun })
	runTeaProgram = func(p *tea.Program) (tea.Model, error) {
		return Model{}, nil // Finished on its own, not cancelled.
	}

	exePath, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.Command(exePath, "-test.run=^$")
	cmd.Env = append(os.Environ(), "_ATMOS_TEST_SLEEP_SECONDS=1")
	require.NoError(t, cmd.Start())

	finalModel, cancelled, runErr := runTUIProgram(NewModel("comp", "stack", "plan", strings.NewReader("")), cmd)

	require.NoError(t, runErr)
	assert.False(t, cancelled)
	_, ok := finalModel.(Model)
	require.True(t, ok)

	require.NoError(t, cmd.Wait(), "process should exit on its own after its 1s sleep, undisturbed")
}

// TestFinalizeExecuteResult covers finalizeExecuteResult's branches: a model-captured error
// takes precedence, a model-captured exit code overrides the wait exit code, a non-zero wait
// exit code is returned when the model has none, and a successful apply displays outputs
// while a successful plan does not error.
func TestFinalizeExecuteResult(t *testing.T) {
	tests := []struct {
		name         string
		opts         *ExecuteOptions
		model        *Model
		exitCode     int
		wantErrIs    error
		wantExitCode *int
	}{
		{
			name:      "model error takes precedence over exit code",
			opts:      &ExecuteOptions{SubCommand: subCommandApply},
			model:     &Model{tracker: NewResourceTracker(), err: errUtils.ErrTUIRun},
			exitCode:  0,
			wantErrIs: errUtils.ErrTUIRun,
		},
		{
			name:         "model exit code overrides wait exit code",
			opts:         &ExecuteOptions{SubCommand: subCommandApply},
			model:        &Model{tracker: NewResourceTracker(), exitCode: 3},
			exitCode:     0,
			wantExitCode: ptrInt(3),
		},
		{
			name:         "non-zero wait exit code returned when model has none",
			opts:         &ExecuteOptions{SubCommand: subCommandPlan},
			model:        &Model{tracker: NewResourceTracker()},
			exitCode:     2,
			wantExitCode: ptrInt(2),
		},
		{
			name:     "apply success displays outputs without error",
			opts:     &ExecuteOptions{SubCommand: subCommandApply},
			model:    &Model{tracker: NewResourceTracker()},
			exitCode: 0,
		},
		{
			name:     "plan success does not attempt to display outputs",
			opts:     &ExecuteOptions{SubCommand: subCommandPlan},
			model:    &Model{tracker: NewResourceTracker()},
			exitCode: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := finalizeExecuteResult(tt.opts, tt.model, tt.exitCode)
			switch {
			case tt.wantErrIs != nil:
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErrIs)
			case tt.wantExitCode != nil:
				require.Error(t, err)
				var exitErr errUtils.ExitCodeError
				require.ErrorAs(t, err, &exitErr)
				assert.Equal(t, *tt.wantExitCode, exitErr.Code)
			default:
				assert.NoError(t, err)
			}
		})
	}
}

// TestFinalizeExecuteInitResult covers finalizeExecuteInitResult's branches: a non-ExitError
// wait error propagates unchanged, an unexpected model type is rejected, a model-captured
// error takes precedence, a model-captured exit code overrides the wait exit code, and a
// clean completion returns nil.
func TestFinalizeExecuteInitResult(t *testing.T) {
	t.Run("non-exit wait error propagates", func(t *testing.T) {
		sentinel := errors.New("wait failed")
		err := finalizeExecuteInitResult(noCancelModel{}, sentinel)
		require.Error(t, err)
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("unexpected model type", func(t *testing.T) {
		err := finalizeExecuteInitResult(noCancelModel{}, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrUnexpectedModelType)
	})

	t.Run("model error takes precedence", func(t *testing.T) {
		err := finalizeExecuteInitResult(InitModel{err: errUtils.ErrTUIRun}, nil)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrTUIRun)
	})

	t.Run("model exit code overrides wait exit code", func(t *testing.T) {
		err := finalizeExecuteInitResult(InitModel{exitCode: 7}, nil)
		require.Error(t, err)
		var exitErr errUtils.ExitCodeError
		require.ErrorAs(t, err, &exitErr)
		assert.Equal(t, 7, exitErr.Code)
	})

	t.Run("clean completion returns nil", func(t *testing.T) {
		err := finalizeExecuteInitResult(InitModel{}, nil)
		assert.NoError(t, err)
	})
}

// TestPlanFilePath_AbsolutizesRelativeWorkingDir is a regression test. A relative working
// directory (e.g. atmosConfig.BasePath, which unlike BasePathAbsolute isn't guaranteed
// absolute) joined naively with a filename and passed as `-out=` to a subprocess whose cmd.Dir
// is already that same relative directory gets re-resolved against the subprocess's own cwd,
// silently doubling the path and failing with "no such file or directory" when the subprocess
// tries to create it. The function under test must always return an absolute path regardless
// of the working directory's form.
func TestPlanFilePath_AbsolutizesRelativeWorkingDir(t *testing.T) {
	tmp := t.TempDir()
	wd, err := os.Getwd()
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.Chdir(wd) })
	require.NoError(t, os.Chdir(tmp))

	relativeWorkingDir := filepath.Join(".workdir", "terraform", "core-ue2-auto-vpc-05bc9914")
	require.NoError(t, os.MkdirAll(relativeWorkingDir, 0o755))

	got := planFilePath(relativeWorkingDir, ".atmos-plan-123.tfplan")

	require.True(t, filepath.IsAbs(got), "expected an absolute path, got %q", got)
	// Build the expected prefix from os.Getwd() (called after the chdir above) rather than
	// from tmp directly. planFilePath resolves the relative path via filepath.Abs, which
	// joins against the OS's own notion of the current directory — os.Getwd() is the only
	// way to observe that same value. tmp's literal string can differ from what the OS
	// reports: on macOS, tmp itself is a symlink (e.g. /tmp -> /private/tmp) that getcwd()
	// resolves; on Windows CI runners, GetCurrentDirectory can report the current directory
	// in its short 8.3-alias form (e.g. "RUNNER~1") even though tmp was created with a long
	// user-profile name. Comparing against os.Getwd()'s output is robust to both.
	cwdAfterChdir, err := os.Getwd()
	require.NoError(t, err)
	want := filepath.Join(cwdAfterChdir, relativeWorkingDir, ".atmos-plan-123.tfplan")
	assert.Equal(t, want, got)
	// The doubled-path bug this guards against would produce a path with the working dir's
	// own name segments appearing twice in a row.
	assert.NotContains(t, got, filepath.Join(relativeWorkingDir, relativeWorkingDir))
}

// TestGenerateTwoPhasePlanFile verifies the apply and destroy planfile name patterns differ
// and both land in the requested working directory.
func TestGenerateTwoPhasePlanFile(t *testing.T) {
	dir := t.TempDir()

	applyFile := generateTwoPhasePlanFile(dir, false)
	destroyFile := generateTwoPhasePlanFile(dir, true)

	assert.Equal(t, dir, filepath.Dir(applyFile))
	assert.Equal(t, dir, filepath.Dir(destroyFile))
	assert.True(t, strings.HasPrefix(filepath.Base(applyFile), ".atmos-plan-"), "apply planfile: %s", applyFile)
	assert.True(t, strings.HasSuffix(applyFile, ".tfplan"))
	assert.True(t, strings.HasPrefix(filepath.Base(destroyFile), ".atmos-destroy-"), "destroy planfile: %s", destroyFile)
	assert.True(t, strings.HasSuffix(destroyFile, ".tfplan"))
}

// TestShowPlanTree_ToleratesUnparsablePlan verifies showPlanTree does not panic when the
// planfile can't be parsed (e.g. no real terraform binary available) - it should silently do
// nothing, per its documented contract.
func TestShowPlanTree_ToleratesUnparsablePlan(t *testing.T) {
	opts := &ExecuteOptions{
		Command:    filepath.Join(t.TempDir(), "nonexistent-terraform-binary"),
		WorkingDir: t.TempDir(),
		Component:  "comp",
		Stack:      "stack",
	}

	assert.NotPanics(t, func() {
		showPlanTree(context.Background(), opts, filepath.Join(opts.WorkingDir, "plan.tfplan"))
	})
}

// TestShowPlanTree_RendersTreeWhenChangesExist verifies showPlanTree renders both the
// dependency tree and the change-summary badge when the parsed plan has changes. The test
// binary stands in for `terraform show -json` (testmain_test.go's _ATMOS_TEST_TF_SHOW_JSON
// handling, also used by TestBuildDependencyTree_Success in tree_builder_test.go).
func TestShowPlanTree_RendersTreeWhenChangesExist(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err)
	stderr := captureUIOutput(t)

	planJSON := `{"format_version":"1.2","resource_changes":[` +
		`{"address":"aws_vpc.main","mode":"managed","change":{"actions":["create"]}}]}`
	t.Setenv("_ATMOS_TEST_TF_SHOW_JSON", planJSON)

	opts := &ExecuteOptions{Command: exePath, WorkingDir: t.TempDir(), Component: "vpc", Stack: "dev"}

	showPlanTree(context.Background(), opts, "plan.tfplan")

	output := stderr.String()
	assert.Contains(t, output, "aws_vpc.main", "dependency tree should be rendered")
	assert.Contains(t, output, "1 ADD", "change-summary badge should reflect the one create")
}

// TestShowPlanTree_NoChangesShowsBadgeOnly verifies showPlanTree renders only the "NO CHANGES"
// badge (no tree) when the parsed plan has no resource changes.
func TestShowPlanTree_NoChangesShowsBadgeOnly(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err)
	stderr := captureUIOutput(t)

	t.Setenv("_ATMOS_TEST_TF_SHOW_JSON", `{"format_version":"1.2","resource_changes":[]}`)

	opts := &ExecuteOptions{Command: exePath, WorkingDir: t.TempDir()}

	showPlanTree(context.Background(), opts, "plan.tfplan")

	assert.Contains(t, stderr.String(), "NO CHANGES")
}

// TestShowTwoPhasePlanTree_ReturnsFalseWhenPlanCannotBeParsed verifies an unparsable plan
// falls through to the confirmation phase (returns false) rather than being mistaken for a
// no-changes plan, matching the function's documented "prior behavior" contract.
func TestShowTwoPhasePlanTree_ReturnsFalseWhenPlanCannotBeParsed(t *testing.T) {
	opts := &ExecuteOptions{
		Command:    filepath.Join(t.TempDir(), "nonexistent-terraform-binary"),
		WorkingDir: t.TempDir(),
	}

	noChanges := showTwoPhasePlanTree(context.Background(), opts, filepath.Join(opts.WorkingDir, "plan.tfplan"))

	assert.False(t, noChanges)
}

// TestShowTwoPhasePlanTree_NoChangesReturnsTrue verifies a successfully parsed plan with zero
// changes is reported as "no changes" (true) and renders only the badge.
func TestShowTwoPhasePlanTree_NoChangesReturnsTrue(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err)
	stderr := captureUIOutput(t)

	t.Setenv("_ATMOS_TEST_TF_SHOW_JSON", `{"format_version":"1.2","resource_changes":[]}`)

	opts := &ExecuteOptions{Command: exePath, WorkingDir: t.TempDir()}

	noChanges := showTwoPhasePlanTree(context.Background(), opts, "plan.tfplan")

	assert.True(t, noChanges)
	assert.Contains(t, stderr.String(), "NO CHANGES")
}

// TestShowTwoPhasePlanTree_WithChangesReturnsFalse verifies a successfully parsed plan with
// changes is reported as "has changes" (false) and renders the dependency tree plus badge.
func TestShowTwoPhasePlanTree_WithChangesReturnsFalse(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err)
	stderr := captureUIOutput(t)

	planJSON := `{"format_version":"1.2","resource_changes":[` +
		`{"address":"aws_s3_bucket.main","mode":"managed","change":{"actions":["delete"]}}]}`
	t.Setenv("_ATMOS_TEST_TF_SHOW_JSON", planJSON)

	opts := &ExecuteOptions{Command: exePath, WorkingDir: t.TempDir()}

	noChanges := showTwoPhasePlanTree(context.Background(), opts, "plan.tfplan")

	assert.False(t, noChanges)
	output := stderr.String()
	assert.Contains(t, output, "aws_s3_bucket.main")
	assert.Contains(t, output, "1 DELETE")
}

// TestConfirmTwoPhaseOperation_PropagatesConfirmError verifies both the apply and destroy
// branches select the correct underlying confirm function and propagate its error (stdin
// isn't a TTY in a `go test` process) instead of swallowing it.
func TestConfirmTwoPhaseOperation_PropagatesConfirmError(t *testing.T) {
	tests := []struct {
		name      string
		isDestroy bool
	}{
		{"apply", false},
		{"destroy", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			confirmed, err := confirmTwoPhaseOperation(tt.isDestroy)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrStreamingNotSupported)
			assert.False(t, confirmed)
		})
	}
}

// TestApplyTwoPhasePlan_PropagatesExecuteError verifies applyTwoPhasePlan builds the apply
// options and delegates to Execute, propagating its error rather than swallowing it.
func TestApplyTwoPhasePlan_PropagatesExecuteError(t *testing.T) {
	opts := &ExecuteOptions{WorkingDir: t.TempDir()}

	err := applyTwoPhasePlan(context.Background(), opts, filepath.Join(opts.WorkingDir, "plan.tfplan"))

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrStreamingNotSupported)
}

// TestRunTwoPhasePlan_DoesNotMutateCallerArgs is an aliasing/isolation regression test:
// runTwoPhasePlan copies opts before rewriting Args for the plan phase, for both the apply
// (plain plan) and destroy (-destroy plan) branches, so the caller's original Args slice must
// be left untouched.
func TestRunTwoPhasePlan_DoesNotMutateCallerArgs(t *testing.T) {
	tests := []struct {
		name      string
		isDestroy bool
	}{
		{"plan", false},
		{"destroy", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origArgs := []string{"apply", "-auto-approve", "-var", "x=1"}
			opts := &ExecuteOptions{
				Args:       append([]string(nil), origArgs...),
				WorkingDir: t.TempDir(),
			}
			planFile := filepath.Join(opts.WorkingDir, "plan.tfplan")

			err := runTwoPhasePlan(context.Background(), opts, planFile, tt.isDestroy)

			require.Error(t, err, "fails fast: no TTY for the streaming UI in `go test`")
			assert.ErrorIs(t, err, errUtils.ErrStreamingNotSupported)
			assert.Equal(t, origArgs, opts.Args, "runTwoPhasePlan must not mutate the caller's Args slice")
		})
	}
}

// TestExecuteTwoPhaseOperation_PropagatesPlanPhaseError verifies both the apply and destroy
// two-phase flows propagate a plan-phase failure (and safely no-op the deferred planfile
// cleanup for a file that was never created).
func TestExecuteTwoPhaseOperation_PropagatesPlanPhaseError(t *testing.T) {
	tests := []struct {
		name      string
		isDestroy bool
	}{
		{"apply", false},
		{"destroy", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &ExecuteOptions{WorkingDir: t.TempDir()}
			err := executeTwoPhaseOperation(context.Background(), opts, tt.isDestroy)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrStreamingNotSupported)
		})
	}
}

// TestExecuteTwoPhaseApply_DelegatesToOperation and TestExecuteTwoPhaseDestroy_DelegatesToOperation
// verify the isDestroy wiring for the two thin wrapper functions used by ExecuteApply/ExecuteDestroy.
func TestExecuteTwoPhaseApply_DelegatesToOperation(t *testing.T) {
	opts := &ExecuteOptions{WorkingDir: t.TempDir()}
	err := executeTwoPhaseApply(context.Background(), opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrStreamingNotSupported)
}

func TestExecuteTwoPhaseDestroy_DelegatesToOperation(t *testing.T) {
	opts := &ExecuteOptions{WorkingDir: t.TempDir()}
	err := executeTwoPhaseDestroy(context.Background(), opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrStreamingNotSupported)
}

// TestExecuteWithPlanFile_PropagatesConfirmError verifies executeWithPlanFile tolerates an
// unparsable planfile (skips the tree display) and then propagates the confirmation error.
func TestExecuteWithPlanFile_PropagatesConfirmError(t *testing.T) {
	opts := &ExecuteOptions{
		Command:    filepath.Join(t.TempDir(), "nonexistent-terraform-binary"),
		WorkingDir: t.TempDir(),
	}

	err := executeWithPlanFile(context.Background(), opts, filepath.Join(opts.WorkingDir, "plan.tfplan"))

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrStreamingNotSupported)
}

// TestExecuteWithPlanFile_WithChangesRendersTreeThenPropagatesConfirmError verifies that when
// the planfile parses successfully and has changes, executeWithPlanFile renders the tree
// before falling through to the (TTY-gated, thus failing) confirmation step.
func TestExecuteWithPlanFile_WithChangesRendersTreeThenPropagatesConfirmError(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err)
	stderr := captureUIOutput(t)

	planJSON := `{"format_version":"1.2","resource_changes":[` +
		`{"address":"aws_vpc.main","mode":"managed","change":{"actions":["create"]}}]}`
	t.Setenv("_ATMOS_TEST_TF_SHOW_JSON", planJSON)

	opts := &ExecuteOptions{Command: exePath, WorkingDir: t.TempDir()}

	runErr := executeWithPlanFile(context.Background(), opts, "plan.tfplan")

	require.Error(t, runErr)
	assert.ErrorIs(t, runErr, errUtils.ErrStreamingNotSupported)
	assert.Contains(t, stderr.String(), "aws_vpc.main")
}

// TestExecuteWithPlanFile_NoChangesDisplaysOutputsAndReturnsNil verifies that when the
// planfile parses successfully with zero changes, executeWithPlanFile shows the "no changes"
// badge, attempts to display current outputs, and returns nil without ever calling Execute
// (i.e. without needing the streaming UI's TTY precondition to hold).
func TestExecuteWithPlanFile_NoChangesDisplaysOutputsAndReturnsNil(t *testing.T) {
	exePath, err := os.Executable()
	require.NoError(t, err)
	stderr := captureUIOutput(t)

	// Reused for both the "terraform show -json" (tree parse) and "terraform output -json"
	// (fetchAndDisplayOutputs) subprocess calls - both hit the fake-subprocess env gate in
	// testmain_test.go regardless of which args are passed. A minimal valid tfjson.Plan (no
	// resource_changes) satisfies the tree parser; it also unmarshals into an empty
	// map[string]OutputValue for fetchAndDisplayOutputs, since format_version is simply an
	// unused extra key from that call's point of view.
	const emptyPlanJSON = `{"format_version":"1.2"}`
	t.Setenv("_ATMOS_TEST_TF_SHOW_JSON", emptyPlanJSON)

	opts := &ExecuteOptions{
		Command:    exePath,
		WorkingDir: t.TempDir(),
		Env:        append(os.Environ(), "_ATMOS_TEST_TF_SHOW_JSON="+emptyPlanJSON),
	}

	runErr := executeWithPlanFile(context.Background(), opts, "plan.tfplan")

	require.NoError(t, runErr)
	assert.Contains(t, stderr.String(), "NO CHANGES")
}

// TestExecute_FailsFastWhenNotTTY verifies Execute checks streaming UI preconditions (no TTY
// in a `go test` process) before doing any real work.
func TestExecute_FailsFastWhenNotTTY(t *testing.T) {
	opts := &ExecuteOptions{Command: "terraform", WorkingDir: t.TempDir()}

	err := Execute(context.Background(), opts)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrStreamingNotSupported)
}

// TestExecuteInit_FailsFastWhenNotTTY mirrors TestExecute_FailsFastWhenNotTTY for ExecuteInit.
func TestExecuteInit_FailsFastWhenNotTTY(t *testing.T) {
	opts := &ExecuteOptions{Command: "terraform", WorkingDir: t.TempDir(), SubCommand: subCommandInit}

	err := ExecuteInit(context.Background(), opts)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrStreamingNotSupported)
}

// TestExecutePlan_PropagatesError covers both ExecutePlan branches - a caller-provided -out
// planfile and the generated-temp-file default - verifying each wires through to Execute
// (which fails fast with no TTY) instead of silently doing nothing.
func TestExecutePlan_PropagatesError(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"user-provided -out planfile", []string{"plan", "-out=myplan.tfplan"}},
		{"generated temp planfile", []string{"plan"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := &ExecuteOptions{Command: "terraform", Args: tt.args, WorkingDir: t.TempDir()}
			err := ExecutePlan(context.Background(), opts)
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrStreamingNotSupported)
		})
	}
}

// TestExecuteApply_AutoApprove_PropagatesExecuteError is a regression test: prior coverage
// only exercised the confirmation-required path (see TestExecuteApply_FailsFastWhenStdinNotTTY);
// this covers the -auto-approve branch, which skips confirmation and calls Execute directly.
func TestExecuteApply_AutoApprove_PropagatesExecuteError(t *testing.T) {
	opts := &ExecuteOptions{
		Command:    "terraform",
		Args:       []string{"apply", "-auto-approve"},
		WorkingDir: t.TempDir(),
	}

	err := ExecuteApply(context.Background(), opts)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrStreamingNotSupported)
}

// TestExecuteDestroy_AutoApprove_PropagatesExecuteError mirrors
// TestExecuteApply_AutoApprove_PropagatesExecuteError for ExecuteDestroy.
func TestExecuteDestroy_AutoApprove_PropagatesExecuteError(t *testing.T) {
	opts := &ExecuteOptions{
		Command:    "terraform",
		Args:       []string{"destroy", "-auto-approve"},
		WorkingDir: t.TempDir(),
	}

	err := ExecuteDestroy(context.Background(), opts)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrStreamingNotSupported)
}
