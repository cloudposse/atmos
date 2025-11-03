package flags

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

func TestNewIdentitySelector(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		provided bool
		want     IdentitySelector
	}{
		{
			name:     "not provided",
			value:    "",
			provided: false,
			want:     IdentitySelector{value: "", provided: false},
		},
		{
			name:     "interactive selection",
			value:    cfg.IdentityFlagSelectValue,
			provided: true,
			want:     IdentitySelector{value: cfg.IdentityFlagSelectValue, provided: true},
		},
		{
			name:     "explicit identity",
			value:    "prod-admin",
			provided: true,
			want:     IdentitySelector{value: "prod-admin", provided: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewIdentitySelector(tt.value, tt.provided)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIdentitySelector_IsInteractiveSelector(t *testing.T) {
	tests := []struct {
		name     string
		selector IdentitySelector
		want     bool
	}{
		{
			name:     "not provided - not interactive",
			selector: NewIdentitySelector("", false),
			want:     false,
		},
		{
			name:     "empty value but provided - not interactive",
			selector: NewIdentitySelector("", true),
			want:     false,
		},
		{
			name:     "interactive selection value",
			selector: NewIdentitySelector(cfg.IdentityFlagSelectValue, true),
			want:     true,
		},
		{
			name:     "explicit identity - not interactive",
			selector: NewIdentitySelector("prod-admin", true),
			want:     false,
		},
		{
			name:     "interactive value but not provided - not interactive",
			selector: NewIdentitySelector(cfg.IdentityFlagSelectValue, false),
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.selector.IsInteractiveSelector()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIdentitySelector_Value(t *testing.T) {
	tests := []struct {
		name     string
		selector IdentitySelector
		want     string
	}{
		{
			name:     "not provided - empty value",
			selector: NewIdentitySelector("", false),
			want:     "",
		},
		{
			name:     "interactive selection - returns select value",
			selector: NewIdentitySelector(cfg.IdentityFlagSelectValue, true),
			want:     cfg.IdentityFlagSelectValue,
		},
		{
			name:     "explicit identity",
			selector: NewIdentitySelector("prod-admin", true),
			want:     "prod-admin",
		},
		{
			name:     "staging identity",
			selector: NewIdentitySelector("staging", true),
			want:     "staging",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.selector.Value()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIdentitySelector_IsEmpty(t *testing.T) {
	tests := []struct {
		name     string
		selector IdentitySelector
		want     bool
	}{
		{
			name:     "not provided - empty",
			selector: NewIdentitySelector("", false),
			want:     true,
		},
		{
			name:     "empty value but provided - empty",
			selector: NewIdentitySelector("", true),
			want:     true,
		},
		{
			name:     "interactive selection - not empty",
			selector: NewIdentitySelector(cfg.IdentityFlagSelectValue, true),
			want:     false,
		},
		{
			name:     "explicit identity - not empty",
			selector: NewIdentitySelector("prod-admin", true),
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.selector.IsEmpty()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIdentitySelector_IsProvided(t *testing.T) {
	tests := []struct {
		name     string
		selector IdentitySelector
		want     bool
	}{
		{
			name:     "not provided",
			selector: NewIdentitySelector("", false),
			want:     false,
		},
		{
			name:     "provided with empty value",
			selector: NewIdentitySelector("", true),
			want:     true,
		},
		{
			name:     "provided with interactive value",
			selector: NewIdentitySelector(cfg.IdentityFlagSelectValue, true),
			want:     true,
		},
		{
			name:     "provided with explicit value",
			selector: NewIdentitySelector("prod-admin", true),
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.selector.IsProvided()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIdentitySelector_UsageScenarios(t *testing.T) {
	tests := []struct {
		name            string
		selector        IdentitySelector
		wantInteractive bool
		wantEmpty       bool
		wantProvided    bool
		wantValue       string
		description     string
	}{
		{
			name:            "scenario 1: user did not provide --identity",
			selector:        NewIdentitySelector("", false),
			wantInteractive: false,
			wantEmpty:       true,
			wantProvided:    false,
			wantValue:       "",
			description:     "Should use default identity from config/env",
		},
		{
			name:            "scenario 2: user provided --identity (alone, no value)",
			selector:        NewIdentitySelector(cfg.IdentityFlagSelectValue, true),
			wantInteractive: true,
			wantEmpty:       false,
			wantProvided:    true,
			wantValue:       cfg.IdentityFlagSelectValue,
			description:     "Should trigger interactive selection",
		},
		{
			name:            "scenario 3: user provided --identity=prod-admin",
			selector:        NewIdentitySelector("prod-admin", true),
			wantInteractive: false,
			wantEmpty:       false,
			wantProvided:    true,
			wantValue:       "prod-admin",
			description:     "Should use the explicit identity",
		},
		{
			name:            "scenario 4: user provided --identity prod-admin (space form)",
			selector:        NewIdentitySelector("prod-admin", true),
			wantInteractive: false,
			wantEmpty:       false,
			wantProvided:    true,
			wantValue:       "prod-admin",
			description:     "Should use the explicit identity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Testing: %s", tt.description)

			assert.Equal(t, tt.wantInteractive, tt.selector.IsInteractiveSelector(), "IsInteractiveSelector()")
			assert.Equal(t, tt.wantEmpty, tt.selector.IsEmpty(), "IsEmpty()")
			assert.Equal(t, tt.wantProvided, tt.selector.IsProvided(), "IsProvided()")
			assert.Equal(t, tt.wantValue, tt.selector.Value(), "Value()")
		})
	}
}
