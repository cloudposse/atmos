package exec

import (
	"testing"
)

var componentName = "test-component"

func TestIsComponentEnabled(t *testing.T) {
	tests := []struct {
		name           string
		componentAttrs map[string]any
		want           bool
	}{
		{
			name: "explicitly enabled component",
			componentAttrs: map[string]any{
				"enabled": true,
			},
			want: true,
		},
		{
			name: "explicitly disabled component",
			componentAttrs: map[string]any{
				"enabled": false,
			},
			want: false,
		},
		{
			name: "component with string true",
			componentAttrs: map[string]any{
				"enabled": "true",
			},
			want: true,
		},
		{
			name: "component with number 1",
			componentAttrs: map[string]any{
				"enabled": 1,
			},
			want: true,
		},
		{
			name:           "component with nil attributes",
			componentAttrs: nil,
			want:           true,
		},
		{
			name:           "component with empty attributes",
			componentAttrs: map[string]any{},
			want:           true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isComponentEnabled(tt.componentAttrs, componentName)
			if got != tt.want {
				t.Errorf("isComponentEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsComponentEnabled_CaseSensitivity(t *testing.T) {
	tests := []struct {
		name           string
		componentAttrs map[string]any
		want           bool
	}{
		{
			name: "uppercase ENABLED",
			componentAttrs: map[string]any{
				"ENABLED": true,
			},
			want: true,
		},
		{
			name: "mixed case EnAbLeD",
			componentAttrs: map[string]any{
				"EnAbLeD": true,
			},
			want: true,
		},
		{
			name: "both cases present",
			componentAttrs: map[string]any{
				"enabled": false,
				"ENABLED": true,
			},
			want: false, // Should use the exact "enabled" key if present
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isComponentEnabled(tt.componentAttrs, componentName)
			if got != tt.want {
				t.Errorf("isComponentEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsComponentLocked(t *testing.T) {
	tests := []struct {
		name           string
		componentAttrs map[string]any
		want           bool
	}{
		{
			name: "explicitly locked component",
			componentAttrs: map[string]any{
				"locked": true,
			},
			want: true,
		},
		{
			name: "explicitly unlocked component",
			componentAttrs: map[string]any{
				"locked": false,
			},
			want: false,
		},
		{
			name: "component with string true",
			componentAttrs: map[string]any{
				"locked": "true",
			},
			want: false,
		},
		{
			name: "component with number 1",
			componentAttrs: map[string]any{
				"locked": 1,
			},
			want: false,
		},
		{
			name:           "component with nil attributes",
			componentAttrs: nil,
			want:           false,
		},
		{
			name:           "component with empty attributes",
			componentAttrs: map[string]any{},
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isComponentLocked(tt.componentAttrs)
			if got != tt.want {
				t.Errorf("isComponentEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
