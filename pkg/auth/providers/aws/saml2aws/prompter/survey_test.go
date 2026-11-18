package prompter

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// useNonInteractiveStdio points the survey prompts at a closed, non-terminal stdin and a
// temporary output file for the duration of a test.
func useNonInteractiveStdio(t *testing.T) {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	require.NoError(t, w.Close())

	out, err := os.Create(filepath.Join(t.TempDir(), "prompts.txt"))
	require.NoError(t, err)

	origStdin, origOutput := os.Stdin, outputWriter
	os.Stdin = r
	SetOutputWriter(out)
	t.Cleanup(func() {
		os.Stdin = origStdin
		SetOutputWriter(origOutput)
		_ = r.Close()
		_ = out.Close()
	})
}

// Without a terminal the survey prompts cannot collect input. They must return
// promptly with zero values rather than block or panic.
func TestCliPrompterWithoutTerminal(t *testing.T) {
	useNonInteractiveStdio(t)
	cli := NewCli()

	// Atmos fork: prompt failures are propagated, never turned into a default selection or an empty value.
	code, err := cli.RequestSecurityCode("000000")
	require.Error(t, err)
	assert.Empty(t, code)

	value, err := cli.String("Region", "us-east-1")
	require.Error(t, err)
	assert.Empty(t, value)

	required, err := cli.StringRequired("Username")
	require.Error(t, err)
	assert.Empty(t, required)

	secret, err := cli.Password("Password")
	require.Error(t, err)
	assert.Empty(t, secret)

	index, err := cli.Choose("Pick", []string{"a", "b"})
	require.Error(t, err)
	assert.Equal(t, 0, index)

	selected, err := cli.ChooseWithDefault("Pick", "b", []string{"a", "b"})
	require.Error(t, err)
	assert.Empty(t, selected)
}
