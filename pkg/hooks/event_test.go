package hooks

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHookEvent_IsPostExecution(t *testing.T) {
	tests := []struct {
		name  string
		event HookEvent
		want  bool
	}{
		{name: "before-init is pre-execution", event: BeforeTerraformInit, want: false},
		{name: "after-init is post-execution", event: AfterTerraformInit, want: true},
		{name: "before-plan is pre-execution", event: BeforeTerraformPlan, want: false},
		{name: "after-plan is post-execution", event: AfterTerraformPlan, want: true},
		{name: "after-apply is post-execution", event: AfterTerraformApply, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.event.IsPostExecution())
		})
	}
}

func TestNormalizeEvent(t *testing.T) {
	cases := []struct {
		input string
		want  HookEvent
		desc  string
	}{
		{"after-terraform-apply", AfterTerraformApply, "dashed terraform form normalizes to dotted"},
		{"after.terraform.apply", AfterTerraformApply, "dotted terraform form passes through"},
		{"  after-terraform-apply  ", AfterTerraformApply, "trims whitespace"},
		{"before-terraform-plan", BeforeTerraformPlan, "dashed before event"},
		{"after-agent-apply", HookEvent("after.agent.apply"), "custom component event, dashed"},
		{"after.agent.apply", HookEvent("after.agent.apply"), "custom component event, dotted"},
		{"after-script-deploy-app", HookEvent("after.script.deploy.app"), "dashes in subcommand normalize consistently"},
		{"unknown-event", HookEvent("unknown.event"), "unknown events pass through normalized"},
	}

	for _, tc := range cases {
		t.Run(tc.desc, func(t *testing.T) {
			assert.Equal(t, tc.want, NormalizeEvent(tc.input))
		})
	}
}

func TestHookEvent_Normalize_InitNotAliased(t *testing.T) {
	// init events have no deploy/apply alias — they normalize to themselves.
	assert.Equal(t, BeforeTerraformInit, BeforeTerraformInit.Normalize())
	assert.Equal(t, AfterTerraformInit, AfterTerraformInit.Normalize())
}

func TestComponentEvent(t *testing.T) {
	cases := []struct {
		phase, kind, sub string
		want             HookEvent
	}{
		{PhaseAfter, "agent", "apply", "after.agent.apply"},
		{PhaseBefore, "agent", "apply", "before.agent.apply"},
		{PhaseAfter, "agent", "describe", "after.agent.describe"},
		{PhaseAfter, "script", "deploy-app", "after.script.deploy-app"},
	}
	for _, tc := range cases {
		t.Run(string(tc.want), func(t *testing.T) {
			assert.Equal(t, tc.want, ComponentEvent(tc.phase, tc.kind, tc.sub))
		})
	}
}

func TestHookMatchesEvent(t *testing.T) {
	cases := []struct {
		name        string
		hookEvents  []string
		firedEvent  HookEvent
		shouldMatch bool
	}{
		{"matches dotted terraform name", []string{"after.terraform.apply"}, AfterTerraformApply, true},
		{"matches dashed terraform alias", []string{"after-terraform-apply"}, AfterTerraformApply, true},
		{"matches custom component event", []string{"after-agent-apply"}, ComponentEvent(PhaseAfter, "agent", "apply"), true},
		{
			"describe does not match apply subscriber",
			[]string{"after-agent-apply"},
			ComponentEvent(PhaseAfter, "agent", "describe"),
			false,
		},
		{"mismatched event does not fire", []string{"after-terraform-plan"}, AfterTerraformApply, false},
		{
			"multiple events, one matches",
			[]string{"before-terraform-plan", "after-agent-apply"},
			ComponentEvent(PhaseAfter, "agent", "apply"),
			true,
		},
		{"empty events list matches all (back-compat)", []string{}, AfterTerraformApply, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := Hook{Events: tc.hookEvents}
			assert.Equal(t, tc.shouldMatch, h.MatchesEvent(tc.firedEvent))
		})
	}
}
