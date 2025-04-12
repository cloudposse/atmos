package config

import (
	"reflect"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGetContextFromVars(t *testing.T) {
	tests := []struct {
		name     string
		vars     map[string]any
		expected schema.Context
	}{
		{
			name: "all fields set correctly",
			vars: map[string]any{
				"namespace":   "ns",
				"tenant":      "tn",
				"environment": "env",
				"stage":       "st",
				"region":      "us-west-1",
				"attributes":  []any{"attr1", "attr2"},
			},
			expected: schema.Context{
				Namespace:   "ns",
				Tenant:      "tn",
				Environment: "env",
				Stage:       "st",
				Region:      "us-west-1",
				Attributes:  []any{"attr1", "attr2"},
			},
		},
		{
			name: "missing all fields",
			vars: map[string]any{},
			expected: schema.Context{
				Namespace:   "",
				Tenant:      "",
				Environment: "",
				Stage:       "",
				Region:      "",
				Attributes:  nil,
			},
		},
		{
			name: "invalid types for all fields",
			vars: map[string]any{
				"namespace":   123,
				"tenant":      true,
				"environment": []string{"not", "a", "string"},
				"stage":       nil,
				"region":      struct{}{},
				"attributes":  "not-a-slice",
			},
			expected: schema.Context{
				Namespace:   "",
				Tenant:      "",
				Environment: "",
				Stage:       "",
				Region:      "",
				Attributes:  nil,
			},
		},
		{
			name: "some fields missing or wrong",
			vars: map[string]any{
				"namespace":  "prod",
				"tenant":     42, // invalid
				"region":     "eu-central-1",
				"attributes": []any{},
			},
			expected: schema.Context{
				Namespace:   "prod",
				Tenant:      "",
				Environment: "",
				Stage:       "",
				Region:      "eu-central-1",
				Attributes:  []any{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetContextFromVars(tt.vars)
			if !reflect.DeepEqual(got, tt.expected) {
				t.Errorf("GetContextFromVars() = %+v, want %+v", got, tt.expected)
			}
		})
	}
}
