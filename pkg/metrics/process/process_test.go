package process

import (
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollect_SuccessfulCommand(t *testing.T) {
	cmd := exec.Command("echo", "hello")
	metrics, err := Collect(cmd)
	require.NoError(t, err)
	require.NotNil(t, metrics)

	assert.Equal(t, 0, metrics.ExitCode)
	assert.Greater(t, metrics.WallTime, time.Duration(0))
	// CPU times should be non-negative.
	assert.GreaterOrEqual(t, metrics.UserCPUTime, time.Duration(0))
	assert.GreaterOrEqual(t, metrics.SystemCPUTime, time.Duration(0))
}

func TestCollect_FailingCommand(t *testing.T) {
	cmd := exec.Command("false")
	metrics, err := Collect(cmd)
	require.Error(t, err)
	require.NotNil(t, metrics)

	assert.NotEqual(t, 0, metrics.ExitCode)
	assert.Greater(t, metrics.WallTime, time.Duration(0))
}

func TestCollect_NonexistentCommand(t *testing.T) {
	cmd := exec.Command("this-command-does-not-exist-12345")
	metrics, err := Collect(cmd)
	require.Error(t, err)
	require.NotNil(t, metrics)

	// Process never started, so ExitCode should be -1.
	assert.Equal(t, -1, metrics.ExitCode)
}

func TestCollect_RusageFields(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Rusage not available on Windows")
	}

	// Run a command that does some work to get non-zero rusage.
	cmd := exec.Command("ls", "-la", "/")
	metrics, err := Collect(cmd)
	require.NoError(t, err)
	require.NotNil(t, metrics)

	// MaxRSSBytes should be positive for any process.
	assert.Greater(t, metrics.MaxRSSBytes, int64(0), "MaxRSSBytes should be > 0")
}

func TestCollect_WallTimeAccuracy(t *testing.T) {
	cmd := exec.Command("sleep", "0.1")
	metrics, err := Collect(cmd)
	require.NoError(t, err)

	// Wall time should be at least 50ms (allowing some slack).
	assert.GreaterOrEqual(t, metrics.WallTime, 50*time.Millisecond)
	// But not absurdly long.
	assert.Less(t, metrics.WallTime, 5*time.Second)
}

func TestCollectFromProcessState(t *testing.T) {
	cmd := exec.Command("echo", "hello")
	err := cmd.Run()
	require.NoError(t, err)

	metrics := CollectFromProcessState(cmd, 100*time.Millisecond)
	require.NotNil(t, metrics)

	assert.Equal(t, 0, metrics.ExitCode)
	assert.Equal(t, 100*time.Millisecond, metrics.WallTime)
}
