package step

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/asciicast"
	"github.com/cloudposse/atmos/pkg/automation"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

// tapeReviewShell uses shell builtins through the Go shell interpreter, without a platform binary.
func tapeReviewShell(t *testing.T, env []string) (*interp.Runner, *bytes.Buffer) {
	t.Helper()
	var output bytes.Buffer
	runner, err := interp.New(interp.Env(expand.ListEnviron(env...)), interp.StdIO(nil, &output, &output))
	require.NoError(t, err)
	return runner, &output
}

func runTapeReviewCommand(t *testing.T, runner *interp.Runner, command string) error {
	t.Helper()
	program, err := syntax.NewParser().Parse(strings.NewReader(command), "")
	require.NoError(t, err)
	return runner.Run(t.Context(), program)
}

func TestTapeUnsetRemovesInheritedEnvironment(t *testing.T) {
	for _, source := range []string{"process", "workflow"} {
		for _, mode := range []string{"steps", "session"} {
			t.Run(source+"/"+mode, func(t *testing.T) {
				t.Setenv("NO_COLOR", "inherited")
				if source == "workflow" {
					require.NoError(t, os.Unsetenv("NO_COLOR"))
				}
				vars := NewVariables()
				if source == "workflow" {
					vars.SetEnv("NO_COLOR", "inherited")
				}
				// Confirm that the actual shell sees the inherited value before unsetting it.
				probe, output := tapeReviewShell(t, vars.EnvSlice())
				require.NoError(t, runTapeReviewCommand(t, probe, `printf '%s' "${NO_COLOR+present}"`))
				require.Equal(t, "present", output.String())
				step := &schema.WorkflowStep{
					Name: "demo", Mode: mode,
					Tape: "Type \"unset NO_COLOR\" Enter\nType `printf 'result=%s' \"${NO_COLOR+present}\"` Enter",
				}
				if mode == "session" {
					step.Tape = "Hide\nType \"unset NO_COLOR\" Enter\nShow\nType `printf 'result=%s' \"${NO_COLOR+present}\"` Enter"
				}
				require.NoError(t, expandCastTape(step))
				assertTapeUnsetExecution(t, step, vars)
				assert.Equal(t, "inherited", vars.Env["NO_COLOR"], "parent environment must remain unchanged")
			})
		}
	}
}

func assertTapeUnsetExecution(t *testing.T, step *schema.WorkflowStep, vars *Variables) {
	t.Helper()
	if step.Mode == "session" {
		env, err := castRecorderEnv(step, vars)
		require.NoError(t, err)
		local := vars.Clone()
		local.Env = env
		runner, output := tapeReviewShell(t, local.EnvSlice())
		for i := range step.Steps {
			action := &step.Steps[i]
			if action.Type == "write" {
				require.NoError(t, runTapeReviewCommand(t, runner, action.Text))
			}
		}
		assert.Equal(t, "result=", output.String())
		return
	}
	require.Len(t, step.Steps, 3)
	assert.Equal(t, `printf 'result=%s' "${NO_COLOR+present}"`, step.Steps[1].Text)
	shell := &step.Steps[2]
	assert.True(t, shell.IsLiteral("command"))
	env, err := (&ShellHandler{}).resolveEnv(shell, vars)
	require.NoError(t, err)
	runner, output := tapeReviewShell(t, env)
	require.NoError(t, runTapeReviewCommand(t, runner, shell.Command))
	assert.Equal(t, "result=", output.String())
}

func TestTapeStepsUnsetExpansionAndReexport(t *testing.T) {
	t.Setenv("NO_COLOR", "inherited")
	step := &schema.WorkflowStep{Name: "demo", Mode: "steps", Tape: `Type "unset NO_COLOR" Enter
Type 'export COPY=${NO_COLOR-absent}' Enter
Type 'printf "%s" "$COPY"' Enter
Type "export NO_COLOR=restored" Enter
Type 'printf "%s" "$NO_COLOR"' Enter`}
	require.NoError(t, expandCastTape(step))
	require.Len(t, step.Steps, 7)
	assert.Equal(t, "absent", step.Steps[3].Env["COPY"])
	assert.Equal(t, "restored", step.Steps[6].Env["NO_COLOR"])
	assert.NotContains(t, step.Steps[6].Command, "unset", "a later export cancels the earlier unset")
	for _, tc := range []struct {
		index int
		want  string
	}{{3, "absent"}, {6, "restored"}} {
		env, err := (&ShellHandler{}).resolveEnv(&step.Steps[tc.index], NewVariables())
		require.NoError(t, err)
		runner, output := tapeReviewShell(t, env)
		require.NoError(t, runTapeReviewCommand(t, runner, step.Steps[tc.index].Command))
		assert.Equal(t, tc.want, output.String())
	}
	state := newTapeTrackedState(&schema.WorkflowStep{})
	state.apply(tapeSetupCommand{Kind: tapeSetupUnset, Env: map[string]string{"NO_COLOR": ""}})
	runner, _ := tapeReviewShell(t, NewVariables().EnvSlice())
	err := runTapeReviewCommand(t, runner, state.shellCommand("exit 7"))
	var status interp.ExitStatus
	require.True(t, errors.As(err, &status))
	assert.Equal(t, interp.ExitStatus(7), status, "unset prelude preserves the command exit status")
}

func TestTapeMixedOutputsProtectHandWrittenTarget(t *testing.T) {
	for _, tapeFormat := range []string{"html", "ascii"} {
		t.Run(tapeFormat, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "in.cast")
			require.NoError(t, os.WriteFile(input, []byte("{\"version\":2,\"width\":20,\"height\":3}\n[0.1,\"o\",\"fresh\"]\n"), 0o600))
			html, ascii := filepath.Join(dir, "out.html"), filepath.Join(dir, "out.ascii")
			require.NoError(t, os.WriteFile(html, []byte("old html"), 0o600))
			require.NoError(t, os.WriteFile(ascii, []byte("old ascii"), 0o600))
			step := &schema.WorkflowStep{CastOutput: &schema.CastOutput{}}
			seeded, protected := html, ascii
			step.CastOutput.ASCII = ascii
			if tapeFormat == "ascii" {
				seeded, protected = ascii, html
				step.CastOutput = &schema.CastOutput{HTML: html}
			}
			require.NoError(t, applyTapeOutputDirective(step, &asciicast.TapeDirective{Value: seeded}))
			require.ErrorIs(t, renderCastOutputs(step, input), errUtils.ErrRenderOutputExists)
			for path, want := range map[string]string{html: "old html", ascii: "old ascii"} {
				got, err := os.ReadFile(path)
				require.NoError(t, err)
				assert.Equal(t, want, string(got), "all targets must pass preflight before any replacement")
			}
			require.NoError(t, os.Remove(protected))
			require.NoError(t, renderCastOutputs(step, input))
			got, err := os.ReadFile(seeded)
			require.NoError(t, err)
			assert.Contains(t, string(got), "fresh")
		})
	}
}

func TestCastTapeRepeatedValidationAndAutomation(t *testing.T) {
	for _, file := range []bool{false, true} {
		name := "inline"
		if file {
			name = "file"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			step := &schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "steps", Tape: `Type "export READY=yes" Enter`}
			config := map[string]any{"mode": "steps", "tape": step.Tape, "output": map[string]any{"cast": filepath.Join(dir, "out.cast")}}
			if file {
				path := filepath.Join(dir, "input.tape")
				require.NoError(t, os.WriteFile(path, []byte(step.Tape), 0o600))
				step.Tape, step.TapeFile = "", path
				delete(config, "tape")
				config["tape_file"] = path
			}
			handler := &CastHandler{}
			require.NoError(t, handler.Validate(step))
			before, err := json.Marshal(step)
			require.NoError(t, err)
			require.NoError(t, handler.Validate(step))
			after, err := json.Marshal(step)
			require.NoError(t, err)
			assert.Equal(t, string(before), string(after))
			require.True(t, step.TapeExpanded)
			assert.NotContains(t, string(after), "TapeExpanded")
			encoded, err := yaml.Marshal(step)
			require.NoError(t, err)
			assert.NotContains(t, string(encoded), "tapeexpanded")
			require.NoError(t, iolib.Initialize())
			library := NewAutomationLibrary(NewVariables(), nil)
			_, err = library.Run(context.Background(), &automation.StepCall{Type: "cast", Configuration: config})
			require.NoError(t, err, "library and executor validate the same step")
			assert.FileExists(t, filepath.Join(dir, "out.cast"))
		})
	}
}

func TestFailedTapeExpansionDoesNotSkipLaterValidation(t *testing.T) {
	step := &schema.WorkflowStep{Mode: "steps", Tape: "Down"}
	for range 2 {
		require.ErrorIs(t, (&CastHandler{}).Validate(step), ErrTapeDirectiveRequiresSessionMode)
		assert.False(t, step.TapeExpanded)
	}
	step.Tape = `Type "export READY=yes" Enter`
	require.NoError(t, (&CastHandler{}).Validate(step))
	conflict := &schema.WorkflowStep{Tape: step.Tape, Steps: []schema.WorkflowStep{{Type: "clear"}}}
	require.ErrorIs(t, (&CastHandler{}).Validate(conflict), ErrCastTapeStepsConflict)
	conflict.Steps = nil
	conflict.TapeFile = "other.tape"
	require.ErrorIs(t, (&CastHandler{}).Validate(conflict), ErrCastTapeSourceConflict)
}

func TestTapeOutputJPEGOverwriteAliases(t *testing.T) {
	for _, ext := range []string{"jpg", "jpeg"} {
		t.Run(ext, func(t *testing.T) {
			dir := t.TempDir()
			input, output := filepath.Join(dir, "in.cast"), filepath.Join(dir, "out."+ext)
			require.NoError(t, os.WriteFile(input, []byte("{\"version\":2,\"width\":20,\"height\":3}\n[0.1,\"o\",\"fresh\"]\n"), 0o600))
			require.NoError(t, os.WriteFile(output, []byte("stale"), 0o600))
			step := &schema.WorkflowStep{}
			require.NoError(t, applyTapeOutputDirective(step, &asciicast.TapeDirective{Value: output}))
			assert.Equal(t, map[string]bool{"jpeg": true}, step.CastOutput.TapeOutputFormats)
			require.NoError(t, renderCastOutputs(step, input))
			data, err := os.ReadFile(output)
			require.NoError(t, err)
			require.Greater(t, len(data), 2)
			assert.Equal(t, []byte{0xff, 0xd8}, data[:2], "both aliases replace the old file with JPEG data")
		})
	}
}

func TestTapeInternalStateDoesNotSerialize(t *testing.T) {
	type state struct {
		Step   schema.WorkflowStep
		Output schema.CastOutput
	}
	original := state{
		Step:   schema.WorkflowStep{TapeExpanded: true},
		Output: schema.CastOutput{HTML: "demo.html", TapeOutputFormats: map[string]bool{"html": true}},
	}
	jsonData, err := json.Marshal(original)
	require.NoError(t, err)
	yamlData, err := yaml.Marshal(original)
	require.NoError(t, err)
	for _, tc := range []struct {
		name      string
		data      []byte
		unmarshal func([]byte, any) error
	}{{"JSON", jsonData, json.Unmarshal}, {"YAML", yamlData, yaml.Unmarshal}} {
		t.Run(tc.name, func(t *testing.T) {
			var decoded state
			require.NoError(t, tc.unmarshal(tc.data, &decoded))
			assert.False(t, decoded.Step.TapeExpanded)
			assert.Nil(t, decoded.Output.TapeOutputFormats)
			assert.Equal(t, "demo.html", decoded.Output.HTML)
		})
	}
}
