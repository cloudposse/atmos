package starlark

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/script"
)

func runWhich(t *testing.T, source string, processEnv []string) (script.Result, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return New().Execute(ctx, script.Spec{Name: "test.star", Source: source, WorkingDirectory: t.TempDir(), ProcessEnv: processEnv})
}

func TestExecWhichResolvesAgainstTheProcessEnvironment(t *testing.T) {
	t.Parallel()
	exe, err := os.Executable()
	require.NoError(t, err)
	// The test binary is the one executable that exists on every platform.
	env := []string{"PATH=" + filepath.Dir(exe), "PATHEXT=" + os.Getenv("PATHEXT")}

	result, err := runWhich(t, `output = exec.which("`+filepath.ToSlash(filepath.Base(exe))+`")`, env)
	require.NoError(t, err)
	assert.Equal(t, filepath.Clean(exe), filepath.Clean(result.Value))

	// Only the script's PATH is searched, never the test process's own.
	result, err = runWhich(t, `output = exec.which("`+filepath.ToSlash(filepath.Base(exe))+`") == None`, []string{"PATH=" + t.TempDir()})
	require.NoError(t, err)
	assert.Equal(t, "true", result.Value)

	result, err = runWhich(t, `output = exec.which("atmos-which-missing-tool") == None`, env)
	require.NoError(t, err)
	assert.Equal(t, "true", result.Value)
}

func TestExecWhichValidation(t *testing.T) {
	t.Parallel()
	_, err := runWhich(t, `exec.which("")`, nil)
	require.ErrorIs(t, err, errUtils.ErrStarlarkInvalidArgument)
	for _, source := range []string{`exec.which()`, `exec.which(1)`, `exec.which("a", "b")`} {
		_, err := runWhich(t, source, nil)
		require.ErrorIs(t, err, errUtils.ErrStarlark, source)
	}
}
