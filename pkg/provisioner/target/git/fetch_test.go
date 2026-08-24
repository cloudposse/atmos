package git

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestFetchIntegrationReadsManagedPath publishes an artifact, then fetches it
// back from a fresh workdir (exercising reconcile's clone-if-absent and the
// managed-tree read).
func TestFetchIntegrationReadsManagedPath(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	isolatedGitEnv(t)

	root := t.TempDir()
	bare := seedBareRepo(t, root)

	deliverCfg := &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"deployments": {URI: bare, Branch: "main", Workdir: filepath.Join(root, "deliver-workdir")},
			},
		},
	}
	g := &gitProvisioner{}
	require.NoError(t, g.Deliver(context.Background(), &target.DeliverInput{
		AtmosConfig: deliverCfg,
		TargetName:  "deployment-repo",
		TargetConfig: map[string]any{
			"repository": "deployments",
			"path":       "clusters/dev/argocd",
			"commit":     map[string]any{"message": "Render argocd", "signing": "never"},
		},
		Artifact: target.ProvisionArtifact{
			Kind:   target.ArtifactKindKubernetesManifests,
			Format: target.FormatYAML,
			Files: map[string][]byte{
				"namespace.yaml":  []byte("apiVersion: v1\nkind: Namespace\n"),
				"deployment.yaml": []byte("apiVersion: apps/v1\nkind: Deployment\n"),
			},
		},
	}))

	// Fetch from a fresh workdir so reconcile clones the published repo.
	fetchCfg := &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"deployments": {URI: bare, Branch: "main", Workdir: filepath.Join(root, "fetch-workdir")},
			},
		},
	}
	artifact, err := g.Fetch(context.Background(), &target.FetchInput{
		AtmosConfig:  fetchCfg,
		TargetName:   "deployment-repo",
		TargetConfig: map[string]any{"repository": "deployments", "path": "clusters/dev/argocd"},
	})
	require.NoError(t, err)
	assert.Equal(t, target.ArtifactKindKubernetesManifests, artifact.Kind)
	assert.Equal(t, "deployment-repo", artifact.Metadata.Target)
	require.Len(t, artifact.Files, 2)
	assert.Contains(t, artifact.Files, "namespace.yaml")
	assert.Contains(t, artifact.Files, "deployment.yaml")
	assert.Equal(t, []byte("apiVersion: v1\nkind: Namespace\n"), artifact.Files["namespace.yaml"])
}

// TestFetchIntegrationEmptyBaseline fetches a path that has not been published
// yet, which must yield an empty (not errored) baseline.
func TestFetchIntegrationEmptyBaseline(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not available")
	}
	isolatedGitEnv(t)

	root := t.TempDir()
	bare := seedBareRepo(t, root)

	cfg := &schema.AtmosConfiguration{
		Git: schema.GitConfig{
			Repositories: map[string]schema.GitRepository{
				"deployments": {URI: bare, Branch: "main", Workdir: filepath.Join(root, "workdir")},
			},
		},
	}
	g := &gitProvisioner{}
	artifact, err := g.Fetch(context.Background(), &target.FetchInput{
		AtmosConfig:  cfg,
		TargetName:   "deployment-repo",
		TargetConfig: map[string]any{"repository": "deployments", "path": "clusters/dev/argocd"},
	})
	require.NoError(t, err)
	assert.Empty(t, artifact.Files)
}
