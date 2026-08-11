package container

import (
	"context"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Compile-time guarantee that both runtimes expose a custom subprocess environment.
var (
	_ EnvSetter = (*DockerRuntime)(nil)
	_ EnvSetter = (*PodmanRuntime)(nil)
)

func TestApplyCommandEnv(t *testing.T) {
	t.Run("empty env leaves cmd.Env nil so it inherits os.Environ()", func(t *testing.T) {
		cmd := exec.CommandContext(context.Background(), "echo")
		applyCommandEnv(cmd, nil)
		assert.Nil(t, cmd.Env)

		applyCommandEnv(cmd, []string{})
		assert.Nil(t, cmd.Env)
	})

	t.Run("non-empty env becomes the command environment verbatim", func(t *testing.T) {
		cmd := exec.CommandContext(context.Background(), "echo")
		env := []string{"DOCKER_CONFIG=/tmp/atmos-docker", "PATH=/usr/bin"}
		applyCommandEnv(cmd, env)
		assert.Equal(t, env, cmd.Env)
	})
}

func TestDockerRuntimeSetEnvAppliesToCommand(t *testing.T) {
	d := NewDockerRuntime()

	// No env configured: commands inherit os.Environ() (cmd.Env stays nil).
	cmd := d.command(context.Background(), "version")
	assert.Nil(t, cmd.Env)
	assert.Equal(t, []string{dockerCmd, "version"}, cmd.Args)

	// After SetEnv the env is applied to every command this runtime builds.
	env := []string{"DOCKER_CONFIG=/tmp/atmos-docker"}
	d.SetEnv(env)
	cmd = d.command(context.Background(), "push", "app:latest")
	assert.Equal(t, env, cmd.Env)
	assert.Equal(t, []string{dockerCmd, "push", "app:latest"}, cmd.Args)
}

func TestPodmanRuntimeSetEnvAppliesToCommand(t *testing.T) {
	p := NewPodmanRuntime()

	cmd := p.command(context.Background(), "version")
	assert.Nil(t, cmd.Env)
	assert.Equal(t, []string{podmanCmd, "version"}, cmd.Args)

	env := []string{"DOCKER_CONFIG=/tmp/atmos-docker"}
	p.SetEnv(env)
	cmd = p.command(context.Background(), "push", "app:latest")
	assert.Equal(t, env, cmd.Env)
	assert.Equal(t, []string{podmanCmd, "push", "app:latest"}, cmd.Args)
}
