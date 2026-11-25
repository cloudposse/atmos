package startup

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// captureStdStreams redirects os.Stdout and os.Stderr to pipes and returns a
// function that restores them and returns everything written to each.
func captureStdStreams(t *testing.T) func() (string, string) {
	t.Helper()

	origOut, origErr := os.Stdout, os.Stderr
	outR, outW, err := os.Pipe()
	require.NoError(t, err)
	errR, errW, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout, os.Stderr = outW, errW

	restored := false
	restore := func() {
		if !restored {
			os.Stdout, os.Stderr = origOut, origErr
			restored = true
		}
	}
	t.Cleanup(restore)

	return func() (string, string) {
		require.NoError(t, outW.Close())
		require.NoError(t, errW.Close())
		restore()
		var stdout, stderr bytes.Buffer
		_, _ = io.Copy(&stdout, outR)
		_, _ = io.Copy(&stderr, errR)
		return stdout.String(), stderr.String()
	}
}

// TestInitializeIOMasksSecretsRegisteredOnTheGlobalMasker guards the wiring
// that makes iolib.RegisterSecret effective. The root command's init() builds
// a separate iolib.NewContext() for the early data/UI writers; that context has
// its own masker, so secrets registered on the global masker would not be
// masked if the writers were never rebuilt. InitializeIO (the root
// PersistentPreRun) must rebind both channels to the global context.
func TestInitializeIOMasksSecretsRegisteredOnTheGlobalMasker(t *testing.T) {
	resetGlobals(t)
	const secret = "startup-registered-secret-0123456789"

	// Writers from before flag parsing, as the root's init() builds them.
	early, err := iolib.NewContext()
	require.NoError(t, err)
	ui.InitFormatter(early)
	data.InitWriter(early)

	require.NoError(t, InitializeIO(&cobra.Command{Use: "atmos"}, nil))

	// Registered after startup, like credentials resolved during a command.
	iolib.RegisterSecret(secret)

	read := captureStdStreams(t)
	require.NoError(t, data.Writeln("data: "+secret))
	ui.Writeln("ui: " + secret)
	stdout, stderr := read()

	assert.Contains(t, stdout, "data: ", "the data channel must have been written")
	assert.NotContains(t, stdout, secret, "the data channel must mask secrets registered on the global masker")
	assert.Contains(t, stderr, "ui: ", "the UI channel must have been written")
	assert.NotContains(t, stderr, secret, "the UI channel must mask secrets registered on the global masker")
}
