package step

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudposse/atmos/pkg/asciicast"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
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

func TestCastRecorderUsesStepCommandInHeader(t *testing.T) {
	castPath := filepath.Join(t.TempDir(), "demo.cast")
	rec, restore, err := startStepRecorder(&schema.WorkflowStep{
		Name:    "terraform-deploy",
		Type:    schema.TaskTypeCast,
		Command: "atmos terraform deploy vpc -s dev -auto-approve",
		CastOutput: &schema.CastOutput{
			Cast: castPath,
		},
	})
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
		Command string `json:"command"`
	}
	if err := json.Unmarshal(headerLine, &header); err != nil {
		t.Fatalf("parse header: %v", err)
	}
	if header.Command != "atmos terraform deploy vpc -s dev -auto-approve" {
		t.Fatalf("header command = %q", header.Command)
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
	})
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
	})
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
	})
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
	})
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
	}, vars)
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
	svgPath := filepath.Join(t.TempDir(), "demo.svg")

	result, err := (&CastHandler{}).Execute(context.Background(), &schema.WorkflowStep{
		Name: "demo",
		Type: schema.TaskTypeCast,
		CastOutput: &schema.CastOutput{
			Cast: castPath,
			SVG:  svgPath,
		},
		Steps: []schema.WorkflowStep{{
			Type: schema.TaskTypeSimulate,
			Mode: "prompt",
		}},
	}, NewVariables())
	if !errors.Is(err, asciicast.ErrMissingAgg) {
		t.Fatalf("expected render error, got %v", err)
	}
	if result == nil || result.Metadata["cast"] != castPath || result.Metadata["svg"] != svgPath {
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
	if got, err := parseDurationDefault("", time.Second); err != nil || got != time.Second {
		t.Fatalf("default duration = %s err=%v", got, err)
	}
	if got, err := parseDurationDefault("0", time.Second); err != nil || got != 0 {
		t.Fatalf("zero duration = %s err=%v", got, err)
	}
	if _, err := parseDurationDefault("invalid", 0); err == nil {
		t.Fatal("expected invalid duration error")
	}
	if delay := castStepPauseDelay(&schema.WorkflowStep{Interval: "bad"}); delay != defaultCastStepPauseDelay {
		t.Fatalf("fallback pause delay = %s", delay)
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
		{name: "typed duration invalid", step: schema.WorkflowStep{Mode: "typed", Text: "x", Duration: "bad"}, expectErr: true},
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
	actions := castSessionActions([]schema.WorkflowStep{{
		Type:     "write",
		Text:     "echo hi",
		Regex:    "hi",
		Key:      "enter",
		Duration: "1s",
		Timeout:  "2s",
		Rate:     "3ms",
		Interval: "4ms",
		Repeat:   2,
	}})
	if len(actions) != 1 {
		t.Fatalf("action count = %d", len(actions))
	}
	want := asciicast.SessionAction{Type: "write", Text: "echo hi", Regex: "hi", Key: "enter", Duration: "1s", Timeout: "2s", Rate: "3ms", Interval: "4ms", Repeat: 2}
	if actions[0] != want {
		t.Fatalf("action = %#v, want %#v", actions[0], want)
	}

	if got := castCommandArgs(&schema.WorkflowStep{Name: "demo"}); strings.Join(got, " ") != "cast demo" {
		t.Fatalf("default command args = %v", got)
	}
	if got := castCommandArgs(&schema.WorkflowStep{Command: "atmos terraform plan vpc"}); strings.Join(got, " ") != "atmos terraform plan vpc" {
		t.Fatalf("explicit command args = %v", got)
	}
	if mode := castMode(&schema.WorkflowStep{}); mode != "steps" {
		t.Fatalf("default cast mode = %q", mode)
	}
}

func TestRunCastBodyRejectsInvalidMode(t *testing.T) {
	err := runCastBody(context.Background(), &schema.WorkflowStep{Name: "demo", Mode: "bogus"}, NewVariables(), nil)
	if !errors.Is(err, ErrInvalidCastMode) {
		t.Fatalf("expected invalid cast mode, got %v", err)
	}
}

func TestRunCastSessionModeRejectsInvalidDurationsBeforeStartingSession(t *testing.T) {
	err := runCastSessionMode(context.Background(), &schema.WorkflowStep{WriteRate: "fast"})
	if err == nil || !strings.Contains(err.Error(), "invalid duration") {
		t.Fatalf("expected invalid write rate, got %v", err)
	}
	err = runCastSessionMode(context.Background(), &schema.WorkflowStep{KeyInterval: "soon"})
	if err == nil || !strings.Contains(err.Error(), "invalid duration") {
		t.Fatalf("expected invalid key interval, got %v", err)
	}
}

func TestRenderCastOutputs(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if err := renderCastOutputs(&schema.WorkflowStep{}, "input.cast"); err != nil {
		t.Fatalf("nil cast output should not render: %v", err)
	}

	err := renderCastOutputs(&schema.WorkflowStep{CastOutput: &schema.CastOutput{SVG: filepath.Join(t.TempDir(), "out.svg")}}, "input.cast")
	if !errors.Is(err, asciicast.ErrMissingAgg) {
		t.Fatalf("expected missing agg, got %v", err)
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
