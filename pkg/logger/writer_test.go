package logger

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	charm "github.com/charmbracelet/log"
	"github.com/muesli/termenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpaqueWriter_HidesFdAndForwards(t *testing.T) {
	var buf bytes.Buffer
	w := &opaqueWriter{w: &buf}

	// Must not look like an *os.File nor expose a file descriptor.
	var asWriter io.Writer = w
	_, isFile := asWriter.(*os.File)
	assert.False(t, isFile)
	assert.False(t, hasFd(asWriter))

	n, err := w.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", buf.String())
}

func TestOpaqueWriter_NilForwardsToCurrentStderr(t *testing.T) {
	orig := os.Stderr
	t.Cleanup(func() { os.Stderr = orig })

	r, pw, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = pw

	w := &opaqueWriter{}
	_, err = w.Write([]byte("to-stderr"))
	require.NoError(t, err)
	require.NoError(t, pw.Close())

	data, err := io.ReadAll(r)
	require.NoError(t, err)
	assert.Equal(t, "to-stderr", string(data))
}

func TestNewOutputWriter(t *testing.T) {
	t.Run("os.Stderr is wrapped and resolved lazily", func(t *testing.T) {
		out, profile := newOutputWriter(os.Stderr)
		require.NotNil(t, profile)
		ow, ok := out.(opaqueWriter)
		require.True(t, ok)
		assert.Nil(t, ow.w)
		assert.False(t, hasFd(out))
	})

	t.Run("nil writer defaults to stderr", func(t *testing.T) {
		out, profile := newOutputWriter(nil)
		require.NotNil(t, profile)
		assert.False(t, hasFd(out))
	})

	t.Run("regular file is wrapped", func(t *testing.T) {
		f, err := os.Create(filepath.Join(t.TempDir(), "log.txt"))
		require.NoError(t, err)
		t.Cleanup(func() { _ = f.Close() })

		out, profile := newOutputWriter(f)
		require.NotNil(t, profile)
		assert.False(t, hasFd(out))
		// A regular file is not a TTY, so no color.
		assert.Equal(t, termenv.Ascii, *profile)

		_, err = out.Write([]byte("x"))
		require.NoError(t, err)
		info, err := f.Stat()
		require.NoError(t, err)
		assert.Equal(t, int64(1), info.Size())
	})

	t.Run("writer without fd is left untouched", func(t *testing.T) {
		var buf bytes.Buffer
		out, profile := newOutputWriter(&buf)
		assert.Same(t, &buf, out)
		assert.Nil(t, profile)
	})
}

func TestNewCharmLogger_UsesOpaqueWriter(t *testing.T) {
	l := newCharmLogger(os.Stderr, false)
	require.NotNil(t, l)

	// Setting a file output must also avoid exposing the descriptor to charm.
	f, err := os.Create(filepath.Join(t.TempDir(), "log.txt"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })

	al := NewAtmosLogger(l)
	al.SetOutput(f)
	al.Error("written to file")
	info, err := f.Stat()
	require.NoError(t, err)
	assert.Positive(t, info.Size())
}

func TestProfileFor(t *testing.T) {
	// Use a regular file: never a TTY, so only CLICOLOR_FORCE can raise the profile.
	f, err := os.Create(filepath.Join(t.TempDir(), "out.txt"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })

	t.Run("non-TTY is Ascii", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		t.Setenv("CLICOLOR_FORCE", "")
		t.Setenv("CLICOLOR", "")
		assert.Equal(t, termenv.Ascii, profileFor(f))
	})

	t.Run("NO_COLOR is Ascii even when forced", func(t *testing.T) {
		t.Setenv("NO_COLOR", "1")
		t.Setenv("CLICOLOR_FORCE", "1")
		t.Setenv("TERM", "xterm-256color")
		assert.Equal(t, termenv.Ascii, profileFor(f))
	})

	t.Run("forced color on non-TTY enables color", func(t *testing.T) {
		t.Setenv("NO_COLOR", "")
		t.Setenv("CLICOLOR_FORCE", "1")
		t.Setenv("CLICOLOR", "")
		t.Setenv("COLORTERM", "truecolor")
		t.Setenv("TERM", "xterm-256color")
		assert.NotEqual(t, termenv.Ascii, profileFor(f))
	})
}

func TestNewCharmLogger_AppliesProfile(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR", "")
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("TERM", "xterm-256color")

	f, err := os.Create(filepath.Join(t.TempDir(), "log.txt"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })

	l := newCharmLogger(f, false)
	l.SetStyles(charm.DefaultStyles())
	l.Error("colored")

	data, err := os.ReadFile(f.Name())
	require.NoError(t, err)
	assert.Contains(t, string(data), "\x1b[", "forced color profile should emit ANSI escapes")

	// NO_COLOR wins.
	t.Setenv("NO_COLOR", "1")
	f2, err := os.Create(filepath.Join(t.TempDir(), "log2.txt"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = f2.Close() })
	l2 := newCharmLogger(f2, false)
	l2.Error("plain")
	data, err = os.ReadFile(f2.Name())
	require.NoError(t, err)
	assert.NotContains(t, string(data), "\x1b[")
}

func TestInit_InstallsCharmDefault(t *testing.T) {
	// Code using charm's package-level functions must share the Atmos default logger.
	assert.Same(t, Default().charm, charm.Default())
}

func TestOutputWriterRetainsDestinationIdentity(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "log.txt"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	for name, destination := range map[string]io.Writer{"stderr": os.Stderr, "nil": nil, "file": f} {
		t.Run(name, func(t *testing.T) {
			// Charm retains a renderer for each distinct output key. Repeated
			// logger construction or SetOutput must not introduce new keys.
			keys := make(map[io.Writer]bool)
			for range 20 {
				out, _ := newOutputWriter(destination)
				keys[out] = true
			}
			assert.Len(t, keys, 1)
		})
	}
}

func TestCharmOutputSwitchPreservesIndependentDestinations(t *testing.T) {
	first, err := os.Create(filepath.Join(t.TempDir(), "first.log"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = first.Close() })
	second, err := os.Create(filepath.Join(t.TempDir(), "second.log"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })

	l := newCharmLogger(first, false)
	independent := newCharmLogger(first, false)
	for range 10 {
		setCharmOutput(l, second)
	}
	l.Print("second destination")
	independent.Print("first destination")
	setCharmOutput(l, first)
	l.Print("back to first")

	firstOutput, err := os.ReadFile(first.Name())
	require.NoError(t, err)
	secondOutput, err := os.ReadFile(second.Name())
	require.NoError(t, err)
	assert.Contains(t, string(firstOutput), "first destination")
	assert.Contains(t, string(firstOutput), "back to first")
	assert.NotContains(t, string(firstOutput), "second destination")
	assert.Contains(t, string(secondOutput), "second destination")
	assert.NotContains(t, string(secondOutput), "first destination")
}

// sliceFDWriter exercises custom file-like writers that cannot be map keys.
type sliceFDWriter []*bytes.Buffer

func (w sliceFDWriter) Write(p []byte) (int, error) { return w[0].Write(p) }
func (w sliceFDWriter) Fd() uintptr                 { return ^uintptr(0) }

func TestCharmOutputSupportsUncomparableFileWriter(t *testing.T) {
	var output bytes.Buffer
	w := sliceFDWriter{&output}
	l := newCharmLogger(w, false)
	l.Print("custom writer")
	setCharmOutput(l, w)
	l.Print("after SetOutput")
	assert.Contains(t, output.String(), "custom writer")
	assert.Contains(t, output.String(), "after SetOutput")
}
