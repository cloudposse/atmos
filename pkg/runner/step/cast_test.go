package step

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/muesli/termenv"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/ansi"
	"github.com/cloudposse/atmos/pkg/asciicast"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestCastValidateSessionWaitRequiresTextOrRegex(t *testing.T) {
	h := &CastHandler{}
	err := h.Validate(&schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		Mode: "session",
		Steps: []schema.WorkflowStep{{
			Type: "wait",
		}},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestCastValidateSessionWaitRegex(t *testing.T) {
	h := &CastHandler{}
	err := h.Validate(&schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		Mode: "session",
		Steps: []schema.WorkflowStep{{
			Type:  "wait",
			Regex: "Deployment (successful|complete)",
		}},
	})
	if err != nil {
		t.Fatalf("expected valid regex wait, got %v", err)
	}
}

func TestCastValidateStepsSimulateRejectsInvalidPromptStyle(t *testing.T) {
	h := &CastHandler{}
	err := h.Validate(&schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		Mode: "steps",
		Steps: []schema.WorkflowStep{{
			Type: schema.TaskTypeSimulate,
			Mode: "typed",
			Text: "atmos version",
			SimulatePrompt: &schema.SimulatePrompt{
				Text:  "> ",
				Style: "purple",
			},
		}},
	})
	if !errors.Is(err, ErrUnsupportedPromptStyle) {
		t.Fatalf("expected unsupported prompt style error, got %v", err)
	}
}

func TestCastValidateStepsSimulateTypedRequiresText(t *testing.T) {
	h := &CastHandler{}
	err := h.Validate(&schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		Mode: "steps",
		Steps: []schema.WorkflowStep{{
			Type: schema.TaskTypeSimulate,
			Mode: "typed",
		}},
	})
	if !errors.Is(err, ErrSimulateTypedRequiresText) {
		t.Fatalf("expected typed text error, got %v", err)
	}
}

func TestCastValidateStepsRejectsCastLevelJitter(t *testing.T) {
	h := &CastHandler{}
	err := h.Validate(&schema.WorkflowStep{
		Name:   "demo",
		Type:   schema.TaskTypeCast,
		Mode:   "steps",
		Jitter: 1.5,
		Steps: []schema.WorkflowStep{{
			Type: schema.TaskTypeSimulate,
			Mode: "prompt",
		}},
	})
	if !errors.Is(err, ErrInvalidSimulateJitter) {
		t.Fatalf("expected jitter validation error, got %v", err)
	}
}

func TestCastValidateStepsModeAcceptsValidStep(t *testing.T) {
	h := &CastHandler{}
	err := h.Validate(&schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		Mode: "steps",
		Steps: []schema.WorkflowStep{
			{Type: schema.TaskTypeShell, Command: "echo hi"},
			{Type: schema.TaskTypeSimulate, Mode: "prompt"},
		},
	})
	if err != nil {
		t.Fatalf("expected valid steps-mode cast, got %v", err)
	}
}

func TestValidateWaitActionAcceptsValidTimeout(t *testing.T) {
	if err := validateWaitAction(&schema.WorkflowStep{Text: "ready", Timeout: "5ms"}); err != nil {
		t.Fatalf("expected valid wait action, got %v", err)
	}
}

func TestCastRecorderUsesStepCommandInHeader(t *testing.T) {
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	rec, restore, err := startStepRecorder(&schema.WorkflowStep{
		Name:    "terraform-deploy",
		Type:    schema.TaskTypeCast,
		Command: "atmos terraform deploy vpc -s dev -auto-approve",
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
	}, NewVariables())
	if err != nil {
		t.Fatalf("start recorder: %v", err)
	}
	restore()
	if err := rec.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}

	content, err := os.ReadFile(castPath)
	if err != nil {
		t.Fatalf("read cast: %v", err)
	}
	headerLine := content
	if index := len(content); index > 0 {
		for i, b := range content {
			if b == '\n' {
				headerLine = content[:i]
				break
			}
		}
	}
	var header struct {
		Version int    `json:"version"`
		Command string `json:"command"`
	}
	if err := json.Unmarshal(headerLine, &header); err != nil {
		t.Fatalf("parse header: %v", err)
	}
	if header.Version != 3 {
		t.Fatalf("header version = %d, want 3", header.Version)
	}
	if header.Command != "atmos terraform deploy vpc -s dev -auto-approve" {
		t.Fatalf("header command = %q", header.Command)
	}
}

func TestCastRecorderUsesStepTitleWithoutFakeCommand(t *testing.T) {
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	rec, restore, err := startStepRecorder(&schema.WorkflowStep{
		Name:  "list-instances",
		Type:  schema.TaskTypeCast,
		Title: "Quick Start Advanced: list instances",
		Env: map[string]string{
			"TERM":         "xterm-256color",
			"COLORTERM":    "truecolor",
			"SECRET_TOKEN": "redacted",
		},
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
	}, NewVariables())
	if err != nil {
		t.Fatalf("start recorder: %v", err)
	}
	restore()
	if err := rec.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}

	content, err := os.ReadFile(castPath)
	if err != nil {
		t.Fatalf("read cast: %v", err)
	}
	headerLine := strings.SplitN(string(content), "\n", 2)[0]
	var header struct {
		Title   string `json:"title"`
		Command string `json:"command"`
		Term    struct {
			Type string `json:"type"`
		} `json:"term"`
		Env map[string]string `json:"env"`
	}
	if err := json.Unmarshal([]byte(headerLine), &header); err != nil {
		t.Fatalf("parse header: %v", err)
	}
	if header.Title != "Quick Start Advanced: list instances" {
		t.Fatalf("header title = %q", header.Title)
	}
	if header.Command != "" {
		t.Fatalf("header command = %q, want empty", header.Command)
	}
	if header.Term.Type != "xterm-256color" {
		t.Fatalf("terminal type = %q", header.Term.Type)
	}
	if header.Env["COLORTERM"] != "truecolor" {
		t.Fatalf("safe env missing COLORTERM: %#v", header.Env)
	}
	if _, ok := header.Env["SECRET_TOKEN"]; ok {
		t.Fatalf("unsafe env was recorded: %#v", header.Env)
	}
}

func TestCastRecorderUsesCastDefaultsForTerminalSettings(t *testing.T) {
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	rec, restore, err := startStepRecorder(&schema.WorkflowStep{
		Name: "terraform-plan",
		Type: schema.TaskTypeCast,
		Defaults: &schema.CastDefaults{
			Cast: &schema.CastRecordingDefaults{
				Rate:   "12ms",
				Width:  100,
				Height: 32,
			},
		},
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
	}, NewVariables())
	if err != nil {
		t.Fatalf("start recorder: %v", err)
	}
	restore()
	if err := rec.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}

	content, err := os.ReadFile(castPath)
	if err != nil {
		t.Fatalf("read cast: %v", err)
	}
	headerLine := content
	for i, b := range content {
		if b == '\n' {
			headerLine = content[:i]
			break
		}
	}
	var header struct {
		Term struct {
			Cols int `json:"cols"`
			Rows int `json:"rows"`
		} `json:"term"`
	}
	if err := json.Unmarshal(headerLine, &header); err != nil {
		t.Fatalf("parse header: %v", err)
	}
	if header.Term.Cols != 100 || header.Term.Rows != 32 {
		t.Fatalf("header dimensions = %dx%d, want 100x32", header.Term.Cols, header.Term.Rows)
	}
}

func TestCastRecordingDefaultsRespectExplicitOverrides(t *testing.T) {
	step := &schema.WorkflowStep{
		Rate:   "7ms",
		Width:  90,
		Height: 28,
		Defaults: &schema.CastDefaults{
			Cast: &schema.CastRecordingDefaults{
				Rate:   "12ms",
				Width:  100,
				Height: 32,
			},
		},
	}

	applyCastRecordingDefaults(step)

	if step.Rate != "7ms" || step.Width != 90 || step.Height != 28 {
		t.Fatalf("explicit cast fields were overwritten: %#v", step)
	}
}

func TestPrepareCastChildStepInheritsOutputMode(t *testing.T) {
	castStep := &schema.WorkflowStep{
		Name:             "demo",
		Output:           string(OutputModeRaw),
		WorkingDirectory: "fixture",
		Show:             &schema.ShowConfig{Labels: BoolPtr(false)},
	}
	child := &schema.WorkflowStep{
		Name: "list",
	}

	prepareCastChildStep(castStep, child, 0)

	if child.Output != string(OutputModeRaw) {
		t.Fatalf("child output = %q, want raw", child.Output)
	}
	if child.Type != schema.TaskTypeShell {
		t.Fatalf("child type = %q, want shell", child.Type)
	}
	if child.WorkingDirectory != "fixture" {
		t.Fatalf("child working directory = %q, want fixture", child.WorkingDirectory)
	}
	if child.Show == nil || child.Show.Labels == nil || *child.Show.Labels {
		t.Fatalf("child show.labels = %#v, want false", child.Show)
	}
}

func TestPrepareCastChildStepAppliesSimulateDefaults(t *testing.T) {
	cursor := true
	castStep := &schema.WorkflowStep{
		Name: "demo",
		Defaults: &schema.CastDefaults{
			Simulate: &schema.CastSimulateDefaults{
				Mode:     "typed",
				Cursor:   &cursor,
				Rate:     "35ms",
				Jitter:   0.25,
				Duration: "10ms",
				Interval: "20ms",
				Prompt: &schema.SimulatePrompt{
					Text:  "> ",
					Style: "command",
				},
			},
		},
	}
	child := &schema.WorkflowStep{
		Type: schema.TaskTypeSimulate,
		Text: "atmos version",
	}

	prepareCastChildStep(castStep, child, 0)

	if child.Mode != "typed" {
		t.Fatalf("child mode = %q, want typed", child.Mode)
	}
	if !child.Cursor || !child.CursorSet {
		t.Fatalf("child cursor = %v cursorSet = %v, want true/true", child.Cursor, child.CursorSet)
	}
	if child.Rate != "35ms" || child.Jitter != 0.25 || child.Duration != "10ms" || child.Interval != "20ms" {
		t.Fatalf("child timing defaults not applied: %#v", child)
	}
	if child.SimulatePrompt == nil || child.SimulatePrompt.Text != "> " || child.SimulatePrompt.Style != "command" {
		t.Fatalf("child prompt = %#v, want command prompt", child.SimulatePrompt)
	}
}

func TestPrepareCastChildStepSimulateDefaultsRespectOverrides(t *testing.T) {
	cursor := true
	castStep := &schema.WorkflowStep{
		Name: "demo",
		Defaults: &schema.CastDefaults{
			Simulate: &schema.CastSimulateDefaults{
				Cursor: &cursor,
				Rate:   "35ms",
				Prompt: &schema.SimulatePrompt{
					Text:  "> ",
					Style: "command",
				},
			},
		},
	}
	child := &schema.WorkflowStep{
		Type:      schema.TaskTypeSimulate,
		Cursor:    false,
		CursorSet: true,
		Rate:      "5ms",
		SimulatePrompt: &schema.SimulatePrompt{
			Text: "$ ",
		},
		Text: "atmos version",
	}

	prepareCastChildStep(castStep, child, 0)

	if child.Cursor {
		t.Fatal("child cursor should keep explicit false override")
	}
	if child.Rate != "5ms" {
		t.Fatalf("child rate = %q, want override 5ms", child.Rate)
	}
	if child.SimulatePrompt == nil || child.SimulatePrompt.Text != "$ " || child.SimulatePrompt.Style != "command" {
		t.Fatalf("child prompt = %#v, want text override with default style", child.SimulatePrompt)
	}
}

func TestPrepareCastChildStepDoesNotApplySimulateDefaultsToShell(t *testing.T) {
	cursor := true
	castStep := &schema.WorkflowStep{
		Name: "demo",
		Defaults: &schema.CastDefaults{
			Simulate: &schema.CastSimulateDefaults{
				Cursor: &cursor,
				Rate:   "35ms",
			},
		},
	}
	child := &schema.WorkflowStep{
		Type: schema.TaskTypeShell,
	}

	prepareCastChildStep(castStep, child, 0)

	if child.Cursor || child.CursorSet || child.Rate != "" {
		t.Fatalf("simulate defaults leaked to shell child: %#v", child)
	}
}

func TestApplyCastSimulatePromptDefaultNilDefaultPromptIsNoop(t *testing.T) {
	child := &schema.WorkflowStep{
		SimulatePrompt: &schema.SimulatePrompt{Text: "$ ", Style: "command"},
	}

	applyCastSimulatePromptDefault(nil, child)

	if child.SimulatePrompt.Text != "$ " || child.SimulatePrompt.Style != "command" {
		t.Fatalf("child prompt changed unexpectedly: %#v", child.SimulatePrompt)
	}
}

func TestApplyCastSimulatePromptDefaultFillsEmptyText(t *testing.T) {
	defaultPrompt := &schema.SimulatePrompt{Text: "default> ", Style: "info"}
	child := &schema.WorkflowStep{
		SimulatePrompt: &schema.SimulatePrompt{Text: "", Style: ""},
	}

	applyCastSimulatePromptDefault(defaultPrompt, child)

	if child.SimulatePrompt.Text != "default> " {
		t.Fatalf("child prompt text = %q, want default text", child.SimulatePrompt.Text)
	}
	if child.SimulatePrompt.Style != "info" {
		t.Fatalf("child prompt style = %q, want default style", child.SimulatePrompt.Style)
	}
}

func TestCloneSimulatePromptNil(t *testing.T) {
	if got := cloneSimulatePrompt(nil); got != nil {
		t.Fatalf("cloneSimulatePrompt(nil) = %#v, want nil", got)
	}
}

func TestCastHandlerStepsInheritWorkingDirectory(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	ui.InitFormatter(iolib.GetContext())
	tmpDir := t.TempDir()
	castPath := filepath.Join(tmpDir, "demo.cast")
	markerPath := filepath.Join(tmpDir, "child.cwd")

	_, err := (&CastHandler{}).Execute(context.Background(), &schema.WorkflowStep{
		Name:             "demo",
		Type:             schema.TaskTypeCast,
		WorkingDirectory: tmpDir,
		Output:           string(OutputModeNone),
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
		Steps: []schema.WorkflowStep{
			{
				Name:    "pwd",
				Type:    schema.TaskTypeShell,
				Command: "printf inherited > child.cwd",
			},
		},
	}, NewVariables())
	if err != nil {
		t.Fatalf("execute cast: %v", err)
	}

	actual, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("read inherited marker: %v", err)
	}
	if string(actual) != "inherited" {
		t.Fatalf("child marker = %q, want inherited", actual)
	}
}

func TestCastHandlerStepsInheritEnv(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	tmpDir := t.TempDir()
	castPath := filepath.Join(tmpDir, "demo.cast")
	envPath := filepath.Join(tmpDir, "env.txt")

	_, err := (&CastHandler{}).Execute(context.Background(), &schema.WorkflowStep{
		Name:             "demo",
		Type:             schema.TaskTypeCast,
		WorkingDirectory: tmpDir,
		Output:           string(OutputModeNone),
		Env: map[string]string{
			"CAST_SHARED_ENV": "from-cast",
		},
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
		Steps: []schema.WorkflowStep{
			{
				Name:    "env",
				Type:    schema.TaskTypeShell,
				Command: `printf %s "$CAST_SHARED_ENV" > env.txt`,
			},
		},
	}, NewVariables())
	if err != nil {
		t.Fatalf("execute cast: %v", err)
	}

	actual, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read inherited env: %v", err)
	}
	if string(actual) != "from-cast" {
		t.Fatalf("child env = %q, want from-cast", actual)
	}
}

func TestCastHandlerDiscardsRecordingWhenChildFails(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	tmpDir := t.TempDir()
	castPath := filepath.Join(tmpDir, "demo.cast")
	if err := os.WriteFile(castPath, []byte("previous good cast"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (&CastHandler{}).Execute(context.Background(), &schema.WorkflowStep{
		Name:             "demo",
		Type:             schema.TaskTypeCast,
		WorkingDirectory: tmpDir,
		Output:           string(OutputModeNone),
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
		Steps: []schema.WorkflowStep{
			{
				Name:    "boom",
				Type:    schema.TaskTypeAtmos,
				Command: "terraform apply",
				Output:  string(OutputModeNone),
				Env:     map[string]string{"_ATMOS_STEP_FAKE": "fail"},
			},
		},
	}, NewVariables())
	if err == nil {
		t.Fatal("expected cast step to fail when a child step fails")
	}

	content, readErr := os.ReadFile(castPath)
	if readErr != nil {
		t.Fatalf("read cast: %v", readErr)
	}
	if string(content) != "previous good cast" {
		t.Fatalf("failed recording replaced the committed cast: %q", content)
	}
}

func TestCastHandlerCommitsRecordingWhenChildrenSucceed(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	tmpDir := t.TempDir()
	castPath := filepath.Join(tmpDir, "demo.cast")
	if err := os.WriteFile(castPath, []byte("stale cast"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (&CastHandler{}).Execute(context.Background(), &schema.WorkflowStep{
		Name:             "demo",
		Type:             schema.TaskTypeCast,
		WorkingDirectory: tmpDir,
		Output:           string(OutputModeNone),
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
		Steps: []schema.WorkflowStep{
			{
				Name:    "ok",
				Type:    schema.TaskTypeAtmos,
				Command: "terraform plan",
				Output:  string(OutputModeNone),
				Env:     map[string]string{"_ATMOS_STEP_FAKE": "ok"},
			},
		},
	}, NewVariables())
	if err != nil {
		t.Fatalf("execute cast: %v", err)
	}

	content, readErr := os.ReadFile(castPath)
	if readErr != nil {
		t.Fatalf("read cast: %v", readErr)
	}
	if string(content) == "stale cast" {
		t.Fatal("successful recording did not replace the stale cast")
	}
	if !strings.HasPrefix(string(content), `{"version":3`) {
		t.Fatalf("committed cast is not an asciicast recording: %.60q", content)
	}
}

func TestCastHandlerRecordsToastStep(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	ui.InitFormatter(iolib.GetContext())
	tmpDir := t.TempDir()
	castPath := filepath.Join(tmpDir, "demo.cast")

	_, err := (&CastHandler{}).Execute(context.Background(), &schema.WorkflowStep{
		Name:   "demo",
		Type:   schema.TaskTypeCast,
		Output: string(OutputModeNone),
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
		Steps: []schema.WorkflowStep{
			{
				Name:    "notice",
				Type:    "toast",
				Level:   "info",
				Content: "Recorded cast: .context/cast-command-demo/about.cast",
			},
		},
	}, NewVariables())
	if err != nil {
		t.Fatalf("execute cast: %v", err)
	}

	content, err := os.ReadFile(castPath)
	if err != nil {
		t.Fatalf("read cast: %v", err)
	}
	if !strings.Contains(ansi.Strip(castEventText(t, content)), "Recorded cast: .context/cast-command-demo/about.cast") {
		t.Fatalf("toast output was not recorded in cast:\n%s", content)
	}
}

func TestNormalizeCastOutputModeUsesStructuredOutput(t *testing.T) {
	castStep := &schema.WorkflowStep{
		Name: "demo",
		CastOutput: &schema.CastOutput{
			Mode: string(OutputModeRaw),
			Cast: "demo.cast",
		},
	}

	normalizeCastOutputMode(castStep)

	if castStep.Output != string(OutputModeRaw) {
		t.Fatalf("cast output = %q, want raw", castStep.Output)
	}
}

func TestApplyCastStepEnvAddsEnvToVariables(t *testing.T) {
	vars := NewVariables()
	binDir := filepath.Join(t.TempDir(), "bin")
	prefix := binDir + string(os.PathListSeparator)
	castStep := &schema.WorkflowStep{
		Name: "demo",
		Env: map[string]string{
			"ATMOS_FORCE_COLOR": "1",
			"PATH":              prefix + "{{ .env.PATH }}",
		},
	}

	if err := applyCastStepEnv(castStep, vars); err != nil {
		t.Fatalf("applyCastStepEnv error: %v", err)
	}

	if vars.Env["ATMOS_FORCE_COLOR"] != "1" {
		t.Fatalf("ATMOS_FORCE_COLOR = %q, want 1", vars.Env["ATMOS_FORCE_COLOR"])
	}
	if !strings.HasPrefix(vars.Env["PATH"], prefix) {
		t.Fatalf("PATH = %q, want %q prefix", vars.Env["PATH"], prefix)
	}
}

// withRestoredEnv snapshots key's current value and schedules it to be
// restored (set or unset, whichever matched) after the test completes.
func withRestoredEnv(t *testing.T, key string) {
	t.Helper()
	origValue, origSet := os.LookupEnv(key)
	t.Cleanup(func() {
		if origSet {
			_ = os.Setenv(key, origValue)
		} else {
			_ = os.Unsetenv(key)
		}
	})
	_ = os.Unsetenv(key)
}

func TestForceRecordingUIEnvForcesTrueColorAndRestoresEnv(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	ui.InitFormatter(iolib.GetContext())
	t.Cleanup(ui.ReinitFormatter)

	// Normally bound once in cmd/root.go's init(), which this package-level
	// test binary never runs. Bind it here so viper.GetBool("force-color")
	// (consulted by pkg/terminal's color detection during ReinitFormatter)
	// sees ATMOS_FORCE_COLOR the same way it would in the real atmos binary.
	if err := viper.BindEnv("force-color", "ATMOS_FORCE_COLOR", "CLICOLOR_FORCE"); err != nil {
		t.Fatalf("bind force-color env: %v", err)
	}

	const key = "ATMOS_FORCE_COLOR"
	withRestoredEnv(t, key)

	restore := forceRecordingUIEnv(map[string]string{key: "1"})

	if got, ok := os.LookupEnv(key); !ok || got != "1" {
		t.Fatalf("%s during recording = (%q, %v), want (1, true)", key, got, ok)
	}
	if got := ui.GetColorProfile(); got != termenv.TrueColor {
		t.Fatalf("color profile during recording = %v, want TrueColor", got)
	}

	restore()

	if _, ok := os.LookupEnv(key); ok {
		t.Fatalf("%s after restore should be unset", key)
	}
}

func TestForceRecordingUIEnvNoColorForcesAsciiProfile(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	ui.InitFormatter(iolib.GetContext())
	t.Cleanup(ui.ReinitFormatter)

	const key = "NO_COLOR"
	withRestoredEnv(t, key)

	restore := forceRecordingUIEnv(map[string]string{key: "1"})

	if got := ui.GetColorProfile(); got != termenv.Ascii {
		t.Fatalf("color profile with NO_COLOR set = %v, want Ascii", got)
	}

	restore()
}

func TestForceRecordingUIEnvNestedCallsOnlyRestoreOnOutermostExit(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	ui.InitFormatter(iolib.GetContext())
	t.Cleanup(ui.ReinitFormatter)

	const key = "FORCE_COLOR"
	withRestoredEnv(t, key)

	outerRestore := forceRecordingUIEnv(map[string]string{key: "1"})
	innerRestore := forceRecordingUIEnv(map[string]string{key: "1"})

	innerRestore()

	// The outer scope is still active — env must still be forced.
	if got, ok := os.LookupEnv(key); !ok || got != "1" {
		t.Fatalf("%s after inner restore = (%q, %v), want (1, true) while outer scope is active", key, got, ok)
	}

	outerRestore()

	if _, ok := os.LookupEnv(key); ok {
		t.Fatalf("%s after outer restore should be unset", key)
	}
}

func TestForceRecordingUIEnvNoopWhenNoRelevantKeysPresent(t *testing.T) {
	restore := forceRecordingUIEnv(map[string]string{"SOME_OTHER_VAR": "x"})
	restore() // must not panic
}

func TestRecordCastTypedLineWritesPromptAndCharactersAsEvents(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	rec, restore, err := startStepRecorder(&schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
	}, NewVariables())
	if err != nil {
		t.Fatalf("start recorder: %v", err)
	}
	t.Cleanup(restore)

	if err := recordCastTypedLine(context.Background(), castTypedLineOptions{
		Prompt: &schema.SimulatePrompt{Text: "> ", Style: "command"},
		Line:   "# inspect",
	}); err != nil {
		t.Fatalf("record typed line: %v", err)
	}
	restore()
	if err := rec.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}

	content, err := os.ReadFile(castPath)
	if err != nil {
		t.Fatalf("read cast: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) < len("# inspect")+3 {
		t.Fatalf("expected prompt, per-character, and newline events, got %d lines:\n%s", len(lines), content)
	}
	for _, want := range []string{`\u003e`, `"#"`, `" "`, `"i"`, `"\n"`} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("typed cast event missing %s in:\n%s", want, content)
		}
	}
}

func TestRecordCastTypedLineKeepsCursorVisibleWhenEnabled(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	rec, restore, err := startStepRecorder(&schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
	}, NewVariables())
	if err != nil {
		t.Fatalf("start recorder: %v", err)
	}
	t.Cleanup(restore)

	if err := recordCastTypedLine(context.Background(), castTypedLineOptions{
		Prompt: &schema.SimulatePrompt{Text: "> ", Style: "command"},
		Line:   "atmos version",
		Cursor: true,
	}); err != nil {
		t.Fatalf("record typed line: %v", err)
	}
	restore()
	if err := rec.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}

	content, err := os.ReadFile(castPath)
	if err != nil {
		t.Fatalf("read cast: %v", err)
	}
	castContent := string(content)
	if strings.Count(castContent, `\u001b[?25h`) != 1 {
		t.Fatalf("cursor show events missing in:\n%s", content)
	}
	if strings.Contains(castContent, `\u001b[?25l`) {
		t.Fatalf("cursor hide event should not span typed input:\n%s", content)
	}
	showIndex := strings.Index(castContent, `\u001b[?25h`)
	firstCharIndex := strings.Index(castContent, `"a"`)
	if showIndex == -1 || firstCharIndex == -1 || showIndex > firstCharIndex {
		t.Fatalf("cursor show must appear before typed characters:\n%s", content)
	}
}

func TestCastTypedCharDelayUsesDeterministicJitter(t *testing.T) {
	base := 100 * time.Millisecond
	line := "atmos plan"
	chars := []rune(line)

	for i := range chars {
		first := castTypedCharDelay(line, chars, i, base, 0.25)
		second := castTypedCharDelay(line, chars, i, base, 0.25)
		if first != second {
			t.Fatalf("delay for index %d was not deterministic: %s != %s", i, first, second)
		}
	}

	if got := castTypedCharDelay(line, chars, 1, base, 0); got != base {
		t.Fatalf("jitter disabled delay = %s, want %s", got, base)
	}
	if got := castTypedCharDelay(line, chars, 6, base, 0.25); got <= base {
		t.Fatalf("word boundary delay = %s, want greater than %s", got, base)
	}
}

func TestCastTypedCharDelayPunctuationBoundaryAndCommentFactor(t *testing.T) {
	base := 100 * time.Millisecond

	punctuationLine := "a:b"
	punctuationChars := []rune(punctuationLine)
	if got := castTypedCharDelay(punctuationLine, punctuationChars, 2, base, 0.25); got <= base {
		t.Fatalf("punctuation boundary delay = %s, want greater than %s", got, base)
	}

	// Recompute the expected delay directly from the same deterministic
	// jitter formula castTypedCharDelay uses, so the comment-typing
	// multiplier is checked exactly rather than by an input-dependent
	// comparison (different line text hashes to a different jitter unit, so
	// comparing two different lines' delays is not reliably ordered).
	commentLine := "# atmos version"
	commentChars := []rune(commentLine)
	index := 5
	jitter := 0.25
	unit := deterministicCastJitterUnit(commentLine, index)
	wantFactor := (1 - jitter + (2 * jitter * unit)) * castCommentTypingFactor
	want := time.Duration(float64(base) * wantFactor)
	if got := castTypedCharDelay(commentLine, commentChars, index, base, jitter); got != want {
		t.Fatalf("comment typing delay = %s, want %s", got, want)
	}
}

func TestRecordCastPromptWritesPromptEvent(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	rec, restore, err := startStepRecorder(&schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
	}, NewVariables())
	if err != nil {
		t.Fatalf("start recorder: %v", err)
	}
	t.Cleanup(restore)

	if err := recordCastPrompt(&schema.SimulatePrompt{Text: "> ", Style: "command"}); err != nil {
		t.Fatalf("record prompt: %v", err)
	}
	restore()
	if err := rec.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}

	content, err := os.ReadFile(castPath)
	if err != nil {
		t.Fatalf("read cast: %v", err)
	}
	if !strings.Contains(string(content), `\u003e`) {
		t.Fatalf("prompt event missing in:\n%s", content)
	}
}

func TestRecordCastPromptReturnsRenderError(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	err := recordCastPrompt(&schema.SimulatePrompt{Text: "> ", Style: "purple"})
	if !errors.Is(err, ErrUnsupportedPromptStyle) {
		t.Fatalf("expected unsupported prompt style error, got %v", err)
	}
}

func TestRecordCastPromptWithCursorReturnsRenderError(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	err := recordCastPromptWithCursor(&schema.SimulatePrompt{Text: "> ", Style: "purple"}, true)
	if !errors.Is(err, ErrUnsupportedPromptStyle) {
		t.Fatalf("expected unsupported prompt style error, got %v", err)
	}
}

func TestRunCastSimulateStepResolvesTextTemplate(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	rec, restore, err := startStepRecorder(&schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
	}, NewVariables())
	if err != nil {
		t.Fatalf("start recorder: %v", err)
	}
	t.Cleanup(restore)

	vars := NewVariables()
	vars.SetEnv("STACK", "dev")
	err = runCastSimulateStep(context.Background(), &schema.WorkflowStep{WriteRate: "0"}, &schema.WorkflowStep{
		Type: schema.TaskTypeSimulate,
		Mode: "typed",
		Text: "atmos list vars api --stack {{ .env.STACK }}",
		SimulatePrompt: &schema.SimulatePrompt{
			Text:  "> ",
			Style: "command",
		},
	}, vars, false)
	if err != nil {
		t.Fatalf("run simulate: %v", err)
	}
	restore()
	if err := rec.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}

	content, err := os.ReadFile(castPath)
	if err != nil {
		t.Fatalf("read cast: %v", err)
	}
	if !strings.Contains(castOutputText(t, content), "atmos list vars api --stack dev") {
		t.Fatalf("simulated command missing in:\n%s", content)
	}
}

func TestRunCastSimulateStepReturnsTextResolutionError(t *testing.T) {
	err := runCastSimulateStep(context.Background(), &schema.WorkflowStep{}, &schema.WorkflowStep{
		Type: schema.TaskTypeSimulate,
		Mode: "typed",
		Text: "{{ .Bad ",
	}, NewVariables(), false)
	if err == nil {
		t.Fatal("expected an error resolving an invalid text template")
	}
}

func TestRunCastSimulateStepReturnsWriteRateParseError(t *testing.T) {
	err := runCastSimulateStep(context.Background(), &schema.WorkflowStep{}, &schema.WorkflowStep{
		Type: schema.TaskTypeSimulate,
		Mode: "typed",
		Text: "atmos version",
		Rate: "bad-duration",
	}, NewVariables(), false)
	if err == nil {
		t.Fatal("expected an error parsing an invalid write rate")
	}
}

func TestRunCastSimulateStepReturnsEnterDelayParseError(t *testing.T) {
	err := runCastSimulateStep(context.Background(), &schema.WorkflowStep{}, &schema.WorkflowStep{
		Type:     schema.TaskTypeSimulate,
		Mode:     "typed",
		Text:     "atmos version",
		Duration: "bad-duration",
	}, NewVariables(), false)
	if err == nil {
		t.Fatal("expected an error parsing an invalid enter delay duration")
	}
}

func TestRunCastSimulateStepPromptModeWritesPrompt(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	rec, restore, err := startStepRecorder(&schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
	}, NewVariables())
	if err != nil {
		t.Fatalf("start recorder: %v", err)
	}
	t.Cleanup(restore)

	err = runCastSimulateStep(context.Background(), &schema.WorkflowStep{}, &schema.WorkflowStep{
		Type: schema.TaskTypeSimulate,
		Mode: "prompt",
		SimulatePrompt: &schema.SimulatePrompt{
			Text:  "ready> ",
			Style: "info",
		},
	}, NewVariables(), false)
	if err != nil {
		t.Fatalf("run prompt simulate: %v", err)
	}
	restore()
	if err := rec.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}

	content, err := os.ReadFile(castPath)
	if err != nil {
		t.Fatalf("read cast: %v", err)
	}
	if !strings.Contains(castOutputText(t, content), "ready> ") {
		t.Fatalf("prompt output missing in:\n%s", content)
	}
}

func TestRecordCastTypedLineReturnsContextCancellation(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	rec, restore, err := startStepRecorder(&schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
	}, NewVariables())
	if err != nil {
		t.Fatalf("start recorder: %v", err)
	}
	t.Cleanup(restore)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = recordCastTypedLine(ctx, castTypedLineOptions{
		Prompt: &schema.SimulatePrompt{Text: "> ", Style: "command"},
		Line:   "atmos version",
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
	restore()
	if err := rec.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}
}

func TestRecordCastTypedLineReturnsEnterDelayCancellation(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	rec, restore, err := startStepRecorder(&schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
	}, NewVariables())
	if err != nil {
		t.Fatalf("start recorder: %v", err)
	}
	t.Cleanup(restore)

	// The context is canceled after the prompt delay and typed characters
	// complete (WriteRate: 0 skips per-character sleeps) but before the enter
	// delay elapses, driving recordCastTypedLine's final sleepCastInput error
	// return.
	ctx, cancel := context.WithTimeout(context.Background(), defaultCastPromptDelay+50*time.Millisecond)
	defer cancel()
	err = recordCastTypedLine(ctx, castTypedLineOptions{
		Prompt:     &schema.SimulatePrompt{Text: "> ", Style: "command"},
		Line:       "x",
		WriteRate:  0,
		EnterDelay: 2 * time.Second,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
	restore()
	if err := rec.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}
}

func TestRecordCastTypedLineReturnsPromptRenderError(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	err := recordCastTypedLine(context.Background(), castTypedLineOptions{
		Prompt: &schema.SimulatePrompt{Text: "> ", Style: "purple"},
		Line:   "atmos version",
	})
	if !errors.Is(err, ErrUnsupportedPromptStyle) {
		t.Fatalf("expected unsupported prompt style error, got %v", err)
	}
}

func TestCastHandlerExecuteWithWorkflowRecordsSimulatedSteps(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	handler := &CastHandler{}
	vars := NewVariables()
	vars.SetEnv("STACK", "dev")

	result, err := handler.ExecuteWithWorkflow(context.Background(), &schema.WorkflowStep{
		Name:      "demo",
		Type:      schema.TaskTypeCast,
		Mode:      "steps",
		Output:    string(OutputModeRaw),
		WriteRate: "0",
		Env: map[string]string{
			"DEPLOY_STACK": "{{ .env.STACK }}",
		},
		CastOutput: &schema.CastOutput{
			Cast: castPath,
			Mode: string(OutputModeRaw),
		},
		Steps: []schema.WorkflowStep{{
			Type:   schema.TaskTypeSimulate,
			Mode:   "typed",
			Cursor: true,
			Text:   "atmos terraform plan app -s {{ .env.DEPLOY_STACK }}",
			SimulatePrompt: &schema.SimulatePrompt{
				Text:  "$ ",
				Style: "command",
			},
			Duration: "0",
		}},
	}, vars, &schema.WorkflowDefinition{Output: string(OutputModeRaw)})
	if err != nil {
		t.Fatalf("execute cast: %v", err)
	}
	if result.Value != castPath || result.Metadata["cast"] != castPath {
		t.Fatalf("unexpected result: %#v", result)
	}
	if vars.Env["DEPLOY_STACK"] != "dev" {
		t.Fatalf("cast env was not applied: %#v", vars.Env)
	}
	content, err := os.ReadFile(castPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(castOutputText(t, content), "atmos terraform plan app -s dev") {
		t.Fatalf("recorded cast missing resolved command:\n%s", content)
	}
	finalLine := strings.Split(strings.TrimSpace(string(content)), "\n")
	if !strings.Contains(finalLine[len(finalLine)-1], `\u003e \u001b[0m\u001b[?25h"`) {
		t.Fatalf("recorded cast should end with prompt and cursor show in one event:\n%s", content)
	}
}

func TestCastHandlerExecuteReturnsRenderErrorsWithMetadata(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	t.Setenv("PATH", t.TempDir())
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	gifPath := filepath.Join(t.TempDir(), "demo.gif")

	result, err := (&CastHandler{}).Execute(context.Background(), &schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		CastOutput: &schema.CastOutput{
			Cast: castPath,
			GIF:  gifPath,
		},
		Steps: []schema.WorkflowStep{{
			Type: schema.TaskTypeSimulate,
			Mode: "prompt",
		}},
	}, NewVariables())
	if !errors.Is(err, asciicast.ErrMissingAgg) {
		t.Fatalf("expected render error, got %v", err)
	}
	if result == nil || result.Metadata["cast"] != castPath || result.Metadata["gif"] != gifPath {
		t.Fatalf("unexpected result metadata: %#v", result)
	}
}

func TestCastHandlerStepsRunAlwaysCleanupAfterFailure(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	tmpDir := t.TempDir()
	castPath := filepath.Join(tmpDir, "demo.cast")
	mainPath := filepath.Join(tmpDir, "main.txt")
	afterPath := filepath.Join(tmpDir, "after.txt")
	cleanupPath := filepath.Join(tmpDir, "cleanup.txt")

	_, err := (&CastHandler{}).Execute(context.Background(), &schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
		Steps: []schema.WorkflowStep{
			{
				Name:             "main",
				Type:             schema.TaskTypeShell,
				WorkingDirectory: tmpDir,
				Output:           string(OutputModeNone),
				Command:          "printf main > main.txt; exit 7",
			},
			{
				Name:             "after",
				Type:             schema.TaskTypeShell,
				WorkingDirectory: tmpDir,
				Output:           string(OutputModeNone),
				Command:          "printf after > after.txt",
			},
			{
				Name:             "cleanup",
				Type:             schema.TaskTypeShell,
				When:             schema.MustCondition(schema.ConditionPredicateAlways),
				WorkingDirectory: tmpDir,
				Output:           string(OutputModeNone),
				Command:          "printf cleanup > cleanup.txt",
			},
		},
	}, NewVariables())

	if err == nil {
		t.Fatal("expected cast step failure")
	}
	if _, readErr := os.Stat(mainPath); readErr != nil {
		t.Fatalf("expected main step to run: %v", readErr)
	}
	if _, readErr := os.Stat(cleanupPath); readErr != nil {
		t.Fatalf("expected cleanup step to run: %v", readErr)
	}
	if _, readErr := os.Stat(afterPath); !errors.Is(readErr, os.ErrNotExist) {
		t.Fatalf("expected success-only step after failure to be skipped, stat err: %v", readErr)
	}
}

func TestRunCastStepModeRunsFailureConditionAfterSimulateError(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	rec, restore, err := startStepRecorder(&schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
	}, NewVariables())
	if err != nil {
		t.Fatalf("start recorder: %v", err)
	}
	t.Cleanup(restore)

	err = runCastStepMode(context.Background(), &schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		Steps: []schema.WorkflowStep{
			{
				Type: schema.TaskTypeSimulate,
				Mode: "invalid",
			},
			{
				Type: schema.TaskTypeSimulate,
				Mode: "prompt",
				When: schema.MustCondition(schema.ConditionPredicateFailure),
				SimulatePrompt: &schema.SimulatePrompt{
					Text:  "failed> ",
					Style: "notice",
				},
			},
			{
				Type: schema.TaskTypeSimulate,
				Mode: "prompt",
				SimulatePrompt: &schema.SimulatePrompt{
					Text: "skipped> ",
				},
			},
		},
	}, NewVariables(), &schema.WorkflowDefinition{})
	if !errors.Is(err, ErrInvalidSimulateMode) {
		t.Fatalf("expected invalid simulate mode, got %v", err)
	}
	restore()
	if err := rec.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}

	content, err := os.ReadFile(castPath)
	if err != nil {
		t.Fatalf("read cast: %v", err)
	}
	output := castOutputText(t, content)
	if !strings.Contains(output, "failed> ") {
		t.Fatalf("failure prompt did not run after simulate error:\n%s", content)
	}
	if strings.Contains(output, "skipped> ") {
		t.Fatalf("implicit-success prompt should be skipped after failure:\n%s", content)
	}
}

func TestCastValidateModesAndRequiredFields(t *testing.T) {
	h := &CastHandler{}
	tests := []struct {
		name string
		step schema.WorkflowStep
		want error
	}{
		{
			name: "steps requires nested steps",
			step: schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "steps"},
			want: ErrCastStepRequiresSteps,
		},
		{
			name: "session requires actions",
			step: schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "session"},
			want: ErrCastSessionRequiresActions,
		},
		{
			name: "invalid cast mode",
			step: schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "video"},
			want: ErrInvalidCastMode,
		},
		{
			name: "write action requires text",
			step: schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "session", Steps: []schema.WorkflowStep{{Type: "write"}}},
			want: ErrWriteActionRequiresText,
		},
		{
			name: "key action requires key",
			step: schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "session", Steps: []schema.WorkflowStep{{Type: "key"}}},
			want: ErrKeyActionRequiresKey,
		},
		{
			name: "pause action requires duration",
			step: schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "session", Steps: []schema.WorkflowStep{{Type: "pause"}}},
			want: ErrPauseActionRequiresDuration,
		},
		{
			name: "unsupported session action",
			step: schema.WorkflowStep{Name: "demo", Type: schema.TaskTypeCast, Mode: "session", Steps: []schema.WorkflowStep{{Type: "bogus"}}},
			want: ErrUnsupportedSessionAction,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.Validate(&tt.step)
			if !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
		})
	}
}

func TestCastValidateDurationsAndRegex(t *testing.T) {
	tests := []struct {
		name   string
		action schema.WorkflowStep
	}{
		{name: "write rate", action: schema.WorkflowStep{Type: "write", Text: "echo", Rate: "fast"}},
		{name: "key interval", action: schema.WorkflowStep{Type: "key", Key: "enter", Interval: "soon"}},
		{name: "pause duration", action: schema.WorkflowStep{Type: "pause", Duration: "later"}},
		{name: "wait regex", action: schema.WorkflowStep{Type: "wait", Regex: "["}},
		{name: "wait timeout", action: schema.WorkflowStep{Type: "wait", Text: "ready", Timeout: "eventually"}},
	}

	h := &CastHandler{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := h.Validate(&schema.WorkflowStep{
				Name:  "demo",
				Type:  schema.TaskTypeCast,
				Mode:  "session",
				Steps: []schema.WorkflowStep{tt.action},
			})
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestCastSimulateHelpers(t *testing.T) {
	if mode := castSimulateMode(&schema.WorkflowStep{}); mode != "typed" {
		t.Fatalf("default simulate mode = %q", mode)
	}
	if text := castPromptText(nil); text != "> " {
		t.Fatalf("default prompt text = %q", text)
	}
	if style := castPromptStyle(nil); style != "command" {
		t.Fatalf("default prompt style = %q", style)
	}
	if got := firstNonEmpty("", "first", "second"); got != "first" {
		t.Fatalf("firstNonEmpty = %q", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Fatalf("firstNonEmpty all empty = %q, want empty", got)
	}
	if got := firstNonZeroFloat(0, 0.5); got != 0.5 {
		t.Fatalf("firstNonZeroFloat = %v, want 0.5", got)
	}
	if got := firstNonZeroFloat(0, 0); got != 0 {
		t.Fatalf("firstNonZeroFloat all zero = %v, want 0", got)
	}
	if got, err := parseDurationDefault("", time.Second); err != nil || got != time.Second {
		t.Fatalf("default duration = %s err=%v", got, err)
	}
	if got, err := parseDurationDefault("0", time.Second); err != nil || got != 0 {
		t.Fatalf("zero duration = %s err=%v", got, err)
	}
	if _, err := parseDurationDefault("invalid", 0); err == nil {
		t.Fatal("expected invalid duration error")
	}
	if _, err := castStepPauseDelay(&schema.WorkflowStep{Interval: "bad"}); err == nil {
		t.Fatal("expected invalid pause delay error")
	}
	if delay, err := castStepEnterDelay(&schema.WorkflowStep{Duration: "2ms"}); err != nil || delay != 2*time.Millisecond {
		t.Fatalf("enter delay = %s err=%v", delay, err)
	}
}

func TestRenderCastPromptStylesAndInvalidStyle(t *testing.T) {
	for _, style := range []string{"command", "label", "muted", "info", "notice", "   "} {
		t.Run(style, func(t *testing.T) {
			rendered, err := renderCastPrompt(&schema.SimulatePrompt{Text: "$ ", Style: style})
			if err != nil {
				t.Fatalf("render prompt: %v", err)
			}
			if !strings.Contains(rendered, "$ ") {
				t.Fatalf("rendered prompt = %q", rendered)
			}
		})
	}

	_, err := renderCastPrompt(&schema.SimulatePrompt{Text: "$ ", Style: "purple"})
	if !errors.Is(err, ErrUnsupportedPromptStyle) {
		t.Fatalf("expected unsupported prompt style, got %v", err)
	}
}

func TestRenderCastTypedLinePartsStylesCommentsAsMuted(t *testing.T) {
	prompt := &schema.SimulatePrompt{Text: "> ", Style: "command"}

	promptText, err := renderCastPrompt(prompt)
	if err != nil {
		t.Fatalf("render prompt: %v", err)
	}
	commandPrefix, commandSuffix, err := renderCastTypedLineParts(prompt, "atmos version")
	if err != nil {
		t.Fatalf("render command parts: %v", err)
	}
	commentPrefix, commentSuffix, err := renderCastTypedLineParts(prompt, "# this is how to run it")
	if err != nil {
		t.Fatalf("render comment parts: %v", err)
	}

	if commandPrefix == "" || commandSuffix == "" {
		t.Fatalf("expected command ANSI parts, got prefix=%q suffix=%q", commandPrefix, commandSuffix)
	}
	if strings.HasPrefix(promptText, commandPrefix) {
		t.Fatalf("expected typed command prefix %q to differ from prompt %q", commandPrefix, promptText)
	}
	if commentPrefix == "" || commentSuffix == "" {
		t.Fatalf("expected comment ANSI parts, got prefix=%q suffix=%q", commentPrefix, commentSuffix)
	}
	if commandPrefix == commentPrefix {
		t.Fatalf("expected command and comment prefixes to differ, both were %q", commandPrefix)
	}
}

func TestValidateCastSimulateStepModesAndDurations(t *testing.T) {
	tests := []struct {
		name      string
		step      schema.WorkflowStep
		want      error
		expectErr bool
	}{
		{name: "typed valid", step: schema.WorkflowStep{Mode: "typed", Text: "atmos version"}},
		{name: "prompt valid", step: schema.WorkflowStep{Mode: "prompt"}},
		{name: "typed rate invalid", step: schema.WorkflowStep{Mode: "typed", Text: "x", Rate: "bad"}, expectErr: true},
		{name: "typed interval invalid", step: schema.WorkflowStep{Mode: "typed", Text: "x", Interval: "bad"}, expectErr: true},
		{name: "typed duration invalid", step: schema.WorkflowStep{Mode: "typed", Text: "x", Duration: "bad"}, expectErr: true},
		{name: "typed jitter invalid", step: schema.WorkflowStep{Mode: "typed", Text: "x", Jitter: 1.25}, want: ErrInvalidSimulateJitter, expectErr: true},
		{name: "invalid mode", step: schema.WorkflowStep{Mode: "movie"}, want: ErrInvalidSimulateMode, expectErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCastSimulateStep(&tt.step)
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("expected %v, got %v", tt.want, err)
			}
			if !tt.expectErr && err != nil {
				t.Fatalf("expected valid step, got %v", err)
			}
			if tt.expectErr && err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestCastSessionActionsAndCommandArgs(t *testing.T) {
	castStep := &schema.WorkflowStep{Steps: []schema.WorkflowStep{{
		Type:     "write",
		Text:     "echo hi",
		Regex:    "hi",
		Key:      "enter",
		Duration: "1s",
		Timeout:  "2s",
		Rate:     "3ms",
		Interval: "4ms",
		Repeat:   2,
	}}}
	actions := castSessionActions(context.Background(), castStep, NewVariables(), nil)
	if len(actions) != 1 {
		t.Fatalf("action count = %d", len(actions))
	}
	// Fn is only set for simulate actions; this is a "write" action, so both
	// sides are nil and reflect.DeepEqual (unlike ==) tolerates that.
	want := asciicast.SessionAction{Type: "write", Text: "echo hi", Regex: "hi", Key: "enter", Duration: "1s", Timeout: "2s", Rate: "3ms", Interval: "4ms", Repeat: 2}
	if !reflect.DeepEqual(actions[0], want) {
		t.Fatalf("action = %#v, want %#v", actions[0], want)
	}

	if got := castCommandArgs(&schema.WorkflowStep{Name: "demo"}); len(got) != 0 {
		t.Fatalf("default command args = %v", got)
	}
	if got := castCommandArgs(&schema.WorkflowStep{Command: "atmos terraform plan vpc"}); strings.Join(got, " ") != "atmos terraform plan vpc" {
		t.Fatalf("explicit command args = %v", got)
	}
	if mode := castMode(&schema.WorkflowStep{}); mode != "steps" {
		t.Fatalf("default cast mode = %q", mode)
	}
}

// TestCastSessionActionsSimulateChildGetsCallback verifies a type: simulate
// child in a session's steps becomes a "simulate" SessionAction carrying a
// non-nil Fn, rather than being converted into a write/key/pause/wait
// action -- this is the wiring that lets a session mix in the same styled,
// non-interactive narration mode: steps uses via type: simulate, instead of
// typing raw (unstyled) keystrokes for comment lines.
func TestCastSessionActionsSimulateChildGetsCallback(t *testing.T) {
	castStep := &schema.WorkflowStep{
		Steps: []schema.WorkflowStep{
			{Type: schema.TaskTypeSimulate, Text: "# narration"},
			{Type: "write", Text: "echo hi"},
		},
	}
	actions := castSessionActions(context.Background(), castStep, NewVariables(), nil)
	if len(actions) != 2 {
		t.Fatalf("action count = %d", len(actions))
	}
	if actions[0].Type != schema.TaskTypeSimulate || actions[0].Fn == nil {
		t.Fatalf("expected a simulate action with a callback, got %#v", actions[0])
	}
	if actions[1].Type != "write" || actions[1].Fn != nil {
		t.Fatalf("expected a plain write action with no callback, got %#v", actions[1])
	}
}

func TestCastSessionActionsExecChildGetsSimulateCallback(t *testing.T) {
	castStep := &schema.WorkflowStep{
		Steps: []schema.WorkflowStep{{
			Name:    "plan",
			Type:    schema.TaskTypeAtmos,
			Command: "terraform plan",
			Output:  string(OutputModeNone),
			Env:     map[string]string{"_ATMOS_STEP_FAKE": "ok"},
		}},
	}
	actions := castSessionActions(context.Background(), castStep, NewVariables(), nil)
	if len(actions) != 1 {
		t.Fatalf("action count = %d", len(actions))
	}
	if actions[0].Type != schema.TaskTypeSimulate || actions[0].Fn == nil {
		t.Fatalf("expected an executable child to become a simulate callback, got %#v", actions[0])
	}
}

// TestSessionPromptDefault covers the three-tier fallback
// applySessionPromptEnv relies on: a child-level SimulatePrompt beats the
// cast step's own, which beats the built-in "> "/"command" default.
func TestSessionPromptDefault(t *testing.T) {
	own := &schema.SimulatePrompt{Text: "own> "}
	fromDefaults := &schema.SimulatePrompt{Text: "default> "}

	if got := sessionPromptDefault(&schema.WorkflowStep{SimulatePrompt: own}); got != own {
		t.Fatalf("expected the step's own prompt to win, got %#v", got)
	}
	if got := sessionPromptDefault(&schema.WorkflowStep{
		Defaults: &schema.CastDefaults{Simulate: &schema.CastSimulateDefaults{Prompt: fromDefaults}},
	}); got != fromDefaults {
		t.Fatalf("expected the cast step's Defaults.Simulate.Prompt, got %#v", got)
	}
	if got := sessionPromptDefault(&schema.WorkflowStep{}); got != nil {
		t.Fatalf("expected nil (built-in fallback) when nothing is configured, got %#v", got)
	}
}

// TestApplySessionPromptEnv covers PS1 injection: it renders the resolved
// prompt into env["PS1"], and leaves an explicit caller-supplied PS1 alone.
func TestApplySessionPromptEnv(t *testing.T) {
	env := map[string]string{}
	if err := applySessionPromptEnv(&schema.WorkflowStep{}, env); err != nil {
		t.Fatalf("applySessionPromptEnv error: %v", err)
	}
	if env["PS1"] == "" || env["PS1"] == "> " {
		t.Fatalf("expected a styled (ANSI-wrapped) PS1, got %q", env["PS1"])
	}

	explicit := map[string]string{"PS1": "custom$ "}
	if err := applySessionPromptEnv(&schema.WorkflowStep{}, explicit); err != nil {
		t.Fatalf("applySessionPromptEnv error: %v", err)
	}
	if explicit["PS1"] != "custom$ " {
		t.Fatalf("expected an explicit PS1 to be left untouched, got %q", explicit["PS1"])
	}
}

// TestRunCastSessionModeInterleavesSimulateNarration is an end-to-end proof
// that a type: simulate action mixed into mode: session's steps renders
// through the same styled path type: simulate uses in mode: steps (writing
// via pkg/data, captured by the same recorder that captures the real PTY's
// output), landing in the final .cast alongside genuine command output.
func TestRunCastSessionModeInterleavesSimulateNarration(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	shell, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(sessionShellHelperEnv, "1")
	castPath := filepath.Join(t.TempDir(), "session-simulate.cast")

	_, err = (&CastHandler{}).Execute(context.Background(), &schema.WorkflowStep{
		Name:  "demo",
		Type:  schema.TaskTypeCast,
		Mode:  "session",
		Shell: shell,
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
		Steps: []schema.WorkflowStep{
			{Type: schema.TaskTypeSimulate, Text: "# narration line", Rate: "0"},
			{Type: "write", Text: "printf ready", Rate: "0"},
			{Type: "key", Key: "enter"},
			{Type: "wait", Text: "ready", Timeout: "2s"},
		},
	}, NewVariables())
	if err != nil {
		t.Fatalf("execute session-mode cast: %v", err)
	}

	content, err := os.ReadFile(castPath)
	if err != nil {
		t.Fatalf("read cast file: %v", err)
	}
	text := castOutputText(t, content)
	if !strings.Contains(text, "# narration line") {
		t.Fatalf("expected narration text in cast output, got %q", text)
	}
	if !strings.Contains(text, "ready") {
		t.Fatalf("expected the real command's output in cast output, got %q", text)
	}
	// The narration line must carry an SGR (color) escape, matching
	// mode: steps' styled rendering -- not just plain, unstyled text.
	narrationIndex := strings.Index(text, "narration line")
	if narrationIndex == -1 || !strings.Contains(text[:narrationIndex], "\x1b[") {
		t.Fatalf("expected the narration line to be styled with an SGR escape, got %q", text)
	}
}

func TestRunCastBodyRejectsInvalidMode(t *testing.T) {
	err := runCastBody(context.Background(), &schema.WorkflowStep{Name: "demo", Mode: "bogus"}, NewVariables(), nil)
	if !errors.Is(err, ErrInvalidCastMode) {
		t.Fatalf("expected invalid cast mode, got %v", err)
	}
}

func TestRunCastSessionModeRejectsInvalidDurationsBeforeStartingSession(t *testing.T) {
	err := runCastSessionMode(context.Background(), &schema.WorkflowStep{WriteRate: "fast"}, NewVariables(), nil)
	if err == nil || !strings.Contains(err.Error(), "invalid duration") {
		t.Fatalf("expected invalid write rate, got %v", err)
	}
	err = runCastSessionMode(context.Background(), &schema.WorkflowStep{KeyInterval: "soon"}, NewVariables(), nil)
	if err == nil || !strings.Contains(err.Error(), "invalid duration") {
		t.Fatalf("expected invalid key interval, got %v", err)
	}
}

func TestRenderCastOutputs(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if err := renderCastOutputs(&schema.WorkflowStep{}, "input.cast"); err != nil {
		t.Fatalf("nil cast output should not render: %v", err)
	}

	err := renderCastOutputs(&schema.WorkflowStep{CastOutput: &schema.CastOutput{GIF: filepath.Join(t.TempDir(), "out.gif")}}, "input.cast")
	if !errors.Is(err, asciicast.ErrMissingAgg) {
		t.Fatalf("expected missing agg renderer, got %v", err)
	}
}

func TestStartStepRecorderDefaultsNilVariables(t *testing.T) {
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	rec, restore, err := startStepRecorder(&schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
	}, nil)
	if err != nil {
		t.Fatalf("start recorder with nil vars: %v", err)
	}
	restore()
	if err := rec.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}
	if _, err := os.Stat(castPath); err != nil {
		t.Fatalf("cast file missing: %v", err)
	}
}

func TestStartStepRecorderReturnsTitleResolutionError(t *testing.T) {
	_, _, err := startStepRecorder(&schema.WorkflowStep{
		Name:  "demo",
		Type:  schema.TaskTypeCast,
		Title: "{{ .Bad ",
	}, NewVariables())
	if err == nil {
		t.Fatal("expected an error resolving an invalid title template")
	}
}

func TestStartStepRecorderReturnsEnvResolutionError(t *testing.T) {
	_, _, err := startStepRecorder(&schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		Env: map[string]string{
			"BAD": "{{ .Bad ",
		},
	}, NewVariables())
	if err == nil {
		t.Fatal("expected an error resolving an invalid env template")
	}
}

func TestStartStepRecorderReturnsInvalidRateError(t *testing.T) {
	_, _, err := startStepRecorder(&schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		Rate: "bad-duration",
	}, NewVariables())
	if err == nil {
		t.Fatal("expected an error parsing an invalid output rate")
	}
}

func TestStartStepRecorderReturnsAsciicastStartError(t *testing.T) {
	// Making a path component of the cast directory an ordinary file (instead
	// of a directory) makes asciicast.Start's os.MkdirAll(filepath.Dir(path))
	// fail, driving startStepRecorder's final error return.
	blockingFile := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	castPath := filepath.Join(blockingFile, "nested", "demo.cast")

	_, _, err := startStepRecorder(&schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
	}, NewVariables())
	if err == nil {
		t.Fatal("expected asciicast.Start to fail when the cast directory cannot be created")
	}
}

func TestCastRecorderEnvReturnsResolutionError(t *testing.T) {
	_, err := castRecorderEnv(&schema.WorkflowStep{
		Name: "demo",
		Env: map[string]string{
			"BAD": "{{ .Bad ",
		},
	}, NewVariables())
	if err == nil {
		t.Fatal("expected an error resolving an invalid env template")
	}
}

func TestApplyCastStepEnvReturnsResolutionError(t *testing.T) {
	err := applyCastStepEnv(&schema.WorkflowStep{
		Name: "demo",
		Env: map[string]string{
			"BAD": "{{ .Bad ",
		},
	}, NewVariables())
	if err == nil {
		t.Fatal("expected an error resolving an invalid env template")
	}
}

func TestRunCastStepModeReturnsEnvResolutionError(t *testing.T) {
	err := runCastStepMode(context.Background(), &schema.WorkflowStep{
		Name: "demo",
		Env: map[string]string{
			"BAD": "{{ .Bad ",
		},
		Steps: []schema.WorkflowStep{{Type: schema.TaskTypeShell, Command: "true"}},
	}, NewVariables(), nil)
	if err == nil {
		t.Fatal("expected an error resolving an invalid cast step env template")
	}
}

func TestRunCastChildStepReturnsPauseDelayError(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	executor := NewStepExecutorWithVars(NewVariables())
	runner := castChildStepRunner{ctx: context.Background(), castStep: &schema.WorkflowStep{}, vars: NewVariables(), executor: executor}
	err := runner.run(&schema.WorkflowStep{
		Name:     "child",
		Type:     schema.TaskTypeAtmos,
		Output:   string(OutputModeNone),
		Command:  "terraform plan",
		Env:      map[string]string{"_ATMOS_STEP_FAKE": "ok"},
		Interval: "bad-duration",
	}, false)
	if err == nil {
		t.Fatal("expected an error parsing an invalid pause delay")
	}
}

func TestRunCastBodyDispatchesSessionMode(t *testing.T) {
	err := runCastBody(context.Background(), &schema.WorkflowStep{
		Name:        "demo",
		Mode:        "session",
		WriteRate:   "bad-duration",
		KeyInterval: "0",
	}, NewVariables(), nil)
	if err == nil || !strings.Contains(err.Error(), "invalid duration") {
		t.Fatalf("expected session mode to surface an invalid write rate, got %v", err)
	}
}

func TestRunCastSessionModeReturnsEnvResolutionError(t *testing.T) {
	err := runCastSessionMode(context.Background(), &schema.WorkflowStep{
		Env: map[string]string{
			"BAD": "{{ .Bad ",
		},
	}, NewVariables(), nil)
	if err == nil {
		t.Fatal("expected an error resolving an invalid session env template")
	}
}

func TestValidateWriteActionAcceptsValidRate(t *testing.T) {
	if err := validateWriteAction(&schema.WorkflowStep{Text: "echo hi", Rate: "5ms"}); err != nil {
		t.Fatalf("expected valid write action, got %v", err)
	}
}

func TestValidateKeyActionAcceptsEmptyAndValidInterval(t *testing.T) {
	if err := validateKeyAction(&schema.WorkflowStep{Key: "enter"}); err != nil {
		t.Fatalf("expected valid key action with no interval, got %v", err)
	}
	if err := validateKeyAction(&schema.WorkflowStep{Key: "enter", Interval: "5ms"}); err != nil {
		t.Fatalf("expected valid key action with interval, got %v", err)
	}
}

func TestValidatePauseActionAcceptsValidDuration(t *testing.T) {
	if err := validatePauseAction(&schema.WorkflowStep{Duration: "5ms"}); err != nil {
		t.Fatalf("expected valid pause action, got %v", err)
	}
}

func TestExecuteWithWorkflowReturnsStartRecorderError(t *testing.T) {
	_, err := (&CastHandler{}).ExecuteWithWorkflow(context.Background(), &schema.WorkflowStep{
		Name:  "demo",
		Type:  schema.TaskTypeCast,
		Title: "{{ .Bad ",
	}, NewVariables(), nil)
	if err == nil {
		t.Fatal("expected ExecuteWithWorkflow to surface a start-recorder error")
	}
}

func TestExecuteWithWorkflowClosesSuccessfully(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	result, err := (&CastHandler{}).ExecuteWithWorkflow(context.Background(), &schema.WorkflowStep{
		Name:   "demo",
		Type:   schema.TaskTypeCast,
		Output: string(OutputModeNone),
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
		Steps: []schema.WorkflowStep{{
			Type: schema.TaskTypeSimulate,
			Mode: "prompt",
		}},
	}, NewVariables(), nil)
	if err != nil {
		t.Fatalf("execute cast: %v", err)
	}
	if result.Value != castPath {
		t.Fatalf("result value = %q, want %q", result.Value, castPath)
	}
	if _, err := os.Stat(castPath); err != nil {
		t.Fatalf("cast file missing: %v", err)
	}
}

func TestExecuteWithWorkflowJoinsDiscardError(t *testing.T) {
	if runtime.GOOS == "windows" {
		// The recorder's temp cast file is still open (in this test process) when
		// the child subprocess below tries to remove it. Go's os.Create on Windows
		// doesn't grant FILE_SHARE_DELETE by default, so a file can't be deleted by
		// another handle/process while any handle keeps it open (unlike POSIX
		// unlink, which succeeds on an open file). The removal silently no-ops,
		// Discard's own os.Remove then succeeds normally, and no discard error ever
		// gets joined - so this scenario is only reproducible on POSIX platforms.
		t.Skip("cross-process deletion of an open file is not possible on Windows")
	}
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	tmpDir := t.TempDir()
	castPath := filepath.Join(tmpDir, "demo.cast")

	// The failing child step removes the recorder's own temp cast file (named
	// ".demo.cast.tmp-*" next to the final path, see asciicast.createCastTempFile)
	// before it exits non-zero. That makes ExecuteWithWorkflow's subsequent
	// rec.Discard() fail because the temp file is already gone, driving the
	// errors.Join(runErr, discardErr) branch.
	_, err := (&CastHandler{}).Execute(context.Background(), &schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
		Steps: []schema.WorkflowStep{
			{
				Name:    "boom",
				Type:    schema.TaskTypeAtmos,
				Output:  string(OutputModeNone),
				Command: "terraform plan",
				Env: map[string]string{
					"_ATMOS_STEP_FAKE":     "rm-glob-and-fail",
					atmosStepFakeRMGlobEnv: filepath.Join(tmpDir, ".demo.cast.tmp-*"),
				},
			},
		},
	}, NewVariables())
	if err == nil {
		t.Fatal("expected cast step failure")
	}
	if !strings.Contains(err.Error(), "no such file") && !strings.Contains(err.Error(), "cannot find") {
		t.Fatalf("expected discard error joined into the failure, got: %v", err)
	}
}

func TestExecuteWithWorkflowReturnsCloseError(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	tmpDir := t.TempDir()
	castPath := filepath.Join(tmpDir, "demo.cast")

	// Occupy the final cast path with a non-empty directory so that, once all
	// steps succeed, Recorder.Close's commit-by-rename fails, driving
	// ExecuteWithWorkflow's "runErr = closeErr" branch in the success path.
	if err := os.Mkdir(castPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(castPath, "occupied.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := (&CastHandler{}).Execute(context.Background(), &schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
		Steps: []schema.WorkflowStep{{
			Type: schema.TaskTypeSimulate,
			Mode: "prompt",
		}},
	}, NewVariables())
	if err == nil {
		t.Fatal("expected a close error when the cast path is occupied by a non-empty directory")
	}
}

func TestRunCastSessionModeExecutesScriptedActions(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	shell, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(sessionShellHelperEnv, "1")

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err = runCastSessionMode(ctx, &schema.WorkflowStep{
		Mode:  "session",
		Shell: shell,
		Steps: []schema.WorkflowStep{
			{Type: "write", Text: "printf ready", Rate: "0"},
			{Type: "key", Key: "enter"},
			{Type: "wait", Text: "ready", Timeout: "2s"},
		},
	}, NewVariables(), nil)
	if err != nil {
		t.Fatalf("runCastSessionMode error: %v", err)
	}
}

func TestCastHandlerExecutesSessionModeEndToEnd(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	shell, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(sessionShellHelperEnv, "1")
	castPath := filepath.Join(t.TempDir(), "session.cast")

	_, err = (&CastHandler{}).Execute(context.Background(), &schema.WorkflowStep{
		Name:  "demo",
		Type:  schema.TaskTypeCast,
		Mode:  "session",
		Shell: shell,
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
		Steps: []schema.WorkflowStep{
			{Type: "write", Text: "printf ready", Rate: "0"},
			{Type: "key", Key: "enter"},
			{Type: "wait", Text: "ready", Timeout: "2s"},
		},
	}, NewVariables())
	if err != nil {
		t.Fatalf("execute session-mode cast: %v", err)
	}
	if _, err := os.Stat(castPath); err != nil {
		t.Fatalf("cast file missing: %v", err)
	}
}

func TestCastHandlerSessionModeFallsThroughToRealExecution(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	shell, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(sessionShellHelperEnv, "1")
	castPath := filepath.Join(t.TempDir(), "session-exec.cast")

	_, err = (&CastHandler{}).Execute(context.Background(), &schema.WorkflowStep{
		Name:  "demo",
		Type:  schema.TaskTypeCast,
		Mode:  "session",
		Shell: shell,
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
		Steps: []schema.WorkflowStep{
			{Type: "write", Text: "printf ready", Rate: "0"},
			{Type: "key", Key: "enter"},
			{Type: "wait", Text: "ready", Timeout: "2s"},
			{
				Name:    "plan",
				Type:    schema.TaskTypeAtmos,
				Command: "terraform plan",
				Output:  string(OutputModeRaw),
				Env:     map[string]string{"_ATMOS_STEP_FAKE": "ok"},
			},
		},
	}, NewVariables())
	if err != nil {
		t.Fatalf("execute session-mode cast with real child: %v", err)
	}

	content, err := os.ReadFile(castPath)
	if err != nil {
		t.Fatalf("read cast file: %v", err)
	}
	if !strings.Contains(castOutputText(t, content), "fake-atmos-output") {
		t.Fatalf("expected real child output in cast")
	}
}

// TestCastHandlerNestedSessionStepFallsThroughToRealExecution is an
// end-to-end proof of the steps-mode `type: session` child: the cast step
// itself defaults to mode: steps (no top-level `mode: session` at all), the
// "session" child drives a real interactive PTY for its own nested actions,
// and once that block completes, control returns to the enclosing steps-mode
// loop, which runs an ordinary (non-PTY) `type: atmos` step -- proving a
// recording can open on real interactive prompts and then fall through to
// real, non-interactive command execution without that command ever running
// inside the PTY.
func TestCastHandlerNestedSessionStepFallsThroughToRealExecution(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	shell, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(sessionShellHelperEnv, "1")
	castPath := filepath.Join(t.TempDir(), "nested-session.cast")

	_, err = (&CastHandler{}).Execute(context.Background(), &schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
		Steps: []schema.WorkflowStep{
			{
				Type:  castSessionStepType,
				Shell: shell,
				Steps: []schema.WorkflowStep{
					{Type: "write", Text: "printf ready", Rate: "0"},
					{Type: "key", Key: "enter"},
					{Type: "wait", Text: "ready", Timeout: "2s"},
				},
			},
			{
				Name:    "plan",
				Type:    schema.TaskTypeAtmos,
				Command: "terraform plan",
				Output:  string(OutputModeRaw),
				Env:     map[string]string{"_ATMOS_STEP_FAKE": "ok"},
			},
		},
	}, NewVariables())
	if err != nil {
		t.Fatalf("execute nested-session cast: %v", err)
	}

	content, err := os.ReadFile(castPath)
	if err != nil {
		t.Fatalf("read cast file: %v", err)
	}
	text := castOutputText(t, content)
	if !strings.Contains(text, "ready") {
		t.Fatalf("expected the session block's real PTY output in cast output, got %q", text)
	}
	if !strings.Contains(text, "fake-atmos-output") {
		t.Fatalf("expected the real atmos step's output after the session block, got %q", text)
	}
}

func TestCastHandlerNestedSessionExecInheritsWorkflowOutput(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	shell, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(sessionShellHelperEnv, "1")
	castPath := filepath.Join(t.TempDir(), "nested-session-workflow.cast")

	_, err = (&CastHandler{}).ExecuteWithWorkflow(context.Background(), &schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
		Steps: []schema.WorkflowStep{{
			Type:  castSessionStepType,
			Shell: shell,
			Steps: []schema.WorkflowStep{
				{Type: "write", Text: "printf ready", Rate: "0"},
				{Type: "key", Key: "enter"},
				{Type: "wait", Text: "ready", Timeout: "2s"},
				{Name: "workflow-child", Type: schema.TaskTypeShell, Command: "printf workflow-child"},
			},
		}},
	}, NewVariables(), &schema.WorkflowDefinition{Output: string(OutputModeRaw)})
	if err != nil {
		t.Fatalf("execute nested session with workflow context: %v", err)
	}

	content, err := os.ReadFile(castPath)
	if err != nil {
		t.Fatalf("read cast file: %v", err)
	}
	if !strings.Contains(castOutputText(t, content), "workflow-child") {
		t.Fatalf("expected nested child output inherited from the workflow in cast")
	}
}

// TestRunCastSimulateStepSkipPromptOmitsPromptDraw verifies skipPrompt
// suppresses "typed" mode's own prompt render: a real shell's already-visible
// prompt (e.g. right after a `type: session` block exits) must not get a
// second, redundant prompt drawn on top of it by the next simulate action.
func TestRunCastSimulateStepSkipPromptOmitsPromptDraw(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	rec, restore, err := startStepRecorder(&schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
	}, NewVariables())
	if err != nil {
		t.Fatalf("start recorder: %v", err)
	}
	t.Cleanup(restore)

	prompt, err := renderCastPrompt(nil)
	if err != nil {
		t.Fatalf("render prompt: %v", err)
	}

	err = runCastSimulateStep(context.Background(), &schema.WorkflowStep{WriteRate: "0"}, &schema.WorkflowStep{
		Type: schema.TaskTypeSimulate,
		Mode: "typed",
		Text: "# narration after a session",
	}, NewVariables(), true)
	if err != nil {
		t.Fatalf("run simulate: %v", err)
	}
	restore()
	if err := rec.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}

	content, err := os.ReadFile(castPath)
	if err != nil {
		t.Fatalf("read cast: %v", err)
	}
	text := castOutputText(t, content)
	if !strings.Contains(text, "# narration after a session") {
		t.Fatalf("expected the narration text in cast output, got %q", text)
	}
	if strings.Contains(text, prompt) {
		t.Fatalf("expected no redundant prompt draw when skipPrompt is true, got %q", text)
	}
}

// TestRunCastStepModeSkipsPromptForSimulateAfterSessionBlock is an
// end-to-end regression test for the doubled-prompt bug: a `type: session`
// block's real shell always leaves its own prompt visible the moment the
// block exits, and the very next `type: simulate` narration line must not
// draw a second one on top of it (reported as "> > # comment" instead of
// "> # comment" in a recorded cast).
func TestRunCastStepModeSkipsPromptForSimulateAfterSessionBlock(t *testing.T) {
	if err := iolib.Initialize(); err != nil {
		t.Fatalf("initialize io: %v", err)
	}
	shell, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(sessionShellHelperEnv, "1")
	castPath := filepath.Join(t.TempDir(), "session-then-simulate.cast")

	_, err = (&CastHandler{}).Execute(context.Background(), &schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
		Steps: []schema.WorkflowStep{
			{
				Type:  castSessionStepType,
				Shell: shell,
				Steps: []schema.WorkflowStep{
					{Type: "write", Text: "printf ready", Rate: "0"},
					{Type: "key", Key: "enter"},
					{Type: "wait", Text: "ready", Timeout: "2s"},
				},
			},
			{Type: schema.TaskTypeSimulate, Text: "# narration after the session", Rate: "0"},
		},
	}, NewVariables())
	if err != nil {
		t.Fatalf("execute session-then-simulate cast: %v", err)
	}

	content, err := os.ReadFile(castPath)
	if err != nil {
		t.Fatalf("read cast file: %v", err)
	}
	text := castOutputText(t, content)
	if !strings.Contains(text, "# narration after the session") {
		t.Fatalf("expected the narration text in cast output, got %q", text)
	}
	// Two adjacent prompt renders are separated by ANSI reset/re-color escapes
	// (e.g. "...m> \x1b[0m\x1b[1;...m> ..."), so "> >" never appears as a raw
	// contiguous substring even when doubled -- check the ANSI-stripped text.
	if strings.Contains(ansi.Strip(text), "> >") {
		t.Fatalf("expected a single prompt at the session/simulate boundary, got a doubled prompt in %q", ansi.Strip(text))
	}
}

func castOutputText(t *testing.T, content []byte) string {
	t.Helper()
	var out strings.Builder
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	for _, line := range lines[1:] {
		var event []any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("parse cast event: %v", err)
		}
		if len(event) < 3 || event[1] != "o" {
			continue
		}
		text, ok := event[2].(string)
		if ok {
			out.WriteString(text)
		}
	}
	return out.String()
}

func castEventText(t *testing.T, content []byte) string {
	t.Helper()
	var out strings.Builder
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	for _, line := range lines[1:] {
		var event []any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("parse cast event: %v", err)
		}
		if len(event) < 3 {
			continue
		}
		text, ok := event[2].(string)
		if ok {
			out.WriteString(text)
		}
	}
	return out.String()
}
