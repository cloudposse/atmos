package process

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunManaged_KillsGrandchildOnCancel(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)

	dir := t.TempDir()
	childPIDFile := filepath.Join(dir, "child.pid")
	grandchildPIDFile := filepath.Join(dir, "grandchild.pid")

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, exe, "-test.run=TestManagedProcessHelper", "--", "child", childPIDFile, grandchildPIDFile)
	cmd.Env = os.Environ()

	done := make(chan error, 1)
	go func() {
		done <- RunManaged(cmd)
	}()

	childPID := readPIDEventually(t, childPIDFile)
	grandchildPID := readPIDEventually(t, grandchildPIDFile)
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("managed command did not exit after cancellation")
	}

	assert.Eventually(t, func() bool { return !isProcessAlive(childPID) }, 5*time.Second, 50*time.Millisecond)
	assert.Eventually(t, func() bool { return !isProcessAlive(grandchildPID) }, 5*time.Second, 50*time.Millisecond)
}

func TestManagedProcessHelper(t *testing.T) {
	args := helperArgs()
	if args == nil {
		return
	}

	switch args[0] {
	case "child":
		require.Len(t, args, 3)
		require.NoError(t, os.WriteFile(args[1], []byte(strconv.Itoa(os.Getpid())), 0o600))

		exe, err := os.Executable()
		require.NoError(t, err)
		grandchild := exec.Command(exe, "-test.run=TestManagedProcessHelper", "--", "grandchild", args[2])
		grandchild.Env = os.Environ()
		require.NoError(t, grandchild.Start())

		for {
			time.Sleep(time.Minute)
		}
	case "grandchild":
		require.Len(t, args, 2)
		require.NoError(t, os.WriteFile(args[1], []byte(strconv.Itoa(os.Getpid())), 0o600))
		for {
			time.Sleep(time.Minute)
		}
	default:
		t.Fatalf("unknown helper command: %s", args[0])
	}
}

func readPIDEventually(t *testing.T, path string) int {
	t.Helper()

	var pid int
	assert.Eventually(t, func() bool {
		content, err := os.ReadFile(path)
		if err != nil {
			return false
		}
		parsed, err := strconv.Atoi(string(content))
		if err != nil {
			return false
		}
		pid = parsed
		return true
	}, 5*time.Second, 50*time.Millisecond)
	require.Positive(t, pid)
	return pid
}
