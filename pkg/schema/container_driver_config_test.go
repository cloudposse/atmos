package schema

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestContainerDriverConfigUnmarshalScalar verifies the shorthand `driver: docker-container`
// sets Provider with no Opts.
func TestContainerDriverConfigUnmarshalScalar(t *testing.T) {
	var d ContainerDriverConfig
	require.NoError(t, yaml.Unmarshal([]byte("docker-container"), &d))

	assert.Equal(t, "docker-container", d.Provider)
	assert.Empty(t, d.Name)
	assert.Empty(t, d.Opts)
}

// TestContainerDriverConfigUnmarshalMapping verifies the full object form decodes all fields.
func TestContainerDriverConfigUnmarshalMapping(t *testing.T) {
	input := `
name: atmos
provider: docker-container
opts:
  image: mirror.gcr.io/moby/buildkit:buildx-stable-1
`
	var d ContainerDriverConfig
	require.NoError(t, yaml.Unmarshal([]byte(input), &d))

	assert.Equal(t, "atmos", d.Name)
	assert.Equal(t, "docker-container", d.Provider)
	assert.Equal(t, "mirror.gcr.io/moby/buildkit:buildx-stable-1", d.Opts["image"])
}

// TestContainerDriverConfigUnmarshalRejectsSequence verifies the default-kind branch
// rejects a YAML sequence value for `driver:`.
func TestContainerDriverConfigUnmarshalRejectsSequence(t *testing.T) {
	var d ContainerDriverConfig
	err := yaml.Unmarshal([]byte("- a\n- b\n"), &d)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidContainerDriver))
}

// TestContainerDriverConfigUnmarshalRejectsInvalidMapping verifies a mapping whose fields
// fail typed decoding (e.g. opts is a list, not a map) surfaces a wrapped error.
func TestContainerDriverConfigUnmarshalRejectsInvalidMapping(t *testing.T) {
	var d ContainerDriverConfig
	err := yaml.Unmarshal([]byte("opts: [not, a, map]\n"), &d)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidContainerDriver))
}

// TestContainerBuildStepDriverShorthandInStep verifies the shorthand form round-trips
// through the full ContainerBuildStep decode path, not just the isolated struct.
func TestContainerBuildStepDriverShorthandInStep(t *testing.T) {
	input := `
engine: buildx
context: .
dockerfile: Dockerfile
driver: docker-container
`
	var step ContainerBuildStep
	require.NoError(t, yaml.Unmarshal([]byte(input), &step))

	require.NotNil(t, step.Driver)
	assert.Equal(t, "docker-container", step.Driver.Provider)
}

// TestContainerBuildStepCache verifies the cache.from/cache.to shape decodes into
// []map[string]string entries.
func TestContainerBuildStepCache(t *testing.T) {
	input := `
engine: buildx
context: .
dockerfile: Dockerfile
cache:
  from:
    - type: registry
      ref: registry.example.com/app:buildcache
  to:
    - type: registry
      ref: registry.example.com/app:buildcache
      mode: max
`
	var step ContainerBuildStep
	require.NoError(t, yaml.Unmarshal([]byte(input), &step))

	require.NotNil(t, step.Cache)
	require.Len(t, step.Cache.From, 1)
	assert.Equal(t, "registry", step.Cache.From[0]["type"])
	require.Len(t, step.Cache.To, 1)
	assert.Equal(t, "max", step.Cache.To[0]["mode"])
}
