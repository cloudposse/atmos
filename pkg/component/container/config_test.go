package container

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDefaultConfig(t *testing.T) {
	assert.Equal(t, "components/container", DefaultConfig().BasePath)
}

func TestParseConfig(t *testing.T) {
	config, err := parseConfig(map[string]any{"base_path": "custom/container"})
	require.NoError(t, err)
	assert.Equal(t, "custom/container", config.BasePath)
}

func TestParseConfig_DecodeError(t *testing.T) {
	// A non-map raw value cannot decode into the Config struct.
	_, err := parseConfig([]any{1, 2, 3})
	require.Error(t, err)
}

func TestDecodeSection(t *testing.T) {
	var build schema.ContainerBuildStep
	require.NoError(t, decodeSection(map[string]any{"context": "app", "dockerfile": "Dockerfile"}, &build))
	assert.Equal(t, "app", build.Context)
	assert.Equal(t, "Dockerfile", build.Dockerfile)
}

func TestDecodeSection_Error(t *testing.T) {
	// A scalar cannot decode into a struct.
	var build schema.ContainerBuildStep
	require.Error(t, decodeSection("not-a-map", &build))
}

func TestFromComponentSection(t *testing.T) {
	// First-class top-level keys (NOT under vars).
	section := map[string]any{
		"composition": "storefront",
		"image":       "localhost:5001/api:abc",
		"build": map[string]any{
			"context":    "app",
			"dockerfile": "Dockerfile",
			"tags":       []any{"localhost:5001/api:abc"},
			"build_args": map[string]any{"VERSION": "1.0"},
		},
		"run": map[string]any{
			"command": "./api",
			"ports": []any{
				map[string]any{"host": 8080, "container": 80},
			},
			"mounts": []any{
				map[string]any{"source": ".", "target": "/workspace", "read_only": true},
			},
			"user": "app",
		},
	}

	spec, err := FromComponentSection(section)
	require.NoError(t, err)
	assert.Equal(t, "localhost:5001/api:abc", spec.Image)
	assert.Equal(t, "storefront", spec.Composition)

	require.NotNil(t, spec.Build)
	assert.Equal(t, "app", spec.Build.Context)
	assert.Equal(t, []string{"localhost:5001/api:abc"}, spec.Build.Tags)
	assert.Equal(t, "1.0", spec.Build.BuildArgs["VERSION"]) // snake_case build_args decoded via yaml tag

	require.NotNil(t, spec.Run)
	assert.Equal(t, "./api", spec.Run.Command)
	assert.Equal(t, "app", spec.Run.User)
	require.Len(t, spec.Run.Ports, 1)
	assert.Equal(t, 8080, spec.Run.Ports[0].Host)
	assert.Equal(t, 80, spec.Run.Ports[0].Container)
	require.Len(t, spec.Run.Mounts, 1)
	assert.Equal(t, "/workspace", spec.Run.Mounts[0].Target)
	assert.True(t, spec.Run.Mounts[0].ReadOnly) // snake_case read_only decoded via yaml tag
}

func TestFromComponentSection_NoVarsNesting(t *testing.T) {
	// image/build/run under `vars` must NOT be picked up (they are first-class now).
	section := map[string]any{
		"vars": map[string]any{"image": "should-be-ignored"},
	}
	spec, err := FromComponentSection(section)
	require.NoError(t, err)
	assert.Empty(t, spec.Image)
	assert.Nil(t, spec.Build)
	assert.Nil(t, spec.Run)
}

func TestContainerSpec_ToBuildConfig(t *testing.T) {
	spec := ContainerSpec{Build: &schema.ContainerBuildStep{
		Context:    "app",
		Dockerfile: "Dockerfile",
		Tags:       []string{"img:1"},
		Target:     "prod",
		NoCache:    true,
	}}
	bc := spec.ToBuildConfig()
	require.NotNil(t, bc)
	assert.Equal(t, "app", bc.Context)
	assert.Equal(t, []string{"img:1"}, bc.Tags)
	assert.Equal(t, "prod", bc.Target)
	assert.True(t, bc.NoCache)
	assert.Nil(t, bc.Driver)
	assert.Nil(t, bc.Cache)

	assert.Nil(t, (&ContainerSpec{}).ToBuildConfig())
}

func TestContainerSpec_ToBuildConfig_DriverAndCache(t *testing.T) {
	spec := ContainerSpec{Build: &schema.ContainerBuildStep{
		Context:    "app",
		Dockerfile: "Dockerfile",
		Driver: &schema.ContainerDriverConfig{
			Name:     "atmos",
			Provider: "docker-container",
			Opts:     map[string]string{"image": "mirror.gcr.io/moby/buildkit:buildx-stable-1"},
		},
		Cache: &schema.ContainerCacheConfig{
			From: []map[string]string{{"type": "registry", "ref": "registry.example.com/app:buildcache"}},
			To:   []map[string]string{{"type": "registry", "ref": "registry.example.com/app:buildcache", "mode": "max"}},
		},
	}}
	bc := spec.ToBuildConfig()
	require.NotNil(t, bc)

	require.NotNil(t, bc.Driver)
	assert.Equal(t, "atmos", bc.Driver.Name)
	assert.Equal(t, "docker-container", bc.Driver.Provider)
	assert.Equal(t, "mirror.gcr.io/moby/buildkit:buildx-stable-1", bc.Driver.Opts["image"])

	require.NotNil(t, bc.Cache)
	require.Len(t, bc.Cache.From, 1)
	assert.Equal(t, "registry.example.com/app:buildcache", bc.Cache.From[0]["ref"])
	require.Len(t, bc.Cache.To, 1)
	assert.Equal(t, "max", bc.Cache.To[0]["mode"])
}

func TestContainerSpec_CommandArgs(t *testing.T) {
	empty, err := (&ContainerSpec{}).CommandArgs()
	require.NoError(t, err)
	assert.Nil(t, empty)

	args, err := (&ContainerSpec{Run: &schema.ContainerRunStep{Command: "./api --port 8080"}}).CommandArgs()
	require.NoError(t, err)
	assert.Equal(t, []string{"./api", "--port", "8080"}, args)

	// Quoted arguments must survive tokenization as a single argv element
	// (strings.Fields would have split "echo hi" into two).
	quoted, err := (&ContainerSpec{Run: &schema.ContainerRunStep{Command: `sh -c "echo hi"`}}).CommandArgs()
	require.NoError(t, err)
	assert.Equal(t, []string{"sh", "-c", "echo hi"}, quoted)

	// An unbalanced quote is a config error, not silently corrupted argv.
	_, err = (&ContainerSpec{Run: &schema.ContainerRunStep{Command: `sh -c "echo`}}).CommandArgs()
	require.ErrorIs(t, err, errUtils.ErrComponentConfigInvalid)
}

func TestContainerSpec_Mounts(t *testing.T) {
	spec := ContainerSpec{Run: &schema.ContainerRunStep{Mounts: []schema.ContainerMount{
		{Source: ".", Target: "/workspace", ReadOnly: true},
	}}}
	mounts := spec.Mounts()
	require.Len(t, mounts, 1)
	assert.Equal(t, "bind", mounts[0].Type) // defaulted
	assert.Equal(t, "/workspace", mounts[0].Target)
	assert.True(t, mounts[0].ReadOnly)

	assert.Nil(t, (&ContainerSpec{}).Mounts())
}

func TestContainerSpec_Ports(t *testing.T) {
	spec := ContainerSpec{Run: &schema.ContainerRunStep{Ports: []schema.ContainerPort{
		{Host: 8080, Container: 80},
		{Host: 53, Container: 53, Protocol: "udp"},
	}}}
	ports := spec.Ports()
	require.Len(t, ports, 2)
	assert.Equal(t, 8080, ports[0].HostPort)
	assert.Equal(t, 80, ports[0].ContainerPort)
	assert.Equal(t, "tcp", ports[0].Protocol) // defaulted
	assert.Equal(t, 53, ports[1].ContainerPort)
	assert.Equal(t, "udp", ports[1].Protocol)

	assert.Nil(t, (&ContainerSpec{}).Ports())
}
