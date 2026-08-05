package hooks

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHook_MatchesEvent(t *testing.T) {
	tests := []struct {
		name     string
		hook     Hook
		event    HookEvent
		expected bool
	}{
		{
			name:     "matches hyphenated yaml format against dot constant",
			hook:     Hook{Events: []string{"after-terraform-apply"}},
			event:    AfterTerraformApply,
			expected: true,
		},
		{
			name:     "matches dot format stored in yaml",
			hook:     Hook{Events: []string{"after.terraform.apply"}},
			event:    AfterTerraformApply,
			expected: true,
		},
		{
			name:     "does not match different event",
			hook:     Hook{Events: []string{"after-terraform-apply"}},
			event:    BeforeTerraformApply,
			expected: false,
		},
		{
			name:     "matches all events when events list is empty (backward compat)",
			hook:     Hook{Events: []string{}},
			event:    AfterTerraformApply,
			expected: true,
		},
		{
			name:     "matches all events when events list is nil (backward compat)",
			hook:     Hook{Events: nil},
			event:    AfterTerraformApply,
			expected: true,
		},
		{
			name:     "matches one of multiple events",
			hook:     Hook{Events: []string{"before-terraform-plan", "after-terraform-apply"}},
			event:    AfterTerraformApply,
			expected: true,
		},
		{
			name:     "does not match when none of multiple events match",
			hook:     Hook{Events: []string{"before-terraform-plan", "before-terraform-apply"}},
			event:    AfterTerraformApply,
			expected: false,
		},
		{
			name:     "matches before-terraform-apply event",
			hook:     Hook{Events: []string{"before-terraform-apply"}},
			event:    BeforeTerraformApply,
			expected: true,
		},
		{
			name:     "matches before-terraform-init event",
			hook:     Hook{Events: []string{"before-terraform-init"}},
			event:    BeforeTerraformInit,
			expected: true,
		},
		{
			name:     "matches after-terraform-init event",
			hook:     Hook{Events: []string{"after-terraform-init"}},
			event:    AfterTerraformInit,
			expected: true,
		},
		{
			name:     "before-terraform-init hook does not fire on after-terraform-init",
			hook:     Hook{Events: []string{"before-terraform-init"}},
			event:    AfterTerraformInit,
			expected: false,
		},
		// deploy and apply are aliases — hooks configured for either fire on both commands.
		{
			name:     "after-terraform-apply hook fires on deploy command",
			hook:     Hook{Events: []string{"after-terraform-apply"}},
			event:    AfterTerraformDeploy,
			expected: true,
		},
		{
			name:     "after-terraform-deploy hook fires on apply command",
			hook:     Hook{Events: []string{"after-terraform-deploy"}},
			event:    AfterTerraformApply,
			expected: true,
		},
		{
			name:     "before-terraform-apply hook fires on deploy command",
			hook:     Hook{Events: []string{"before-terraform-apply"}},
			event:    BeforeTerraformDeploy,
			expected: true,
		},
		{
			name:     "before-terraform-deploy hook fires on apply command",
			hook:     Hook{Events: []string{"before-terraform-deploy"}},
			event:    BeforeTerraformApply,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.hook.MatchesEvent(tt.event))
		})
	}
}

func TestHook_MatchesAllTerraformEventNotations(t *testing.T) {
	events := []HookEvent{
		BeforeTerraformInit,
		AfterTerraformInit,
		BeforeTerraformPlan,
		AfterTerraformPlan,
		AfterTerraformPlanAggregate,
		BeforeTerraformApply,
		AfterTerraformApply,
		AfterTerraformApplyAggregate,
		BeforeTerraformDeploy,
		AfterTerraformDeploy,
		AfterTerraformDestroyAggregate,
	}

	for _, event := range events {
		canonical := string(event)
		hyphenated := strings.ReplaceAll(canonical, ".", "-")

		t.Run(canonical+"/dotted", func(t *testing.T) {
			assert.True(t, Hook{Events: []string{canonical}}.MatchesEvent(event))
		})

		t.Run(canonical+"/hyphenated", func(t *testing.T) {
			assert.True(t, Hook{Events: []string{hyphenated}}.MatchesEvent(event))
		})
	}
}

func TestHook_MatchesDeployApplyAliasesInBothNotations(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		event      HookEvent
	}{
		{name: "dotted apply matches deploy", configured: "after.terraform.apply", event: AfterTerraformDeploy},
		{name: "hyphenated apply matches deploy", configured: "after-terraform-apply", event: AfterTerraformDeploy},
		{name: "dotted deploy matches apply", configured: "after.terraform.deploy", event: AfterTerraformApply},
		{name: "hyphenated deploy matches apply", configured: "after-terraform-deploy", event: AfterTerraformApply},
		{name: "dotted before apply matches before deploy", configured: "before.terraform.apply", event: BeforeTerraformDeploy},
		{name: "hyphenated before apply matches before deploy", configured: "before-terraform-apply", event: BeforeTerraformDeploy},
		{name: "dotted before deploy matches before apply", configured: "before.terraform.deploy", event: BeforeTerraformApply},
		{name: "hyphenated before deploy matches before apply", configured: "before-terraform-deploy", event: BeforeTerraformApply},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, Hook{Events: []string{tt.configured}}.MatchesEvent(tt.event))
		})
	}
}
