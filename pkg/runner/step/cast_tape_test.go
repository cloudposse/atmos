package step

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

// captureTapeLogOutput redirects the package logger to a buffer for the
// duration of fn and returns everything logged, restoring stderr afterward.
func captureTapeLogOutput(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	fn()
	return buf.String()
}

func TestExpandCastTapeNoopWithoutTapeFields(t *testing.T) {
	step := &schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Steps: []schema.WorkflowStep{{Type: "write", Text: "x"}}}
	if err := expandCastTape(step); err != nil {
		t.Fatalf("expandCastTape error: %v", err)
	}
	if len(step.Steps) != 1 || step.Steps[0].Text != "x" {
		t.Fatalf("expected hand-authored steps to be left untouched, got %+v", step.Steps)
	}
}

func TestExpandCastTapeRejectsBothTapeAndTapeFile(t *testing.T) {
	step := &schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Tape: "Sleep 1s", TapeFile: "x.tape"}
	err := expandCastTape(step)
	if !errors.Is(err, errUtils.ErrCastTapeSourceConflict) {
		t.Fatalf("expected ErrCastTapeSourceConflict, got %v", err)
	}
}

func TestExpandCastTapeRejectsTapeWithExplicitSteps(t *testing.T) {
	step := &schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Tape: "Sleep 1s",
		Steps: []schema.WorkflowStep{{Type: "write", Text: "x"}},
	}
	err := expandCastTape(step)
	if !errors.Is(err, errUtils.ErrCastTapeStepsConflict) {
		t.Fatalf("expected ErrCastTapeStepsConflict, got %v", err)
	}
}

func TestExpandCastTapeSessionModeTranslatesCommonDirectives(t *testing.T) {
	step := &schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "session",
		Tape: `Type "atmos list stacks" Sleep 600ms Enter
Wait /prompt$/
Down 2
Ctrl+C
Screenshot out.png`,
	}
	if err := expandCastTape(step); err != nil {
		t.Fatalf("expandCastTape error: %v", err)
	}
	want := []schema.WorkflowStep{
		{Type: "write", Text: "atmos list stacks"},
		{Type: "pause", Duration: "600ms"},
		{Type: "key", Key: "enter"},
		{Type: "wait", Regex: "prompt$"},
		{Type: "key", Key: "down", Repeat: 2},
		{Type: "key", Key: "ctrl+c"},
		{Type: schema.TaskTypeScreenshot, Path: "out.png"},
	}
	assertWorkflowStepsEqual(t, want, step.Steps)
}

func TestExpandCastTapeSessionModeBareWaitErrors(t *testing.T) {
	step := &schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "session", Tape: "Wait"}
	err := expandCastTape(step)
	if !errors.Is(err, errUtils.ErrUnsupportedTapeDirective) {
		t.Fatalf("expected ErrUnsupportedTapeDirective for a bare Wait, got %v", err)
	}
}

func TestExpandCastTapeSessionModeHideShowLiftsRecognizedSetupCommands(t *testing.T) {
	step := &schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "session",
		Tape: `Hide
Type "export FOO=bar" Enter
Type "unset NO_COLOR" Enter
Type "cd /tmp/demo" Enter
Type "clear" Enter
Show
Type "echo hi" Enter`,
	}
	if err := expandCastTape(step); err != nil {
		t.Fatalf("expandCastTape error: %v", err)
	}
	// All four hidden commands matched a recognized setup pattern, so nothing
	// was buffered -- no hide/show pair is emitted at all, since there was
	// nothing to mute in the first place.
	want := []schema.WorkflowStep{
		{Type: "write", Text: "echo hi"},
		{Type: "key", Key: "enter"},
	}
	assertWorkflowStepsEqual(t, want, step.Steps)
	if step.Env["FOO"] != "bar" {
		t.Fatalf("expected export to be lifted into step.Env, got %+v", step.Env)
	}
	if v, ok := step.Env["NO_COLOR"]; !ok || v != "" {
		t.Fatalf("expected unset to be lifted into step.Env as empty string, got %+v", step.Env)
	}
	if step.WorkingDirectory != "/tmp/demo" {
		t.Fatalf("expected cd to be lifted into step.WorkingDirectory, got %q", step.WorkingDirectory)
	}
}

func TestExpandCastTapeSessionModeHideFallsBackToLiteralReplayForUnrecognizedCommands(t *testing.T) {
	step := &schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "session",
		Tape: `Hide
Type "docker compose up -d" Enter
Show`,
	}
	if err := expandCastTape(step); err != nil {
		t.Fatalf("expandCastTape error: %v", err)
	}
	want := []schema.WorkflowStep{
		{Type: "hide"},
		{Type: "write", Text: "docker compose up -d"},
		{Type: "key", Key: "enter"},
		{Type: "show"},
	}
	assertWorkflowStepsEqual(t, want, step.Steps)
}

func TestExpandCastTapeStepsModeTranslatesTypeToSimulateAndShellPair(t *testing.T) {
	step := &schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "steps",
		Tape: `Type "atmos list stacks" Enter
Sleep 1s
Wait /prompt$/`,
	}
	if err := expandCastTape(step); err != nil {
		t.Fatalf("expandCastTape error: %v", err)
	}
	want := []schema.WorkflowStep{
		{Type: schema.TaskTypeSimulate, Mode: "typed", Text: "atmos list stacks"},
		{Type: schema.TaskTypeShell, Command: "atmos list stacks"},
	}
	assertWorkflowStepsEqual(t, want, step.Steps)
}

func TestExpandCastTapeStepsModeTracksWorkingDirectoryAndEnvAcrossCommands(t *testing.T) {
	// This is the bug the restored legacy demo.tape fixture exposed: a
	// *visible* `cd` (not inside Hide) must still affect every subsequent
	// real shell child, even though each runs as an independent subprocess.
	step := &schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "steps",
		Tape: `Type "cd examples/quick-start" Enter
Type "export GREETING=hi" Enter
Type "ls -al" Enter
Type "cd nested" Enter
Type "pwd" Enter`,
	}
	if err := expandCastTape(step); err != nil {
		t.Fatalf("expandCastTape error: %v", err)
	}

	var shellChildren []schema.WorkflowStep
	for _, child := range step.Steps {
		if child.Type == schema.TaskTypeShell {
			shellChildren = append(shellChildren, child)
		}
	}
	if len(shellChildren) != 2 {
		t.Fatalf("expected 2 real shell children (cd/export produce none), got %d: %+v", len(shellChildren), shellChildren)
	}
	if shellChildren[0].Command != "ls -al" || shellChildren[0].WorkingDirectory != "examples/quick-start" {
		t.Fatalf("unexpected first shell child: %+v", shellChildren[0])
	}
	if shellChildren[0].Env["GREETING"] != "hi" {
		t.Fatalf("expected first shell child to see the export, got %+v", shellChildren[0].Env)
	}
	if shellChildren[1].Command != "pwd" || shellChildren[1].WorkingDirectory != "examples/quick-start/nested" {
		t.Fatalf("unexpected second shell child (relative cd should compose with the tracked cwd): %+v", shellChildren[1])
	}
}

func TestExpandCastTapeStepsModeDropsClearAndItsSimulateNarration(t *testing.T) {
	step := &schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "steps",
		Tape: `Type "clear" Enter
Type "atmos version" Enter`,
	}
	if err := expandCastTape(step); err != nil {
		t.Fatalf("expandCastTape error: %v", err)
	}
	want := []schema.WorkflowStep{
		{Type: schema.TaskTypeSimulate, Mode: "typed", Text: "atmos version"},
		{Type: schema.TaskTypeShell, Command: "atmos version"},
	}
	assertWorkflowStepsEqual(t, want, step.Steps)
}

func TestExpandCastTapeStepsModeScreenshotSupported(t *testing.T) {
	step := &schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "steps", Tape: "Screenshot out.png"}
	if err := expandCastTape(step); err != nil {
		t.Fatalf("expandCastTape error: %v", err)
	}
	want := []schema.WorkflowStep{{Type: schema.TaskTypeScreenshot, Path: "out.png"}}
	assertWorkflowStepsEqual(t, want, step.Steps)
}

func TestExpandCastTapeStepsModeErrorsOnSessionOnlyDirectives(t *testing.T) {
	tests := []struct {
		name string
		tape string
	}{
		{name: "bare keypress", tape: "Down"},
		{name: "Hide", tape: "Hide"},
		{name: "Show", tape: "Show"},
		{name: "Type without trailing Enter", tape: `Type "q"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			step := &schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "steps", Tape: tt.tape}
			err := expandCastTape(step)
			if !errors.Is(err, errUtils.ErrTapeDirectiveRequiresSessionMode) {
				t.Fatalf("expected ErrTapeDirectiveRequiresSessionMode, got %v", err)
			}
		})
	}
}

func TestExpandCastTapeCopyPasteEnvAlwaysError(t *testing.T) {
	tests := []string{`Copy "text"`, "Paste", "Env KEY value"}
	for _, tape := range tests {
		for _, mode := range []string{"session", "steps"} {
			step := &schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: mode, Tape: tape}
			err := expandCastTape(step)
			if !errors.Is(err, errUtils.ErrUnsupportedTapeDirective) {
				t.Fatalf("mode %s, tape %q: expected ErrUnsupportedTapeDirective, got %v", mode, tape, err)
			}
		}
	}
}

func TestExpandCastTapeAppliesSetPrecedenceExplicitFieldWins(t *testing.T) {
	step := &schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "session",
		Width: 100, // explicit -- must survive the tape's Set Width.
		Tape:  "Set Width 1280\nSet Height 640",
	}
	if err := expandCastTape(step); err != nil {
		t.Fatalf("expandCastTape error: %v", err)
	}
	if step.Width != 100 {
		t.Fatalf("explicit step.Width was overridden: got %d, want 100", step.Width)
	}
	if step.Height != 640 {
		t.Fatalf("expected tape Set Height to fill in the unset field, got %d", step.Height)
	}
}

func TestExpandCastTapeOutputExtensionValidation(t *testing.T) {
	step := &schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "session", Tape: "Output demo.webm"}
	err := expandCastTape(step)
	if !errors.Is(err, errUtils.ErrUnsupportedTapeOutputExtension) {
		t.Fatalf("expected ErrUnsupportedTapeOutputExtension, got %v", err)
	}
}

func TestExpandCastTapeOutputExtensionAccepted(t *testing.T) {
	step := &schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "session", Tape: "Output demo.mp4"}
	if err := expandCastTape(step); err != nil {
		t.Fatalf("expandCastTape error: %v", err)
	}
	if step.CastOutput == nil || step.CastOutput.MP4 != "demo.mp4" {
		t.Fatalf("expected CastOutput.MP4 to be set, got %+v", step.CastOutput)
	}
}

func TestExpandCastTapeRequireSynthesizesTools(t *testing.T) {
	step := &schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "session", Tape: "Require atmos\nRequire atmos"}
	if err := expandCastTape(step); err != nil {
		t.Fatalf("expandCastTape error: %v", err)
	}
	if len(step.Tools) != 1 || step.Tools[0] != "atmos" {
		t.Fatalf("expected deduplicated Tools=[atmos], got %+v", step.Tools)
	}
}

// TestCastHandlerRequireStepRunsThroughRealRequireHandler proves the
// black-box reuse: a tape's Require directive is checked via the same real
// type: require step (and its hint text) that a hand-authored precondition
// step would use, not a reimplementation.
func TestCastHandlerRequireStepRunsThroughRealRequireHandler(t *testing.T) {
	step := &schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "session",
		Tape: "Require definitely-not-a-real-tool-xyz\nSleep 1ms",
	}
	_, err := NewStepExecutorWithVars(NewVariables()).Execute(context.Background(), step)
	if err == nil {
		t.Fatal("expected the synthesized require step to fail for a missing tool")
	}
	if !errors.Is(err, errUtils.ErrRequirementsNotMet) {
		t.Fatalf("expected the real RequireHandler's ErrRequirementsNotMet, got %v", err)
	}
}

// TestHeroTapeInterpretation walks a trimmed inline reproduction of the real
// demo/landing/hero.tape end to end through Validate, mirroring the plan's
// hero.tape trace-through: Source-inlined Set defaults, a Hide block whose
// four setup commands all lift into step.Env/WorkingDirectory (so no hide/
// show/write/key setup actions appear at all), two typed atmos commands each
// followed by a Wait, and a trailing Screenshot.
func TestHeroTapeInterpretation(t *testing.T) {
	step := &schema.WorkflowStep{
		Name: "hero-demo", Type: schema.TaskTypeCast, Mode: "session",
		Tape: `Set Shell bash
Set TypingSpeed 35ms
Set Width 1280
Set Height 640
Set WaitTimeout 300s
Output website/static/img/demos/hero.mp4
Hide
Type "unset NO_COLOR ATMOS_NO_COLOR" Enter
Type "export ATMOS_FORCE_COLOR=true" Enter
Type "cd /repo/demo/landing/fixtures/hero" Enter
Type "clear" Enter
Wait /❯$/
Show
Sleep 800ms
Type "atmos list stacks" Sleep 600ms Enter
Wait /❯$/
Sleep 1.5s
Type "atmos list components -s dev" Sleep 600ms Enter
Wait /❯$/
Sleep 3s
Screenshot website/static/img/demos/hero.png
Sleep 5s`,
	}
	if err := expandCastTape(step); err != nil {
		t.Fatalf("expandCastTape error: %v", err)
	}

	if step.Shell != "bash" || step.WriteRate != "35ms" || step.Width != 1280 || step.Height != 640 || step.Timeout != "300s" {
		t.Fatalf("Set directives not applied: shell=%q writeRate=%q width=%d height=%d timeout=%q",
			step.Shell, step.WriteRate, step.Width, step.Height, step.Timeout)
	}
	if step.CastOutput == nil || step.CastOutput.MP4 != "website/static/img/demos/hero.mp4" {
		t.Fatalf("Output directive not applied: %+v", step.CastOutput)
	}
	if step.Env["NO_COLOR"] != "" || step.Env["ATMOS_NO_COLOR"] != "" || step.Env["ATMOS_FORCE_COLOR"] != "true" {
		t.Fatalf("Hide block env not lifted: %+v", step.Env)
	}
	if step.WorkingDirectory != "/repo/demo/landing/fixtures/hero" {
		t.Fatalf("Hide block cd not lifted: %q", step.WorkingDirectory)
	}

	want := []schema.WorkflowStep{
		{Type: "wait", Regex: "❯$", Timeout: "300s"},
		{Type: "pause", Duration: "800ms"},
		{Type: "write", Text: "atmos list stacks"},
		{Type: "pause", Duration: "600ms"},
		{Type: "key", Key: "enter"},
		{Type: "wait", Regex: "❯$", Timeout: "300s"},
		{Type: "pause", Duration: "1.5s"},
		{Type: "write", Text: "atmos list components -s dev"},
		{Type: "pause", Duration: "600ms"},
		{Type: "key", Key: "enter"},
		{Type: "wait", Regex: "❯$", Timeout: "300s"},
		{Type: "pause", Duration: "3s"},
		{Type: schema.TaskTypeScreenshot, Path: "website/static/img/demos/hero.png"},
		{Type: "pause", Duration: "5s"},
	}
	// The Hide block's four setup commands all matched recognized patterns, so
	// no "hide"/"show"/"write"/"key" actions appear for them at all -- unlike
	// the literal-replay fallback case, there's no `hide` action here either,
	// since Hide/Show themselves are only emitted when at least one contained
	// command needed literal replay. hero.tape's real Wait comes right after
	// the lifted setup, before Show, which is why the first want entry is the
	// wait, not a hide/show pair.
	assertWorkflowStepsEqual(t, want, step.Steps)
}

func TestExpandCastTapeWarnsOnCosmeticSetDirectiveSkipped(t *testing.T) {
	step := &schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "session", Tape: "Set Theme dark\nSleep 1ms"}
	out := captureTapeLogOutput(t, func() {
		if err := expandCastTape(step); err != nil {
			t.Fatalf("expandCastTape error: %v", err)
		}
	})
	if !strings.Contains(out, "not supported") || !strings.Contains(out, "Theme") {
		t.Fatalf("expected a warning naming the skipped Set key, got: %s", out)
	}
}

func TestExpandCastTapeWarnsOnSleepAndWaitDroppedInStepsMode(t *testing.T) {
	step := &schema.WorkflowStep{
		Name: "demo", Type: schema.TaskTypeCast, Mode: "steps",
		Tape: "Type \"atmos version\" Enter\nSleep 1s\nWait /prompt$/",
	}
	out := captureTapeLogOutput(t, func() {
		if err := expandCastTape(step); err != nil {
			t.Fatalf("expandCastTape error: %v", err)
		}
	})
	if !strings.Contains(out, "Sleep") || !strings.Contains(out, "no effect in mode: steps") {
		t.Fatalf("expected a warning naming the dropped Sleep directive, got: %s", out)
	}
	if !strings.Contains(out, "Wait") {
		t.Fatalf("expected a warning naming the dropped Wait directive, got: %s", out)
	}
}

func assertWorkflowStepsEqual(t *testing.T, want, got []schema.WorkflowStep) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("step count = %d, want %d\ngot:  %+v\nwant: %+v", len(got), len(want), got, want)
	}
	for i := range want {
		w, g := want[i], got[i]
		if w.Type != g.Type || w.Text != g.Text || w.Key != g.Key || w.Duration != g.Duration ||
			w.Regex != g.Regex || w.Repeat != g.Repeat || w.Path != g.Path || w.Mode != g.Mode || w.Command != g.Command {
			t.Fatalf("step %d = %+v, want %+v", i, g, w)
		}
	}
}
