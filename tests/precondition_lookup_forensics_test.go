package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDescribeToolchainPathEntry covers describeToolchainPathEntry's on-disk
// state reporting: target presence (found/missing/matched via an alternate
// candidate name), directory contents listing, and writer-lock sibling
// detection. These are the exact forensic signals CI uses to tell apart "tool
// deleted", "writer mid-flight", and "PATH entry just doesn't have it" -- see
// the function's doc comment.
func TestDescribeToolchainPathEntry(t *testing.T) {
	tests := []struct {
		name          string
		writeFiles    []string // files to create in the entry dir
		writeLock     bool     // whether to create a "<dir>.lock" sibling
		candidates    []string
		wantSubstr    []string
		wantNotSubstr []string
	}{
		{
			name:       "target present, no lock",
			writeFiles: []string{"tofu"},
			candidates: []string{"tofu"},
			wantSubstr: []string{"target present=true", "contents(1): tofu"},
			wantNotSubstr: []string{
				"WRITER LOCK PRESENT",
				"dir unreadable",
			},
		},
		{
			name:       "target absent",
			writeFiles: []string{"some-other-file"},
			candidates: []string{"tofu"},
			wantSubstr: []string{"target present=false", "contents(1): some-other-file"},
		},
		{
			name:       "target matched via alternate Windows exe candidate",
			writeFiles: []string{"tofu.exe"},
			candidates: []string{"tofu", "tofu.exe"},
			wantSubstr: []string{"target present=true"},
		},
		{
			name:       "writer lock sibling present",
			writeFiles: []string{"tofu"},
			writeLock:  true,
			candidates: []string{"tofu"},
			wantSubstr: []string{"WRITER LOCK PRESENT"},
		},
		{
			name:       "empty directory has no contents",
			candidates: []string{"tofu"},
			wantSubstr: []string{"target present=false", "contents(0):"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tt.writeFiles {
				require.NoError(t, os.WriteFile(filepath.Join(dir, f), []byte("fake\n"), 0o755))
			}
			if tt.writeLock {
				lockPath := dir + ".lock"
				require.NoError(t, os.WriteFile(lockPath, []byte("locked\n"), 0o644))
				t.Cleanup(func() { _ = os.Remove(lockPath) })
			}

			var b strings.Builder
			describeToolchainPathEntry(&b, dir, tt.candidates)
			got := b.String()

			assert.True(t, strings.HasSuffix(got, "\n"), "report should end with a newline: %q", got)
			assert.Contains(t, got, dir+" -> target present=")
			for _, want := range tt.wantSubstr {
				assert.Contains(t, got, want)
			}
			for _, notWant := range tt.wantNotSubstr {
				assert.NotContains(t, got, notWant)
			}
		})
	}
}

// TestExecutableLookupForensics covers the PATH-scanning behavior:
// non-toolchain-like entries must be ignored entirely, every toolchain-like
// entry must be described, and the "no toolchain dirs at all" case must be
// reported distinctly -- that message is how CI tells "PATH itself lost the
// toolchain dirs" apart from "toolchain dir present but tool missing".
func TestExecutableLookupForensics(t *testing.T) {
	t.Run("no matching entries reports PATH corruption case", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("PATH", dir)

		got := executableLookupForensics("tofu")
		assert.Contains(t, got, "NO toolchain-like PATH entries found")
		assert.Contains(t, got, `lookup forensics for "tofu"`)
		assert.Contains(t, got, "(1 PATH entries)")
	})

	t.Run("matching entry is described", func(t *testing.T) {
		toolchainDir := filepath.Join(t.TempDir(), "toolchain", "bin")
		require.NoError(t, os.MkdirAll(toolchainDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(toolchainDir, "tofu"), []byte("fake\n"), 0o755))
		t.Setenv("PATH", toolchainDir)

		got := executableLookupForensics("tofu")
		assert.Contains(t, got, "target present=true")
		assert.NotContains(t, got, "NO toolchain-like")
	})

	t.Run("unrelated entries are ignored, matching entry still found", func(t *testing.T) {
		unrelatedDir := t.TempDir()
		toolchainDir := filepath.Join(t.TempDir(), "toolchain", "bin")
		require.NoError(t, os.MkdirAll(toolchainDir, 0o755))
		t.Setenv("PATH", unrelatedDir+string(os.PathListSeparator)+toolchainDir)

		got := executableLookupForensics("tofu")
		assert.NotContains(t, got, unrelatedDir)
		assert.Contains(t, got, toolchainDir)
		assert.Contains(t, got, "target present=false")
		assert.Contains(t, got, "(2 PATH entries)")
	})
}
