package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestContainerConfig_Decode(t *testing.T) {
	var cc ContainerConfig
	require.NoError(t, yaml.Unmarshal([]byte("runtime:\n  provider: podman\n  auto_start: true\n"), &cc))
	assert.Equal(t, "podman", cc.Runtime.Provider)
	assert.True(t, cc.Runtime.AutoStart)

	// Raw YAML decoding leaves absent fields at their zero values; config loading
	// applies the auto-start default separately.
	var empty ContainerConfig
	require.NoError(t, yaml.Unmarshal([]byte("{}\n"), &empty))
	assert.Empty(t, empty.Runtime.Provider)
	assert.False(t, empty.Runtime.AutoStart)
}

func TestContainerStep_ProviderDecode(t *testing.T) {
	// The per-step auto/docker/podman selector is `provider` (renamed from `runtime`).
	var auto ContainerRunStep
	require.NoError(t, yaml.Unmarshal([]byte("provider: auto\n"), &auto))
	assert.Equal(t, "auto", auto.Provider)

	var run ContainerRunStep
	require.NoError(t, yaml.Unmarshal([]byte("provider: docker\n"), &run))
	assert.Equal(t, "docker", run.Provider)

	var build ContainerBuildStep
	require.NoError(t, yaml.Unmarshal([]byte("provider: podman\n"), &build))
	assert.Equal(t, "podman", build.Provider)

	var wc WorkflowContainer
	require.NoError(t, yaml.Unmarshal([]byte("image: alpine\nprovider: podman\n"), &wc))
	assert.Equal(t, "podman", wc.Provider)
}
