package stack

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/merge"
)

func TestBuildComponentInFilePath(t *testing.T) {
	tests := []struct {
		ctype, name, rel, want string
	}{
		{"terraform", "vpc", "vars.region", "components.terraform.vpc.vars.region"},
		{"helmfile", "nginx", "vars.replicas", "components.helmfile.nginx.vars.replicas"},
		{"terraform", "vpc/prod", "settings.spacelift.workspace_enabled", "components.terraform.vpc/prod.settings.spacelift.workspace_enabled"},
		{"terraform", "vpc", "", "components.terraform.vpc"},
	}
	for _, tt := range tests {
		got := BuildComponentInFilePath(tt.ctype, tt.name, tt.rel)
		assert.Equal(t, tt.want, got)
	}
}

func TestPickProvenanceFile(t *testing.T) {
	// Last entry wins (effective value after deep-merge).
	entries := []merge.ProvenanceEntry{
		{File: "catalog/vpc.yaml", Line: 5},
		{File: "stacks/prod.yaml", Line: 12},
	}
	file, line, ok := PickProvenanceFile(entries)
	assert.True(t, ok)
	assert.Equal(t, "stacks/prod.yaml", file)
	assert.Equal(t, 12, line)
}

func TestPickProvenanceFile_Empty(t *testing.T) {
	_, _, ok := PickProvenanceFile(nil)
	assert.False(t, ok)
}
