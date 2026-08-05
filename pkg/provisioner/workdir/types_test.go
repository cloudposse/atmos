package workdir

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBuildPath covers BuildPath's instance-name resolution: it must prefer
// componentConfig's "atmos_component" (the per-instance name, used to keep
// inherited components isolated in distinct workdirs) over the shared
// component/metadata name, and fall back to component when no instance name
// is present.
func TestBuildPath(t *testing.T) {
	tests := []struct {
		name            string
		basePath        string
		componentType   string
		component       string
		stack           string
		componentConfig map[string]any
		want            []string
	}{
		{
			name:            "falls back to component when atmos_component is absent",
			basePath:        "/base",
			componentType:   "terraform",
			component:       "vpc",
			stack:           "dev",
			componentConfig: map[string]any{},
			want:            []string{"terraform", "dev-vpc"},
		},
		{
			name:          "prefers atmos_component instance name over shared component name",
			basePath:      "/base",
			componentType: "terraform",
			component:     "vpc",
			stack:         "dev",
			componentConfig: map[string]any{
				"atmos_component": "vpc-inherited-instance",
			},
			want: []string{"terraform", "dev-vpc-inherited-instance"},
		},
		{
			name:          "falls back to component when atmos_component is empty string",
			basePath:      "/base",
			componentType: "terraform",
			component:     "vpc",
			stack:         "dev",
			componentConfig: map[string]any{
				"atmos_component": "",
			},
			want: []string{"terraform", "dev-vpc"},
		},
		{
			name:          "falls back to component when atmos_component is not a string",
			basePath:      "/base",
			componentType: "terraform",
			component:     "vpc",
			stack:         "dev",
			componentConfig: map[string]any{
				"atmos_component": 123,
			},
			want: []string{"terraform", "dev-vpc"},
		},
		{
			name:            "nil componentConfig falls back to component",
			basePath:        "/base",
			componentType:   "helmfile",
			component:       "app",
			stack:           "prod",
			componentConfig: nil,
			want:            []string{"helmfile", "prod-app"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildPath(tt.basePath, tt.componentType, tt.component, tt.stack, tt.componentConfig)
			want := filepath.Join(append([]string{tt.basePath, WorkdirPath}, tt.want...)...)
			assert.Equal(t, want, got)
		})
	}
}
