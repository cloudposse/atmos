package ui

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
)

// resetFormatterForTest clears the global formatter and restores it when the test ends.
func resetFormatterForTest(t *testing.T) {
	t.Helper()

	formatterMu.Lock()
	oldFormatter := globalFormatter
	oldIO := globalIO
	globalFormatter = nil
	globalIO = nil
	formatterMu.Unlock()

	t.Cleanup(func() {
		formatterMu.Lock()
		globalFormatter = oldFormatter
		globalIO = oldIO
		formatterMu.Unlock()
	})
}

func TestOutput_Markdown_RendersHeadingToWriter(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	_, stderr, cleanup := setupTestUI(t)
	defer cleanup()

	var buf bytes.Buffer
	New(&buf).Markdown("# Output Heading\n\nSome body text.")

	assert.Contains(t, buf.String(), "Output Heading")
	assert.Contains(t, buf.String(), "Some body text.")
	assert.Equal(t, byte('\n'), buf.Bytes()[buf.Len()-1], "output should end with a newline")
	assert.Empty(t, stderr.String(), "explicit writer must not fall back to the UI channel")
}

func TestOutput_Markdown_NilWriterUsesUIChannel(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	_, stderr, cleanup := setupTestUI(t)
	defer cleanup()

	New(nil).Markdown("# Channel Heading")

	assert.Contains(t, stderr.String(), "Channel Heading")
}

func TestOutput_Markdown_MasksSecrets(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	_, _, cleanup := setupTestUI(t)
	defer cleanup()

	const secret = "markdown-secret-value-8271"
	iolib.RegisterSecret(secret)
	globalIO.Masker().RegisterSecret(secret)

	var buf bytes.Buffer
	New(&buf).Markdown("# Report\n\nToken: " + secret)

	assert.Contains(t, buf.String(), "Report")
	assert.NotContains(t, buf.String(), secret)
	assert.Contains(t, buf.String(), "MASKED")
}

func TestOutput_Markdown_FormatterNotInitialized(t *testing.T) {
	resetFormatterForTest(t)

	const secret = "uninitialized-secret-value-5519"
	iolib.RegisterSecret(secret)

	t.Run("explicit writer receives masked plain content", func(t *testing.T) {
		var buf bytes.Buffer
		New(&buf).Markdown("# Plain Heading\n\nToken: " + secret)

		assert.Contains(t, buf.String(), "# Plain Heading")
		assert.NotContains(t, buf.String(), secret)
		assert.Contains(t, buf.String(), "MASKED")
	})

	t.Run("nil writer drops content without panicking", func(t *testing.T) {
		assert.NotPanics(t, func() { New(nil).Markdown("# Dropped") })
	})
}

func TestOutput_ErrorAndWritef(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	_, stderr, cleanup := setupTestUI(t)
	defer cleanup()

	t.Run("Error writes a status line", func(t *testing.T) {
		var buf bytes.Buffer
		New(&buf).Error("boom happened")

		assert.Contains(t, buf.String(), "boom happened")
		assert.NotEqual(t, "boom happened\n", buf.String(), "Error should include an icon")
		assert.Equal(t, byte('\n'), buf.Bytes()[buf.Len()-1])
	})

	t.Run("Errorf formats the message", func(t *testing.T) {
		var buf bytes.Buffer
		New(&buf).Errorf("failed %d of %s", 3, "jobs")

		assert.Contains(t, buf.String(), "failed 3 of jobs")
	})

	t.Run("Writef has no icon or trailing newline", func(t *testing.T) {
		var buf bytes.Buffer
		New(&buf).Writef("plain %s", "text")

		assert.Equal(t, "plain text", buf.String())
	})

	t.Run("Writef masks secrets", func(t *testing.T) {
		const secret = "writef-secret-value-3304"
		globalIO.Masker().RegisterSecret(secret)

		var buf bytes.Buffer
		New(&buf).Writef("value=%s", secret)

		assert.NotContains(t, buf.String(), secret)
		assert.Contains(t, buf.String(), "MASKED")
	})

	t.Run("nil writer uses the UI channel", func(t *testing.T) {
		New(nil).Errorf("fallback %s", "error")
		New(nil).Writef("fallback %s", "write")

		assert.Contains(t, stderr.String(), "fallback error")
		assert.Contains(t, stderr.String(), "fallback write")
	})
}

func TestOutput_Writef_FormatterNotInitialized(t *testing.T) {
	resetFormatterForTest(t)

	var buf bytes.Buffer
	New(&buf).Writef("hello %s", "world")
	assert.Equal(t, "hello world", buf.String())

	assert.NotPanics(t, func() {
		New(nil).Writef("dropped")
		New(nil).Error("dropped")
		New(nil).Errorf("dropped %d", 1)
	})
	require.Equal(t, "hello world", buf.String())
}
