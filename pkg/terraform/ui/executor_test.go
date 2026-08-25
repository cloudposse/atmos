package ui

import (
	"context"
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
