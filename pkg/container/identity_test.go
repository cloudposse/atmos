package container

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInstanceAddress(t *testing.T) {
	assert.Equal(t, "dev/container/api", InstanceAddress("dev", "container", "api"))
}

func TestRuntimeName(t *testing.T) {
	tests := []struct {
		name      string
		stack     string
		compType  string
		component string
		want      string
	}{
		{
			name:      "simple",
			stack:     "dev",
			compType:  "container",
			component: "api",
			want:      "atmos-dev-container-api",
		},
		{
			name:      "slashes and uppercase are sanitized",
			stack:     "plat-ue2-dev",
			compType:  "container",
			component: "eks/cluster",
			want:      "atmos-plat-ue2-dev-container-eks-cluster",
		},
		{
			name:      "mixed invalid characters collapse to a single dash",
			stack:     "dev",
			compType:  "container",
			component: "api@@svc",
			want:      "atmos-dev-container-api-svc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, RuntimeName(tt.stack, tt.compType, tt.component))
		})
	}
}

func TestRuntimeName_BoundsLength(t *testing.T) {
	long := strings.Repeat("a", 500)
	got := RuntimeName(long, "container", "api")
	assert.LessOrEqual(t, len(got), maxRuntimeNameLength)
}

func TestInstanceLabels(t *testing.T) {
	labels := InstanceLabels("dev", "container", "api")
	assert.Equal(t, "dev", labels[LabelStack])
	assert.Equal(t, "container", labels[LabelComponentType])
	assert.Equal(t, "api", labels[LabelComponent])
	assert.Equal(t, "dev/container/api", labels[LabelInstance])
}

func TestDiscoveryFilter(t *testing.T) {
	filter := DiscoveryFilter("dev", "container", "api")
	assert.Equal(t, "tools.atmos.instance=dev/container/api", filter["label"])
}

func TestIsContainerRunning_Exported(t *testing.T) {
	tests := []struct {
		status string
		want   bool
	}{
		{"running", true},
		{"Running", true},
		{"Up 3 minutes", true},
		{"up 2 hours (healthy)", true},
		{"exited", false},
		{"exited (0) 5 minutes ago", false},
		{"created", false},
		{"", false},
		// "not running" must not match on a naive substring check.
		{"not running", false},
		{"  running  ", true},
	}
	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			assert.Equal(t, tt.want, IsContainerRunning(tt.status))
		})
	}
}
