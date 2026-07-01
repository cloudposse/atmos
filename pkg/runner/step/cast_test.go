package step

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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
