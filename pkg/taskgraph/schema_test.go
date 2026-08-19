package taskgraph

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

// TestRefsFromDependencies covers RefsFromDependencies' conversion of schema.Dependencies'
// Commands/Workflows entries into taskgraph.Ref values, across empty, single, and mixed inputs.
func TestRefsFromDependencies(t *testing.T) {
	tests := []struct {
		name string
		deps schema.Dependencies
		want []Ref
	}{
		{
			name: "empty dependencies produce no refs",
			deps: schema.Dependencies{},
			want: []Ref{},
		},
		{
			name: "commands convert to KindCommand refs, preserving flags/args/fail",
			deps: schema.Dependencies{
				Commands: schema.UnitDependencies{
					{Name: "build"},
					{
						Name:  "lint",
						Flags: map[string]string{"env": "prod"},
						Args:  []string{"--verbose"},
						Fail:  FailBestEffort,
					},
				},
			},
			want: []Ref{
				{Kind: KindCommand, Name: "build"},
				{
					Kind:  KindCommand,
					Name:  "lint",
					Flags: map[string]string{"env": "prod"},
					Args:  []string{"--verbose"},
					Fail:  FailBestEffort,
				},
			},
		},
		{
			name: "workflows convert to KindWorkflow refs, preserving file, and are appended after commands",
			deps: schema.Dependencies{
				Commands:  schema.UnitDependencies{{Name: "build"}},
				Workflows: schema.UnitDependencies{{Name: "deploy", File: "deploy.yaml"}},
			},
			want: []Ref{
				{Kind: KindCommand, Name: "build"},
				{Kind: KindWorkflow, Name: "deploy", File: "deploy.yaml"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RefsFromDependencies(tt.deps)
			assert.Equal(t, tt.want, got)
		})
	}
}
