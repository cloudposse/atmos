package main

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	if os.Getenv("NOTICEGEN_FAKE_GO") == "1" {
		if os.Getenv("NOTICEGEN_FAKE_GO_BLOCK") == "1" {
			time.Sleep(time.Hour)
		}
		_ = os.WriteFile(os.Getenv("NOTICEGEN_FAKE_GO_ARGS"), []byte(strings.Join(os.Args[1:], "\n")), 0o600)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestGoInstallTimeoutAndSuccessfulInvocation(t *testing.T) {
	self, err := os.Executable()
	require.NoError(t, err)
	data, err := os.ReadFile(self)
	require.NoError(t, err)
	dir := t.TempDir()
	name := "go"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), data, 0o700))
	t.Setenv("PATH", dir)
	t.Setenv("NOTICEGEN_FAKE_GO", "1")
	args := filepath.Join(t.TempDir(), "args")
	t.Setenv("NOTICEGEN_FAKE_GO_ARGS", args)
	t.Setenv("NOTICEGEN_FAKE_GO_BLOCK", "1")
	err = runGoInstallWithTimeout("example.com/license-tool@v1.0.0", 20*time.Millisecond)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoFileExists(t, args)
	t.Setenv("NOTICEGEN_FAKE_GO_BLOCK", "")
	require.NoError(t, defaultRunGoInstall("example.com/license-tool@v1.0.0"))
	data, err = os.ReadFile(args)
	require.NoError(t, err)
	require.Equal(t, "install\nexample.com/license-tool@v1.0.0", string(data))
}
