package aws

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Regression: a helper started through `sh -c` that leaves a grandchild holding the stdout
// pipe (for example `sleep 70`) used to run the full grandchild duration past the timeout,
// because exec.Cmd.Wait blocks until the pipe closes unless WaitDelay is set. The documented
// one-minute timeout then took 72 seconds in the field.
func TestRetrieveProcessCredentials_TimeoutIsPromptWhenGrandchildHoldsStdout(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	const grandchildLifetime = 12 * time.Second

	start := time.Now()
	creds, err := RetrieveProcessCredentials(
		context.Background(), "corp-slow", quoteCommandPath(exe),
		WithCredentialProcessTimeout(300*time.Millisecond),
		WithCredentialProcessEnviron(append(baseEnviron(), testCredentialProcessGrandchildEnv+"="+grandchildLifetime.String())),
	)
	elapsed := time.Since(start)

	require.ErrorIs(t, err, errUtils.ErrCredentialProcessFailed)
	assert.Nil(t, creds)
	// Timeout plus the bounded drain, far below the grandchild lifetime.
	assert.Less(t, elapsed, grandchildLifetime-4*time.Second,
		"the timeout must not wait for grandchildren that still hold the stdout pipe")
	assert.GreaterOrEqual(t, elapsed, 300*time.Millisecond)
}

func TestDefaultCredentialProcessCommandBuilder_BoundsPipeDrain(t *testing.T) {
	cmd, err := DefaultCredentialProcessCommandBuilder(context.Background(), "helper", nil)
	require.NoError(t, err)
	assert.Equal(t, credentialProcessWaitDelay, cmd.WaitDelay)
	assert.Positive(t, cmd.WaitDelay, "a zero WaitDelay lets a lingering grandchild defeat the timeout")
}
