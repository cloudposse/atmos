package prompter

import (
	"io"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression: survey v2.3.7 panics when Ctrl+D is pressed after the filter removed every option.
// This drives the real survey Select through pipes, with no fake backend.
func TestCliPrompterChooseCtrlDWithNoMatches(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("survey reads the Windows console API, which pipes do not emulate")
	}

	inR, inW, err := os.Pipe()
	require.NoError(t, err)
	outR, outW, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = inR.Close()
		_ = outR.Close()
	})
	go func() { _, _ = io.Copy(io.Discard, outR) }()

	// "zzz" filters the list to zero matches; 0x04 is Ctrl+D.
	_, err = inW.WriteString("zzz\x04")
	require.NoError(t, err)
	require.NoError(t, inW.Close())

	origIn, origOut := os.Stdin, outputWriter
	os.Stdin, outputWriter = inR, outW
	t.Cleanup(func() {
		os.Stdin, outputWriter = origIn, origOut
		_ = outW.Close()
	})

	var got int
	require.NotPanics(t, func() { got, err = NewCli().Choose("Pick", []string{"alpha", "beta"}) })
	require.ErrorIs(t, err, errSelectAborted)
	assert.Zero(t, got)
}
