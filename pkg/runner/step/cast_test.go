package step

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	castStep := &schema.WorkflowStep{
		Name: "demo",
		Env: map[string]string{
			"ATMOS_FORCE_COLOR": "1",
			"PATH":              "/tmp/bin:{{ .env.PATH }}",
		},
	}

	if err := applyCastStepEnv(castStep, vars); err != nil {
		t.Fatalf("applyCastStepEnv error: %v", err)
	}

	if vars.Env["ATMOS_FORCE_COLOR"] != "1" {
		t.Fatalf("ATMOS_FORCE_COLOR = %q, want 1", vars.Env["ATMOS_FORCE_COLOR"])
	}
	if !strings.HasPrefix(vars.Env["PATH"], "/tmp/bin:") {
		t.Fatalf("PATH = %q, want /tmp/bin prefix", vars.Env["PATH"])
	}
}

func TestCastStepInputLinesIncludesTextAndCommand(t *testing.T) {
	vars := NewVariables()
	vars.SetEnv("STACK", "dev")
	child := &schema.WorkflowStep{
		Text:    "# inspect the quick start stack\n# then run the plan",
		Command: "atmos terraform plan station -s {{ .env.STACK }}",
	}

	lines, err := castStepInputLines(child, vars)
	if err != nil {
		t.Fatalf("castStepInputLines error: %v", err)
	}

	want := []string{
		"# inspect the quick start stack",
		"# then run the plan",
		"atmos terraform plan station -s dev",
	}
	if len(lines) != len(want) {
		t.Fatalf("lines = %#v, want %#v", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Fatalf("lines[%d] = %q, want %q", i, lines[i], want[i])
		}
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

	if err := recordCastTypedLine(context.Background(), "# inspect", 0, 0); err != nil {
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

	if err := recordCastPrompt(); err != nil {
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
