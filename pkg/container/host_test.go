package container

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyHostRuntime(t *testing.T) {
	t.Run("mounts socket, forces root, relabels, sets DOCKER_HOST", func(t *testing.T) {
		config := &CreateConfig{Host: true}
		applyHostRuntime(config, "/run/podman/podman.sock")

		require.Len(t, config.Mounts, 1)
		assert.Equal(t, Mount{Type: "bind", Source: "/run/podman/podman.sock", Target: "/var/run/docker.sock"}, config.Mounts[0])
		assert.Equal(t, "0", config.User)
		assert.Contains(t, config.SecurityOpt, "label=disable")
		assert.Equal(t, "unix:///var/run/docker.sock", config.Env["DOCKER_HOST"])
	})

	t.Run("preserves an explicit user", func(t *testing.T) {
		config := &CreateConfig{Host: true, User: "1000"}
		applyHostRuntime(config, "/var/run/docker.sock")
		assert.Equal(t, "1000", config.User)
	})

	t.Run("preserves an explicit DOCKER_HOST", func(t *testing.T) {
		config := &CreateConfig{Host: true, Env: map[string]string{"DOCKER_HOST": "tcp://1.2.3.4:2375"}}
		applyHostRuntime(config, "/var/run/docker.sock")
		assert.Equal(t, "tcp://1.2.3.4:2375", config.Env["DOCKER_HOST"])
	})
}

// TestBuildCreateArgsHost verifies the host-runtime additions reach the final create
// args once applyHostRuntime has mutated the config (the runtime Create chokepoint).
func TestBuildCreateArgsHost(t *testing.T) {
	config := &CreateConfig{Name: "demo", Image: "busybox", Host: true}
	applyHostRuntime(config, "/run/podman/podman.sock")
	args := strings.Join(buildCreateArgs(config), " ")

	assert.Contains(t, args, "--mount type=bind,source=/run/podman/podman.sock,target=/var/run/docker.sock")
	assert.Contains(t, args, "--user 0")
	assert.Contains(t, args, "--security-opt label=disable")
	assert.Contains(t, args, "-e DOCKER_HOST=unix:///var/run/docker.sock")
}

func TestDockerHostSocket(t *testing.T) {
	t.Run("unix DOCKER_HOST", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "unix:///custom/docker.sock")
		assert.Equal(t, "/custom/docker.sock", dockerHostSocket())
	})

	t.Run("non-unix DOCKER_HOST falls back to default", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "tcp://1.2.3.4:2375")
		assert.Equal(t, defaultDockerSocket, dockerHostSocket())
	})

	t.Run("unset DOCKER_HOST falls back to default", func(t *testing.T) {
		t.Setenv("DOCKER_HOST", "")
		assert.Equal(t, defaultDockerSocket, dockerHostSocket())
	})
}

// TestPrepareHostRuntimeNoop confirms a config without Host is left untouched.
func TestPrepareHostRuntimeNoop(t *testing.T) {
	config := &CreateConfig{Name: "demo"}
	require.NoError(t, prepareHostRuntime(context.Background(), nil, config))
	assert.Empty(t, config.Mounts)
	assert.Empty(t, config.User)
	assert.Empty(t, config.SecurityOpt)
	assert.Empty(t, config.Env)
}
