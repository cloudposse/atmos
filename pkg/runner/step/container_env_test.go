package step

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/container"
)

func TestVariablesEnvSlice(t *testing.T) {
	v := &Variables{Env: map[string]string{
		"B":             "2",
		"A":             "1",
		"DOCKER_CONFIG": "/tmp/atmos-docker",
	}}

	// Sorted, complete "KEY=VALUE" entries.
	assert.Equal(t, []string{"A=1", "B=2", "DOCKER_CONFIG=/tmp/atmos-docker"}, v.EnvSlice())
}

func TestVariablesEnvSliceEmpty(t *testing.T) {
	v := &Variables{Env: map[string]string{}}
	assert.Empty(t, v.EnvSlice())
}

// envCapturingRuntime embeds the container.Runtime interface (left nil — its
// methods are never invoked here) and overrides SetEnv so it also satisfies
// container.EnvSetter. This lets us verify applyRuntimeEnv forwards the resolved
// environment without implementing the full runtime surface.
type envCapturingRuntime struct {
	container.Runtime
	captured []string
}

func (r *envCapturingRuntime) SetEnv(env []string) {
	r.captured = env
}

func TestApplyRuntimeEnvForwardsResolvedEnv(t *testing.T) {
	vars := &Variables{Env: map[string]string{
		"DOCKER_CONFIG":         "/tmp/atmos-docker",
		"AWS_ACCESS_KEY_ID":     "AKIA",
		"AWS_SECRET_ACCESS_KEY": "secret",
	}}

	rt := &envCapturingRuntime{}
	applyRuntimeEnv(rt, vars)

	// The identity/integration-materialized credentials reach the runtime so its
	// docker/podman subprocesses can authenticate to the registry.
	assert.Equal(t, []string{
		"AWS_ACCESS_KEY_ID=AKIA",
		"AWS_SECRET_ACCESS_KEY=secret",
		"DOCKER_CONFIG=/tmp/atmos-docker",
	}, rt.captured)
}

// runtimeWithoutEnvSetter satisfies container.Runtime but not EnvSetter, so
// applyRuntimeEnv must be a no-op (the runtime inherits os.Environ()).
type runtimeWithoutEnvSetter struct {
	container.Runtime
}

func TestApplyRuntimeEnvNoOpWhenUnsupported(t *testing.T) {
	vars := &Variables{Env: map[string]string{"DOCKER_CONFIG": "/tmp/atmos-docker"}}

	// Must not panic or attempt to set env on a runtime that can't accept it.
	assert.NotPanics(t, func() {
		applyRuntimeEnv(&runtimeWithoutEnvSetter{}, vars)
	})
}

func TestVariablesEnsureBinaryInPath(t *testing.T) {
	binary := filepath.Join(string(filepath.Separator), "repo", "build", "atmos")
	binDir := filepath.Dir(binary)

	t.Run("prepends binary dir after PATH override", func(t *testing.T) {
		vars := NewVariables()
		vars.SetEnv("PATH", filepath.Join(string(filepath.Separator), "usr", "bin"))

		vars.EnsureBinaryInPath(binary)

		path := vars.Env["PATH"]
		require.True(t, strings.HasPrefix(path, binDir+string(os.PathListSeparator)),
			"binary dir should be first in PATH, got %q", path)
		require.Contains(t, path, filepath.Join(string(filepath.Separator), "usr", "bin"))
	})

	t.Run("no duplicate when already present", func(t *testing.T) {
		vars := NewVariables()
		vars.SetEnv("PATH", binDir)

		vars.EnsureBinaryInPath(binary)

		require.Equal(t, 1, strings.Count(vars.Env["PATH"], binDir))
	})

	t.Run("matches existing key casing", func(t *testing.T) {
		vars := NewVariables()
		delete(vars.Env, "PATH")
		vars.SetEnv("Path", filepath.Join(string(filepath.Separator), "usr", "bin"))

		vars.EnsureBinaryInPath(binary)

		_, hasUpper := vars.Env["PATH"]
		require.False(t, hasUpper, "must update the existing Path key, not add PATH")
		require.True(t, strings.HasPrefix(vars.Env["Path"], binDir+string(os.PathListSeparator)))
	})
}
