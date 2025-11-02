package flagparser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewPagerSelector(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		provided bool
		want     PagerSelector
	}{
		{
			name:     "not provided",
			value:    "",
			provided: false,
			want:     PagerSelector{value: "", provided: false},
		},
		{
			name:     "enabled with default pager",
			value:    "true",
			provided: true,
			want:     PagerSelector{value: "true", provided: true},
		},
		{
			name:     "explicit pager command",
			value:    "less",
			provided: true,
			want:     PagerSelector{value: "less", provided: true},
		},
		{
			name:     "disabled",
			value:    "false",
			provided: true,
			want:     PagerSelector{value: "false", provided: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NewPagerSelector(tt.value, tt.provided)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPagerSelector_IsEnabled(t *testing.T) {
	tests := []struct {
		name     string
		selector PagerSelector
		want     bool
	}{
		{
			name:     "not provided - disabled",
			selector: NewPagerSelector("", false),
			want:     false,
		},
		{
			name:     "explicitly disabled",
			selector: NewPagerSelector("false", true),
			want:     false,
		},
		{
			name:     "enabled with default (true)",
			selector: NewPagerSelector("true", true),
			want:     true,
		},
		{
			name:     "enabled with empty value",
			selector: NewPagerSelector("", true),
			want:     true,
		},
		{
			name:     "enabled with specific pager",
			selector: NewPagerSelector("less", true),
			want:     true,
		},
		{
			name:     "enabled with more pager",
			selector: NewPagerSelector("more", true),
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.selector.IsEnabled()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPagerSelector_Pager(t *testing.T) {
	tests := []struct {
		name     string
		selector PagerSelector
		want     string
	}{
		{
			name:     "not provided - empty",
			selector: NewPagerSelector("", false),
			want:     "",
		},
		{
			name:     "true - use default pager",
			selector: NewPagerSelector("true", true),
			want:     "",
		},
		{
			name:     "false - disabled",
			selector: NewPagerSelector("false", true),
			want:     "",
		},
		{
			name:     "empty string - use default pager",
			selector: NewPagerSelector("", true),
			want:     "",
		},
		{
			name:     "less - explicit pager",
			selector: NewPagerSelector("less", true),
			want:     "less",
		},
		{
			name:     "more - explicit pager",
			selector: NewPagerSelector("more", true),
			want:     "more",
		},
		{
			name:     "custom pager command",
			selector: NewPagerSelector("bat --paging=always", true),
			want:     "bat --paging=always",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.selector.Pager()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPagerSelector_IsProvided(t *testing.T) {
	tests := []struct {
		name     string
		selector PagerSelector
		want     bool
	}{
		{
			name:     "not provided",
			selector: NewPagerSelector("", false),
			want:     false,
		},
		{
			name:     "provided with empty value",
			selector: NewPagerSelector("", true),
			want:     true,
		},
		{
			name:     "provided with true",
			selector: NewPagerSelector("true", true),
			want:     true,
		},
		{
			name:     "provided with false",
			selector: NewPagerSelector("false", true),
			want:     true,
		},
		{
			name:     "provided with specific pager",
			selector: NewPagerSelector("less", true),
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

func TestPagerSelector_Value(t *testing.T) {
	tests := []struct {
		name     string
		selector PagerSelector
		want     string
	}{
		{
			name:     "not provided",
			selector: NewPagerSelector("", false),
			want:     "",
		},
		{
			name:     "true value",
			selector: NewPagerSelector("true", true),
			want:     "true",
		},
		{
			name:     "false value",
			selector: NewPagerSelector("false", true),
			want:     "false",
		},
		{
			name:     "less value",
			selector: NewPagerSelector("less", true),
			want:     "less",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.selector.Value()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPagerSelector_UsageScenarios(t *testing.T) {
	tests := []struct {
		name         string
		selector     PagerSelector
		wantEnabled  bool
		wantProvided bool
		wantPager    string
		wantValue    string
		description  string
	}{
		{
			name:         "scenario 1: user did not provide --pager",
			selector:     NewPagerSelector("", false),
			wantEnabled:  false,
			wantProvided: false,
			wantPager:    "",
			wantValue:    "",
			description:  "Should use config default",
		},
		{
			name:         "scenario 2: user provided --pager (alone, no value)",
			selector:     NewPagerSelector("true", true),
			wantEnabled:  true,
			wantProvided: true,
			wantPager:    "",
			wantValue:    "true",
			description:  "Should enable with default pager",
		},
		{
			name:         "scenario 3: user provided --pager=false",
			selector:     NewPagerSelector("false", true),
			wantEnabled:  false,
			wantProvided: true,
			wantPager:    "",
			wantValue:    "false",
			description:  "Should disable pager",
		},
		{
			name:         "scenario 4: user provided --pager=less",
			selector:     NewPagerSelector("less", true),
			wantEnabled:  true,
			wantProvided: true,
			wantPager:    "less",
			wantValue:    "less",
			description:  "Should use less pager",
		},
		{
			name:         "scenario 5: user provided --pager=more",
			selector:     NewPagerSelector("more", true),
			wantEnabled:  true,
			wantProvided: true,
			wantPager:    "more",
			wantValue:    "more",
			description:  "Should use more pager",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Testing: %s", tt.description)

			assert.Equal(t, tt.wantEnabled, tt.selector.IsEnabled(), "IsEnabled()")
			assert.Equal(t, tt.wantProvided, tt.selector.IsProvided(), "IsProvided()")
			assert.Equal(t, tt.wantPager, tt.selector.Pager(), "Pager()")
			assert.Equal(t, tt.wantValue, tt.selector.Value(), "Value()")
		})
	}
}
