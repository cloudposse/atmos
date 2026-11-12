package paths

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultRoot(t *testing.T) {
	tests := []struct {
		goos string
		want []string
	}{
		{"darwin", []string{".local", "share", "atmos", "ssh"}},
		{"linux", []string{".local", "share", "atmos", "ssh"}},
		{"windows", []string{"AppData", "Local", "atmos", "ssh"}},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			home := filepath.Join(string(filepath.Separator), "home", "me")
			got := defaultRoot(home, tt.goos)
			assert.Equal(t, filepath.Join(append([]string{home}, tt.want...)...), got)
		})
	}
}

func TestDefaultIgnoresEnvironment(t *testing.T) {
	t.Setenv("HOME", filepath.Join(t.TempDir(), "fake-home"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "xdg"))
	p, err := Default()
	require.NoError(t, err)
	home, err := HomeDir()
	require.NoError(t, err)
	assert.Equal(t, defaultRoot(home, runtime.GOOS), p.Root())
	assert.NotContains(t, p.Root(), "fake-home")
}

func TestLayout(t *testing.T) {
	root := t.TempDir()
	p, err := New(root)
	require.NoError(t, err)

	tests := map[string]struct{ got, want string }{
		"root":     {p.Root(), root},
		"keys":     {p.KeysDir(), filepath.Join(root, "keys")},
		"store":    {p.StoreDir("file"), filepath.Join(root, "keys", "file")},
		"sockets":  {p.SocketsDir(), filepath.Join(root, "sockets")},
		"receipts": {p.ReceiptsDir(), filepath.Join(root, "receipts")},
		"bin":      {p.BinDir(), filepath.Join(root, "bin")},
		"config":   {p.AgentConfigFile(), filepath.Join(root, "agent.json")},
		"audit":    {p.AuditLogFile(), filepath.Join(root, "audit.log")},
		"state":    {p.StateFile(), filepath.Join(root, "daemon.json")},
		"log":      {p.ServiceLogFile(), filepath.Join(root, "agent.log")},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) { assert.Equal(t, tt.want, tt.got) })
	}
}

func TestNewRejectsEmptyRoot(t *testing.T) {
	_, err := New("")
	require.ErrorIs(t, err, ErrEmptyRoot)
}

func TestSocketPath(t *testing.T) {
	p, err := New(t.TempDir())
	require.NoError(t, err)

	got, err := p.SocketPath("ops")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(p.SocketsDir(), "ops.sock"), got)

	for _, bad := range []string{"", "../x", "a/b", ".hidden", "a b", strings.Repeat("a", 65)} {
		_, err := p.SocketPath(bad)
		require.ErrorIs(t, err, ErrInvalidIdentity, "name %q", bad)
	}
}

func TestSocketPathTooLong(t *testing.T) {
	p, err := New(filepath.Join(t.TempDir(), strings.Repeat("d", 120)))
	require.NoError(t, err)
	_, err = p.SocketPath("ops")
	require.ErrorIs(t, err, ErrSocketPathTooLong)
	assert.Contains(t, err.Error(), "shorter data directory")
}

func TestMaxSocketPath(t *testing.T) {
	tests := map[string]int{"darwin": 104, "linux": 108, "windows": 108, "freebsd": 104}
	for goos, want := range tests {
		assert.Equal(t, want, maxSocketPath(goos), goos)
	}
}

func TestSocketPathBoundary(t *testing.T) {
	// A path exactly one byte shorter than the limit is accepted; at the limit it is refused.
	limit := maxSocketPath(runtime.GOOS)
	base := filepath.Join(string(filepath.Separator), "r")
	suffix := len(filepath.Join(string(filepath.Separator), "sockets", "ops.sock"))
	pad := limit - 1 - len(base) - suffix
	require.Positive(t, pad)

	ok, err := New(base + strings.Repeat("y", pad))
	require.NoError(t, err)
	_, err = ok.SocketPath("ops")
	require.NoError(t, err)

	tooLong, err := New(base + strings.Repeat("y", pad+1))
	require.NoError(t, err)
	_, err = tooLong.SocketPath("ops")
	require.ErrorIs(t, err, ErrSocketPathTooLong)
}

func TestEnsureDirs(t *testing.T) {
	p, err := New(filepath.Join(t.TempDir(), "ssh"))
	require.NoError(t, err)
	require.NoError(t, p.EnsureDirs())
	require.NoError(t, p.EnsureDirs(), "second call is a no-op")

	for _, dir := range []string{p.Root(), p.KeysDir(), p.SocketsDir(), p.ReceiptsDir(), p.BinDir()} {
		info, err := os.Stat(dir)
		require.NoError(t, err)
		require.True(t, info.IsDir())
		if runtime.GOOS != "windows" {
			assert.Equal(t, DirMode, info.Mode().Perm(), dir)
		}
	}
}

func TestEnsureDirsTightensMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes only")
	}
	root := filepath.Join(t.TempDir(), "ssh")
	require.NoError(t, os.MkdirAll(root, 0o755))
	p, err := New(root)
	require.NoError(t, err)
	require.NoError(t, p.EnsureDirs())
	info, err := os.Stat(root)
	require.NoError(t, err)
	assert.Equal(t, DirMode, info.Mode().Perm())
}

func TestErrorsAreDistinct(t *testing.T) {
	assert.False(t, errors.Is(ErrNoHome, ErrEmptyRoot))
}
