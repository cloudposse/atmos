package step

import (
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
