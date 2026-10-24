package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunDefaultsOutputPathUnderRoot covers run()'s argument handling: with
// only a root argument, it must write NOTICE under that root rather than
// the current working directory.
func TestRunDefaultsOutputPathUnderRoot(t *testing.T) {
	stubDir := buildStubGoLicenses(t, "example.com/dep,https://example.com/dep/LICENSE,MIT\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := t.TempDir()
	writeTestModule(t, root)

	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })
	os.Args = []string{"noticegen", root}

	require.NoError(t, runNotice(stubDescription))

	_, err := os.Stat(filepath.Join(root, "NOTICE"))
	require.NoError(t, err)
}

// TestRunExplicitOutputPath covers run()'s second (output path) argument.
func TestRunExplicitOutputPath(t *testing.T) {
	stubDir := buildStubGoLicenses(t, "example.com/dep,https://example.com/dep/LICENSE,MIT\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := t.TempDir()
	writeTestModule(t, root)
	outPath := filepath.Join(t.TempDir(), "CUSTOM_NOTICE")

	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })
	os.Args = []string{"noticegen", root, outPath}

	require.NoError(t, runNotice(stubDescription))

	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "example.com/dep")
}

// TestRunPropagatesGenerateError covers run() itself (not the injected
// runNotice core): an empty PATH fails at ensureGoLicenses, before the
// network-dependent description fetch runs, so this exercises run()'s
// production closure without ever making a real GitHub API call.
func TestRunPropagatesGenerateError(t *testing.T) {
	t.Setenv("PATH", "")

	origArgs := os.Args
	t.Cleanup(func() { os.Args = origArgs })
	os.Args = []string{"noticegen", t.TempDir()}

	require.Error(t, run())
}
