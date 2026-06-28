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

func TestBuildComponentYqPath(t *testing.T) {
	tests := []struct {
		name              string
		ctype, cname, rel string
		want              string
	}{
		// Simple names: identical to the raw provenance path (no quoting).
		{"simple", "terraform", "vpc", "vars.region", "components.terraform.vpc.vars.region"},
		{"no rel", "terraform", "vpc", "", "components.terraform.vpc"},
		// A slash is not a path separator, but it is not a simple identifier
		// either, so the segment is quoted (consistent with DotPathToYqPath, and
		// yq addresses it the same way as the unquoted form).
		{"slash name", "terraform", "vpc/prod", "vars.region", `components.terraform."vpc/prod".vars.region`},
		// Special names with literal dots/brackets are quoted so they address the
		// intended manifest node instead of being parsed as nested path syntax.
		{"dotted name", "terraform", "vpc.prod", "vars.region", `components.terraform."vpc.prod".vars.region`},
		{"bracket name", "terraform", "foo[0]", "vars.region", `components.terraform."foo[0]".vars.region`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, BuildComponentYqPath(tt.ctype, tt.cname, tt.rel))
		})
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
