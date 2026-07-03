package keyring

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	keyringlib "github.com/99designs/keyring"
	"github.com/charmbracelet/huh"
	"github.com/creack/pty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errRing is a fake keyringlib.Keyring whose Get/Remove fail with a configurable error, used to
// exercise the non-ErrNotFound branches of fileKeyring.Delete and fileKeyring.Has.
type errRing struct {
	getErr    error
	removeErr error
}

func (e errRing) Get(string) (keyringlib.Item, error) { return keyringlib.Item{}, e.getErr }
func (e errRing) GetMetadata(string) (keyringlib.Metadata, error) {
	return keyringlib.Metadata{}, e.getErr
}
func (e errRing) Set(keyringlib.Item) error { return nil }
func (e errRing) Remove(string) error       { return e.removeErr }
func (e errRing) Keys() ([]string, error)   { return nil, nil }

func TestFileKeyring_Delete_ErrorBranches(t *testing.T) {
	t.Run("transport error propagates", func(t *testing.T) {
		boom := errors.New("disk failure")
		k := &fileKeyring{ring: errRing{removeErr: boom}, dir: t.TempDir()}
		err := k.Delete("k")
		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("key-not-found is idempotent", func(t *testing.T) {
		k := &fileKeyring{ring: errRing{removeErr: keyringlib.ErrKeyNotFound}, dir: t.TempDir()}
		assert.NoError(t, k.Delete("k"))
	})

	t.Run("missing file is idempotent", func(t *testing.T) {
		k := &fileKeyring{ring: errRing{removeErr: os.ErrNotExist}, dir: t.TempDir()}
		assert.NoError(t, k.Delete("k"))
	})
}

func TestFileKeyring_Has_ErrorBranch(t *testing.T) {
	boom := errors.New("disk failure")
	k := &fileKeyring{ring: errRing{getErr: boom}, dir: t.TempDir()}
	_, err := k.Has("k")
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}

func TestNewFileKeyring_MkdirFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file-as-parent semantics differ on Windows")
	}
	// Create a regular file, then ask for a keyring directory beneath it. MkdirAll must fail
	// because a path component is not a directory.
	parent := filepath.Join(t.TempDir(), "afile")
	require.NoError(t, os.WriteFile(parent, []byte("x"), 0o600))

	_, err := newFileKeyring(Config{Type: TypeFile, FileDir: filepath.Join(parent, "sub")})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnavailable)
}

func TestNewFileKeyring_DefaultDir(t *testing.T) {
	// Empty FileDir resolves the XDG data dir; point XDG at a temp dir so no real home is touched.
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("ATMOS_KEYRING_PASSWORD", "test-password-12345")

	k, err := newFileKeyring(Config{Type: TypeFile})
	require.NoError(t, err)
	require.NoError(t, k.Set("k", "v"))
	got, err := k.Get("k")
	require.NoError(t, err)
	assert.Equal(t, "v", got)
}

func TestNewPasswordPrompt_EnvPath(t *testing.T) {
	t.Run("valid password returned", func(t *testing.T) {
		t.Setenv("ATMOS_KEYRING_PASSWORD", "long-enough-password")
		got, err := newPasswordPrompt("ATMOS_KEYRING_PASSWORD")("Password:")
		require.NoError(t, err)
		assert.Equal(t, "long-enough-password", got)
	})

	t.Run("too-short password rejected", func(t *testing.T) {
		t.Setenv("ATMOS_KEYRING_PASSWORD", "short")
		_, err := newPasswordPrompt("ATMOS_KEYRING_PASSWORD")("Password:")
		assert.ErrorIs(t, err, ErrPasswordTooShort)
	})
}

func TestNewPasswordPrompt_NoTTYNoEnv(t *testing.T) {
	// Force the non-interactive path: no env password and stdin is not a terminal.
	t.Setenv("ATMOS_KEYRING_PASSWORD", "")
	restore := stdinIsTerminal
	stdinIsTerminal = func() bool { return false }
	t.Cleanup(func() { stdinIsTerminal = restore })

	_, err := newPasswordPrompt("ATMOS_KEYRING_PASSWORD")("Password:")
	assert.ErrorIs(t, err, ErrPasswordRequired)
}

func TestNewPasswordPrompt_InteractiveFormError(t *testing.T) {
	t.Setenv("ATMOS_KEYRING_PASSWORD", "")
	restoreTTY := stdinIsTerminal
	stdinIsTerminal = func() bool { return true }
	t.Cleanup(func() { stdinIsTerminal = restoreTTY })

	boom := errors.New("form boom")
	restoreForm := runPasswordForm
	runPasswordForm = func(*huh.Form) error { return boom }
	t.Cleanup(func() { runPasswordForm = restoreForm })

	_, err := newPasswordPrompt("ATMOS_KEYRING_PASSWORD")("Password:")
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "password prompt failed")
}

// TestNewPasswordPrompt_InteractivePTY drives the real masked-input form over a pty. The first
// (too-short) entry trips the length validator, then a valid password is accepted — covering both
// the validator's error branch and the success path.
func TestNewPasswordPrompt_InteractivePTY(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty is not supported on Windows")
	}
	t.Setenv("ATMOS_KEYRING_PASSWORD", "")

	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Skipf("pty unavailable in this environment: %v", err)
	}
	t.Cleanup(func() {
		_ = ptmx.Close()
		_ = tty.Close()
	})

	restoreTTY := stdinIsTerminal
	stdinIsTerminal = func() bool { return true }
	t.Cleanup(func() { stdinIsTerminal = restoreTTY })

	restoreForm := runPasswordForm
	runPasswordForm = func(f *huh.Form) error {
		return f.WithAccessible(true).WithInput(tty).WithOutput(io.Discard).Run()
	}
	t.Cleanup(func() { runPasswordForm = restoreForm })

	writeErr := make(chan error, 1)
	go func() {
		_, e := io.WriteString(ptmx, "short\rlong-enough-password\r")
		writeErr <- e
	}()

	got, err := newPasswordPrompt("ATMOS_KEYRING_PASSWORD")("Password:")
	require.NoError(t, err)
	assert.Equal(t, "long-enough-password", got)
	require.NoError(t, <-writeErr)
}
