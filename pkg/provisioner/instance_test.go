package provisioner

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInstanceKey(t *testing.T) {
	tests := []struct {
		name            string
		componentConfig map[string]any
		want            string
	}{
		{
			name:            "stack and atmos_component",
			componentConfig: map[string]any{"atmos_stack": "dev", "atmos_component": "vpc"},
			want:            "dev-vpc",
		},
		{
			name:            "falls back to component when atmos_component empty",
			componentConfig: map[string]any{"atmos_stack": "prod", "component": "eks"},
			want:            "prod-eks",
		},
		{
			name:            "sanitizes path separators in stack name",
			componentConfig: map[string]any{"atmos_stack": "tenant1/ue2/prod", "atmos_component": "vpc/network"},
			want:            "tenant1-ue2-prod-vpc-network",
		},
		{
			name:            "trims leading separators when stack empty",
			componentConfig: map[string]any{"atmos_component": "vpc"},
			want:            "vpc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, InstanceKey(tt.componentConfig))
		})
	}
}

func TestInstanceLockFilename(t *testing.T) {
	got := InstanceLockFilename(map[string]any{"atmos_stack": "dev", "atmos_component": "vpc"})
	assert.Equal(t, ".dev-vpc.terraform.lock.hcl", got)
}
