package mirror

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMirrorScanner verifies the providers-mirror output parser emits one event per
// provider/platform package, including across chunked writes.
func TestMirrorScanner(t *testing.T) {
	var got []pkgEvent
	s := newMirrorScanner(func(ev pkgEvent) { got = append(got, ev) })

	// Write in two chunks, splitting mid-line, to exercise buffering.
	chunk1 := "- Mirroring hashicorp/random...\n" +
		"  - Selected v3.9.0 to match dependency lock file\n" +
		"  - Downloading package for linux_amd64...\n" +
		"  - Package authenticated: signed\n" +
		"  - Downloading package for darwin_ar"
	chunk2 := "ch64...\n" +
		"  - Package authenticated: signed\n" +
		"- Mirroring hashicorp/null...\n" +
		"  - Downloading package for windows_amd64...\n" +
		"  - Package authenticated: signed\n"

	_, _ = s.Write([]byte(chunk1))
	_, _ = s.Write([]byte(chunk2))

	want := []pkgEvent{
		{provider: "hashicorp/random", platform: "linux_amd64"},
		{provider: "hashicorp/random", platform: "darwin_arch64"},
		{provider: "hashicorp/null", platform: "windows_amd64"},
	}
	assert.Equal(t, want, got)
	assert.Equal(t, 3, s.count)
}

// TestMirrorScannerNoPackages verifies count stays 0 when no package lines appear,
// which triggers the per-component fallback line in the model.
func TestMirrorScannerNoPackages(t *testing.T) {
	var got []pkgEvent
	s := newMirrorScanner(func(ev pkgEvent) { got = append(got, ev) })
	_, _ = s.Write([]byte("Initializing the backend...\nNothing to mirror.\n"))
	assert.Empty(t, got)
	assert.Equal(t, 0, s.count)
}
