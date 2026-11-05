package process

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	envpkg "github.com/cloudposse/atmos/pkg/env"
)

func TestDefaultRunnerResolvesInvocationPATH(t *testing.T) {
	t.Parallel()
	command, args, env := processHelperCommand(t, "stdout-stderr")
	parentPath := os.Getenv("PATH")
	env = envpkg.UpdateEnvVar(env, "PATH", filepath.Dir(command))
	var stdout bytes.Buffer
	result := DefaultRunner{}.Run(context.Background(), TaskSpec{
		Command: filepath.Base(command), Args: args, Env: env, Streams: Streams{Stdout: &stdout},
	})
	require.NoError(t, result.Err)
	assert.Equal(t, "stdout", stdout.String())
	assert.Equal(t, parentPath, os.Getenv("PATH"))

	// A different invocation must not inherit the first one's search path.
	emptyEnv := envpkg.UpdateEnvVar(env, "PATH", t.TempDir())
	result = DefaultRunner{}.Run(context.Background(), TaskSpec{
		Command: filepath.Base(command), Args: args, Env: emptyEnv,
	})
	require.ErrorIs(t, result.Err, errUtils.ErrProcessStartFailed)
	assert.False(t, result.Started)
	assert.Equal(t, parentPath, os.Getenv("PATH"))
}

func TestDefaultRunnerResolvesRelativeDirectoryPATH(t *testing.T) {
	command, args, env := processHelperCommand(t, "stdout-stderr")
	project := t.TempDir()
	bin := filepath.Join(project, "work", "bin")
	require.NoError(t, os.MkdirAll(bin, 0o755))
	// Copy the Go helper executable so this also exercises Windows without
	// requiring a shell or permission to create symbolic links.
	data, err := os.ReadFile(command)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(bin, filepath.Base(command)), data, 0o755))
	t.Chdir(project)
	env = envpkg.UpdateEnvVar(env, "PATH", "bin")
	for _, test := range []struct {
		name    string
		command string
		dir     string
	}{
		{name: "relative directory and PATH", command: filepath.Base(command), dir: "work"},
		{name: "relative explicit command", command: filepath.Join("bin", filepath.Base(command)), dir: "work"},
		{name: "empty directory", command: filepath.Join("work", "bin", filepath.Base(command))},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			result := DefaultRunner{}.Run(context.Background(), TaskSpec{
				Command: test.command, Args: args, Dir: test.dir, Env: env,
				Streams: Streams{Stdout: &stdout},
			})
			require.NoError(t, result.Err)
			assert.True(t, result.Success())
			assert.Equal(t, "stdout", stdout.String())
		})
	}
}
