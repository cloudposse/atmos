package cmd

import (
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestPluginVerifyPublicCLI(t *testing.T) {
	_ = NewTestKit(t)
	config := pluginCoverageProject(t)
	server := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	defer server.Close()
	ref, err := name.NewTag(server.Listener.Addr().String() + "/example/catalog:stable")
	require.NoError(t, err)
	require.NoError(t, remote.Write(ref, empty.Image))
	digest, err := empty.Image.Digest()
	require.NoError(t, err)
	_, err = invokePluginCoverage(t, config, "verify-public", ref.Context().Name(), "--reference", "stable", "--digest", digest.String())
	require.NoError(t, err)
	_, err = invokePluginCoverage(t, config, "verify-public", ref.Context().Name(), "--digest", digest.String())
	require.ErrorIs(t, err, errUtils.ErrPluginFlagConflict)
	_, err = invokePluginCoverage(t, config, "verify-public", ref.Context().Name())
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	_, err = invokePluginCoverage(t, config, "verify-public", ref.Context().Name(), "--reference", "missing")
	require.Error(t, err)
}

func TestPluginCatalogSyncCLIReadsCommittedInventory(t *testing.T) {
	_ = NewTestKit(t)
	config := pluginCoverageProject(t)
	server := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	defer server.Close()
	directory := t.TempDir()
	repository, err := git.PlainInit(directory, false)
	require.NoError(t, err)
	// Catalog reconciliation needs neither native CI runner mappings nor build history.
	files := map[string]string{
		"inventory.yaml": "packages:\n  example:\n    path: example\n    repository: oci://" + server.Listener.Addr().String() + "/example/plugin\n",
		"example/plugin.yaml": `apiVersion: atmos/v1
kind: AtmosPluginConfig
metadata:
  name: team/example
  version: 1.0.0
spec:
  runtime:
    type: native
    protocol: 1
    executable: bin/plugin
  platforms:
    linux/amd64:
      executable: bin/plugin
  compatibility:
    protocol: '>=1 <2'
  capabilities: {}
`,
	}
	for relative, content := range files {
		filename := filepath.Join(directory, filepath.FromSlash(relative))
		require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o700))
		require.NoError(t, os.WriteFile(filename, []byte(content), 0o600))
	}
	worktree, err := repository.Worktree()
	require.NoError(t, err)
	require.NoError(t, worktree.AddWithOptions(&git.AddOptions{All: true}))
	head, err := worktree.Commit("accepted inventory", &git.CommitOptions{Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Unix(1, 0)}})
	require.NoError(t, err)
	for relative := range files {
		require.NoError(t, os.WriteFile(filepath.Join(directory, filepath.FromSlash(relative)), []byte("invalid working copy"), 0o600))
	}
	t.Chdir(filepath.Join(directory, "example"))
	target := "oci://" + server.Listener.Addr().String() + "/example/catalog:stable"
	_, err = invokePluginCoverage(t, config, "catalog", "sync", target, "--inventory", "inventory.yaml", "--head", head.String(), "--skip-missing")
	require.ErrorIs(t, err, errUtils.ErrPluginVersionNotFound, "committed inventory must reach registry reconciliation without a build plan")
	_, err = invokePluginCoverage(t, config, "catalog", "sync", target, "--inventory", "inventory.yaml", "--verify-public")
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	_, err = invokePluginCoverage(t, config, "catalog", "sync", "bad alias", "--inventory", "inventory.yaml")
	require.Error(t, err)
	_, err = invokePluginCoverage(t, config, "catalog", "sync", target, "--inventory", "missing.yaml")
	require.Error(t, err)
}
