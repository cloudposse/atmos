package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard: rename of any field below must fail the build, since the
// tests in this file rely on these schema.Affected fields by name.
var _ = schema.Affected{
	Component:     "sentinel",
	Stack:         "sentinel",
	ComponentType: cfg.TerraformComponentType,
	Deleted:       true,
	Affected:      "sentinel",
}

// TestFilterTerraformAffected verifies that `atmos terraform plan/apply --affected`
// only executes against terraform components that still exist in HEAD. Helmfile,
// Packer, and deleted components must be dropped — they are the surface of bug #2361
// where users saw `atmos terraform apply example-helmfile` and
// `atmos terraform apply example-packer` lines appear in the dry-run output.
func TestFilterTerraformAffected(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []schema.Affected
		want []schema.Affected
	}{
		{
			name: "nil input returns nil",
			in:   nil,
			want: nil,
		},
		{
			name: "empty input returns empty",
			in:   []schema.Affected{},
			want: []schema.Affected{},
		},
		{
			name: "mixed types: helmfile and packer dropped, terraform kept",
			in: []schema.Affected{
				{Component: "example-terraform", Stack: "example", ComponentType: cfg.TerraformComponentType},
				{Component: "example-helmfile", Stack: "example", ComponentType: cfg.HelmfileComponentType},
				{Component: "example-packer", Stack: "example", ComponentType: cfg.PackerComponentType},
			},
			want: []schema.Affected{
				{Component: "example-terraform", Stack: "example", ComponentType: cfg.TerraformComponentType},
			},
		},
		{
			name: "deleted terraform components are dropped",
			in: []schema.Affected{
				{Component: "live", Stack: "prod", ComponentType: cfg.TerraformComponentType},
				{Component: "removed", Stack: "prod", ComponentType: cfg.TerraformComponentType, Deleted: true, Affected: "deleted"},
				{Component: "removed-stack", Stack: "old", ComponentType: cfg.TerraformComponentType, Deleted: true, Affected: "deleted.stack"},
			},
			want: []schema.Affected{
				{Component: "live", Stack: "prod", ComponentType: cfg.TerraformComponentType},
			},
		},
		{
			name: "deleted helmfile/packer dropped (both filters apply)",
			in: []schema.Affected{
				{Component: "helm-gone", Stack: "prod", ComponentType: cfg.HelmfileComponentType, Deleted: true},
				{Component: "packer-gone", Stack: "prod", ComponentType: cfg.PackerComponentType, Deleted: true},
			},
			want: []schema.Affected{},
		},
		{
			name: "empty component type is dropped (not a terraform component)",
			in: []schema.Affected{
				{Component: "unknown", Stack: "prod", ComponentType: ""},
				{Component: "tf", Stack: "prod", ComponentType: cfg.TerraformComponentType},
			},
			want: []schema.Affected{
				{Component: "tf", Stack: "prod", ComponentType: cfg.TerraformComponentType},
			},
		},
		{
			name: "all terraform, none deleted: list preserved in order",
			in: []schema.Affected{
				{Component: "a", Stack: "prod", ComponentType: cfg.TerraformComponentType},
				{Component: "b", Stack: "prod", ComponentType: cfg.TerraformComponentType},
				{Component: "c", Stack: "prod", ComponentType: cfg.TerraformComponentType},
			},
			want: []schema.Affected{
				{Component: "a", Stack: "prod", ComponentType: cfg.TerraformComponentType},
				{Component: "b", Stack: "prod", ComponentType: cfg.TerraformComponentType},
				{Component: "c", Stack: "prod", ComponentType: cfg.TerraformComponentType},
			},
		},
		{
			name: "reproduces issue #2361 fixture (terraform + helmfile + packer in one stack)",
			in: []schema.Affected{
				{Component: "example-terraform", Stack: "example", ComponentType: cfg.TerraformComponentType, Affected: "stack.vars"},
				{Component: "example-helmfile", Stack: "example", ComponentType: cfg.HelmfileComponentType, Affected: "stack.vars"},
				{Component: "example-packer", Stack: "example", ComponentType: cfg.PackerComponentType, Affected: "stack.vars"},
			},
			want: []schema.Affected{
				{Component: "example-terraform", Stack: "example", ComponentType: cfg.TerraformComponentType, Affected: "stack.vars"},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := filterTerraformAffected(tc.in)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestFilterTerraformAffected_DoesNotMutateOriginalLength documents that the
// filter reuses the backing array (`affectedList[:0]` pattern) and therefore
// callers should treat the input slice as consumed. The slice header returned
// is the safe one to use; the original header may point to overwritten elements.
func TestFilterTerraformAffected_InPlaceSemantics(t *testing.T) {
	t.Parallel()

	in := []schema.Affected{
		{Component: "helm", ComponentType: cfg.HelmfileComponentType},
		{Component: "tf", ComponentType: cfg.TerraformComponentType},
	}
	got := filterTerraformAffected(in)

	assert.Len(t, got, 1)
	assert.Equal(t, "tf", got[0].Component)
	// Prove the filter compacted into the same backing array rather than
	// allocating a new one — the result's first element must be the input's
	// first slot, now overwritten to the kept terraform entry.
	assert.Equal(t, "tf", in[0].Component, "input should be compacted in place")
	assert.Same(t, &in[0], &got[0], "result should reuse input backing array")
	// The filter intentionally reuses the backing array. Future maintainers must
	// not assume `in` is unchanged. If a non-destructive variant is ever needed,
	// allocate a fresh slice instead.
}
