package exec

import (
	"context"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultExecutor(t *testing.T) {
	e := Default()
	require.NotNil(t, e)

	t.Run("LookPath finds a known binary", func(t *testing.T) {
		// `go` is guaranteed present in the test environment.
		path, err := e.LookPath("go")
		require.NoError(t, err)
		assert.NotEmpty(t, path)
	})

	t.Run("LookPath errors on a missing binary", func(t *testing.T) {
		_, err := e.LookPath("atmos-definitely-not-a-real-binary-xyz")
		assert.Error(t, err)
	})

	t.Run("CommandContext runs a trivial command", func(t *testing.T) {
		ctx := context.Background()
		cmd := e.CommandContext(ctx, "go", "version")
		if runtime.GOOS == "windows" {
			cmd = e.CommandContext(ctx, "cmd", "/c", "ver")
		}
		out, err := cmd.Output()
		require.NoError(t, err)
		assert.NotEmpty(t, out)
	})
}
