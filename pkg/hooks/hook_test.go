package hooks

import (
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
			name:     "does not match when events list is empty",
			hook:     Hook{Events: []string{}},
			event:    AfterTerraformApply,
			expected: false,
		},
		{
			name:     "does not match when events list is nil",
			hook:     Hook{Events: nil},
			event:    AfterTerraformApply,
			expected: false,
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.hook.MatchesEvent(tt.event))
		})
	}
}
