package ui

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	iolib "github.com/cloudposse/atmos/pkg/io"
)

// TestReinitFormatterKeepsGlobalMasker verifies that re-creating the formatter
// (after cast recording or `--ai` output capture) keeps masking secrets that
// were registered on the global masker with iolib.RegisterSecret. A fresh
// iolib.NewContext() has its own masker that does not know those secrets.
func TestReinitFormatterKeepsGlobalMasker(t *testing.T) {
	formatterMu.Lock()
	oldFormatter, oldIO, oldTerminal := globalFormatter, globalIO, globalTerminal
	formatterMu.Unlock()
	t.Cleanup(func() {
		formatterMu.Lock()
		globalFormatter, globalIO, globalTerminal = oldFormatter, oldIO, oldTerminal
		formatterMu.Unlock()
	})

	const secret = "reinit-registered-secret-0123456789"
	iolib.RegisterSecret(secret)

	origErr := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = origErr })

	ReinitFormatter()
	Writeln("ui: " + secret)

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stderr = origErr
	var stderr bytes.Buffer
	_, _ = io.Copy(&stderr, r)

	got := stderr.String()
	if !strings.Contains(got, "ui: ") {
		t.Fatalf("expected the UI channel to be written, got %q", got)
	}
	if strings.Contains(got, secret) {
		t.Errorf("ReinitFormatter must keep masking secrets registered on the global masker, got %q", got)
	}
}
