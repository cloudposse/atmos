package service

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// The test binary doubles as a cross-platform command for ExecRunner tests.
	if mode := os.Getenv("_ATMOS_SERVICE_TEST_MODE"); mode != "" {
		switch mode {
		case "exit3":
			os.Stderr.WriteString("boom")
			os.Exit(3)
		case "env":
			for _, kv := range os.Environ() {
				os.Stdout.WriteString(kv + "\n")
			}
			os.Exit(0)
		}
	}
	os.Exit(m.Run())
}

func TestStablePath(t *testing.T) {
	p, _ := testPaths(t)
	got, err := StablePath(p, "1.2.3")
	require.NoError(t, err)
	name := "atmos-ssh-1.2.3"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	assert.Equal(t, filepath.Join(p.BinDir(), name), got)

	for _, bad := range []string{"", "../x", "a/b", "a b", ".x"} {
		_, err := StablePath(p, bad)
		require.ErrorIs(t, err, ErrInvalidVersion, "version %q", bad)
	}
}

func TestParseStableVersion(t *testing.T) {
	p, _ := testPaths(t)
	path, err := StablePath(p, "0.9.0-rc.1+build5")
	require.NoError(t, err)
	v, ok := ParseStableVersion(path)
	require.True(t, ok)
	assert.Equal(t, "0.9.0-rc.1+build5", v)

	for _, other := range []string{"/x/plugin", "/x/atmos-ssh-", ""} {
		_, ok := ParseStableVersion(other)
		assert.False(t, ok, other)
	}
}

func TestCopyExecutable(t *testing.T) {
	p, _ := testPaths(t)
	src := filepath.Join(t.TempDir(), "plugin")
	require.NoError(t, os.WriteFile(src, []byte("binary v1"), 0o644))

	dst, err := CopyExecutable(p, src, "1.0.0")
	require.NoError(t, err)
	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "binary v1", string(got))
	if runtime.GOOS != "windows" {
		info, err := os.Stat(dst)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o755), info.Mode().Perm())
		dirInfo, err := os.Stat(p.BinDir())
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
	}
	entries, err := os.ReadDir(p.BinDir())
	require.NoError(t, err)
	require.Len(t, entries, 1, "no temporary files left behind")
}

func TestCopyExecutableSkipsIdenticalAndReplacesChanged(t *testing.T) {
	p, _ := testPaths(t)
	src := filepath.Join(t.TempDir(), "plugin")
	require.NoError(t, os.WriteFile(src, []byte("same"), 0o644))
	dst, err := CopyExecutable(p, src, "1.0.0")
	require.NoError(t, err)
	before, err := os.Stat(dst)
	require.NoError(t, err)

	_, err = CopyExecutable(p, src, "1.0.0")
	require.NoError(t, err)
	after, err := os.Stat(dst)
	require.NoError(t, err)
	assert.True(t, os.SameFile(before, after), "identical content is not rewritten")

	require.NoError(t, os.WriteFile(src, []byte("changed"), 0o644))
	_, err = CopyExecutable(p, src, "1.0.0")
	require.NoError(t, err)
	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "changed", string(got))
}

func TestCopyExecutableErrors(t *testing.T) {
	p, _ := testPaths(t)
	_, err := CopyExecutable(p, filepath.Join(t.TempDir(), "missing"), "1.0.0")
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = CopyExecutable(p, os.Args[0], "bad version")
	require.ErrorIs(t, err, ErrInvalidVersion)
}

func TestInstallExecutableCopiesTheRunningBinary(t *testing.T) {
	p, _ := testPaths(t)
	dst, err := InstallExecutable(p, "7.7.7")
	require.NoError(t, err)

	self, err := os.Executable()
	require.NoError(t, err)
	want, err := os.ReadFile(self)
	require.NoError(t, err)
	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Equal(t, p.BinDir(), filepath.Dir(dst))
}

func TestExecRunner(t *testing.T) {
	self, err := os.Executable()
	require.NoError(t, err)
	r := ExecRunner{}

	t.Run("non-zero exit is not an error", func(t *testing.T) {
		out, err := r.Run(context.Background(), Command{Name: self, Env: []string{"_ATMOS_SERVICE_TEST_MODE=exit3"}})
		require.NoError(t, err)
		assert.Equal(t, 3, out.ExitCode)
		// A binary built with -cover adds a GOCOVERDIR warning to stderr when run with this exact environment.
		assert.Contains(t, out.Stderr, "boom")
	})
	t.Run("environment is exactly the one given", func(t *testing.T) {
		t.Setenv("LEAK_CHECK", "leaked")
		out, err := r.Run(context.Background(), Command{Name: self, Env: []string{"_ATMOS_SERVICE_TEST_MODE=env", "ONLY=this"}})
		require.NoError(t, err)
		assert.Contains(t, out.Stdout, "ONLY=this")
		assert.NotContains(t, out.Stdout, "LEAK_CHECK")
	})
	t.Run("missing program is an error", func(t *testing.T) {
		_, err := r.Run(context.Background(), Command{Name: filepath.Join(t.TempDir(), "nope")})
		require.Error(t, err)
	})
}

func TestUnsupportedPlatform(t *testing.T) {
	p, _ := testPaths(t)
	m, err := newForOS("windows", Options{Paths: p})
	require.NoError(t, err)
	ctx := context.Background()
	require.ErrorIs(t, m.Install(ctx, Spec{Executable: "/x"}), ErrUnsupportedPlatform)
	require.ErrorIs(t, m.Start(ctx), ErrUnsupportedPlatform)
	require.ErrorIs(t, m.Stop(ctx), ErrUnsupportedPlatform)
	require.ErrorIs(t, m.Uninstall(ctx), ErrUnsupportedPlatform)
	_, err = m.Status(ctx)
	require.ErrorIs(t, err, ErrUnsupportedPlatform)
}

func TestNewRequiresPaths(t *testing.T) {
	_, err := New(Options{})
	require.ErrorIs(t, err, ErrInvalidSpec)
}
