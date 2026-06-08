package main

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/store"
)

// TestMainHooksAndKeychainStoreIntegration proves end-to-end, with no cloud credentials and no
// running server, that an after-terraform-apply store hook writes a Terraform output into an
// encrypted-at-rest secrets store (the keychain `file` backend).
//
// It deploys a component whose hook stores the output `.random`, then asserts both that the value
// round-trips through the store API and that the on-disk keyring file is encrypted (the plaintext
// never appears). A keychain store is `secret: true` by default, so the written value is reachable
// only via the declarative `!secret` function, never `!store`; this test deliberately verifies the
// write path (the subject of the question) rather than a cross-component read-back.
//
// Isolation is purely env-driven: XDG_DATA_HOME points the keyring file at a temp dir, and
// ATMOS_KEYRING_PASSWORD drives the file backend non-interactively (see pkg/keyring/file.go).
func TestMainHooksAndKeychainStoreIntegration(t *testing.T) {
	// Skip gracefully if no terraform binary. Atmos invokes `terraform` by default (the fixture
	// does not override `components.terraform.command`), so that is the binary that must exist.
	if _, err := exec.LookPath("terraform"); err != nil {
		t.Skip("terraform not available: required for hook+store integration test")
	}

	// Credential-free, server-free encrypted secrets store.
	xdgDataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDataHome)                // isolates the keyring file.
	t.Setenv("ATMOS_KEYRING_PASSWORD", "atmos-test-pass") // >= 8 chars, read non-interactively.

	// Disable CI auto-detection so deploy hooks don't try to download planfiles from GitHub
	// Artifacts during tests. t.Setenv restores the prior value automatically at test end.
	t.Setenv("GITHUB_ACTIONS", "")

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}
	defer os.RemoveAll(filepath.Join(origDir, "tests", "fixtures", "scenarios", "hooks-keychain-test", ".terraform"))

	t.Chdir("tests/fixtures/scenarios/hooks-keychain-test")

	// This integration test calls run() (not main()) to avoid os.Exit() which panics in Go 1.25+.
	// run() returns an exit code instead of calling os.Exit().
	// We manipulate os.Args since run() uses cmd.Execute() which reads os.Args internally.
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Deploy `component1`, whose after-terraform-apply hook writes the terraform output `.random`
	// into the keychain file store under key `random_id`.
	os.Args = []string{"atmos", "terraform", "deploy", "component1", "-s", "test"}
	if exitCode := run(); exitCode != 0 {
		t.Fatalf("component1 deploy returned non-zero exit code: %d", exitCode)
	}

	// Round-trip: read the value back through a keychain store constructed with the same options.
	// This asserts the hook persisted the exact content, not merely that a deploy succeeded.
	s, err := store.NewKeychainStore(&store.KeychainStoreOptions{Backend: "file"})
	require.NoError(t, err)
	got, err := s.Get("test", "component1", "random_id")
	require.NoError(t, err)
	require.Equal(t, "random1", got)

	// Encrypted at rest: the secret value must not appear in plaintext anywhere in the keyring
	// file the keychain `file` backend wrote under XDG_DATA_HOME.
	requireEncryptedAtRest(t, xdgDataHome, "random1")
}

// requireEncryptedAtRest fails if the plaintext appears in any file under dir, and fails if no
// files were written at all (which would make the check vacuous).
func requireEncryptedAtRest(t *testing.T, dir, plaintext string) {
	t.Helper()

	fileCount := 0
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		fileCount++
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		require.NotContains(t, string(data), plaintext,
			"plaintext secret found in keyring file %q — value is not encrypted at rest", path)
		return nil
	})
	require.NoError(t, err)
	require.Positive(t, fileCount, "no keyring files were written under %q; encryption check would be vacuous", dir)
}
