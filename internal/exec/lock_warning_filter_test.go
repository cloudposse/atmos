package exec

import (
	"bytes"
	stdio "io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lockBlock returns the canonical Terraform diagnostic block for the incomplete
// lock file warning, with a single provider listed.
func lockWarningBlock() string {
	return "╷\n│ Warning: Incomplete lock file information for providers\n│ \n│ Due to your customized provider installation methods, Terraform was forced\n│ to calculate lock file checksums locally for the following providers:\n│   - hashicorp/aws\n│ \n│ The current .terraform.lock.hcl file only includes checksums for\n│ linux_amd64, so Terraform running on another platform will fail to install\n│ these providers.\n│ \n│ To calculate additional checksums for another platform, run:\n│   terraform providers lock -platform=linux_amd64\n│ (where linux_amd64 is the platform to generate)\n╵\n"
}

// otherWarningBlock returns a different diagnostic block that should NOT be suppressed.
func otherWarningBlock() string {
	return "╷\n│ Warning: Argument is deprecated\n│ \n│ The argument \"foo\" is deprecated.\n╵\n"
}

func TestIncompleteLockWarningFilter_SuppressesLockWarning(t *testing.T) {
	var buf bytes.Buffer
	f := newIncompleteLockWarningFilter(&buf)

	_, err := f.Write([]byte(lockWarningBlock()))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	assert.Empty(t, buf.String(), "lock warning block should be suppressed")
}

func TestIncompleteLockWarningFilter_PassesThroughOtherBlock(t *testing.T) {
	var buf bytes.Buffer
	f := newIncompleteLockWarningFilter(&buf)

	block := otherWarningBlock()
	_, err := f.Write([]byte(block))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	assert.Equal(t, block, buf.String(), "non-lock-warning blocks should pass through unchanged")
}

func TestIncompleteLockWarningFilter_PassesThroughNormalOutput(t *testing.T) {
	var buf bytes.Buffer
	f := newIncompleteLockWarningFilter(&buf)

	normal := "Terraform has been successfully initialized!\n\nYou may now begin working with Terraform.\n"
	_, err := f.Write([]byte(normal))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	assert.Equal(t, normal, buf.String())
}

func TestIncompleteLockWarningFilter_MixedOutput(t *testing.T) {
	var buf bytes.Buffer
	f := newIncompleteLockWarningFilter(&buf)

	before := "Initializing provider plugins...\n"
	after := "Terraform has been successfully initialized!\n"

	input := before + lockWarningBlock() + after
	_, err := f.Write([]byte(input))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	got := buf.String()
	assert.Contains(t, got, before)
	assert.Contains(t, got, after)
	assert.NotContains(t, got, "Incomplete lock file information for providers")
}

func TestIncompleteLockWarningFilter_MultipleBlocks(t *testing.T) {
	var buf bytes.Buffer
	f := newIncompleteLockWarningFilter(&buf)

	input := otherWarningBlock() + lockWarningBlock() + otherWarningBlock()
	_, err := f.Write([]byte(input))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	got := buf.String()
	// Other warning appears twice, lock warning is suppressed.
	assert.Equal(t, 2, strings.Count(got, "Warning: Argument is deprecated"))
	assert.NotContains(t, got, "Incomplete lock file information for providers")
}

func TestIncompleteLockWarningFilter_ChunkedWrites(t *testing.T) {
	var buf bytes.Buffer
	f := newIncompleteLockWarningFilter(&buf)

	// Write byte-by-byte to simulate chunked/partial writes.
	// Use index-based loop (not range) to iterate actual bytes, not runes.
	block := lockWarningBlock()
	for i := 0; i < len(block); i++ {
		_, err := f.Write([]byte{block[i]})
		require.NoError(t, err)
	}
	require.NoError(t, f.Close())

	assert.Empty(t, buf.String(), "byte-by-byte write should still suppress the lock warning")
}

func TestIncompleteLockWarningFilter_CloseFlushesIncompleteBlock(t *testing.T) {
	var buf bytes.Buffer
	f := newIncompleteLockWarningFilter(&buf)

	// Write the start of a block but never the closing ╵.
	// Close() should flush whatever was buffered.
	partial := "╷\n│ Warning: Some other warning\n│ Details here\n"
	_, err := f.Write([]byte(partial))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	assert.Equal(t, partial, buf.String(), "incomplete block should be flushed on Close")
}

func TestIncompleteLockWarningFilter_CloseFlushesPartialLine(t *testing.T) {
	var buf bytes.Buffer
	f := newIncompleteLockWarningFilter(&buf)

	// Write a line with no terminating newline.
	partial := "some output without newline"
	_, err := f.Write([]byte(partial))
	require.NoError(t, err)
	require.NoError(t, f.Close())

	assert.Equal(t, partial, buf.String(), "partial line should be flushed on Close")
}

func TestWithStderrFilter_WrapsWriter(t *testing.T) {
	var buf bytes.Buffer
	opt := WithStderrFilter(func(w stdio.Writer) stdio.Writer {
		return newIncompleteLockWarningFilter(w)
	})
	opts := &shellCommandOpts{}
	opt(opts)
	assert.NotNil(t, opts.stderrFilter)

	// Apply the filter to a buffer writer.
	filtered := opts.stderrFilter(&buf)
	assert.NotNil(t, filtered)
}
