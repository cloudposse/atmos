package adoption

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/terraform/backend"
)

func TestSharedNativeTarget(t *testing.T) {
	r := registration(t)
	r.Explicit = terraform
	require.NoError(t, r.Save(t.Context()))
	r.Stack = "prod"
	for _, dry := range []bool{true, false} {
		r.DryRun = dry
		err := r.Save(t.Context())
		require.ErrorContains(t, err, "component net in Atmos stack dev (workspace default)")
		require.ErrorContains(t, err, "--share-state")
		require.NoFileExists(t, filepath.Join(r.Project, "stacks", "prod.yaml"))
	}
	r.ShareState = true
	require.NoError(t, r.Save(t.Context()))
	r.ShareState = false
	require.NoError(t, r.Save(t.Context()))
	r.Stack = "next"
	r.Workspace = "blue"
	require.ErrorContains(t, r.Save(t.Context()), "does not exist")
	require.NoError(t, os.MkdirAll(filepath.Join(r.Dir, "terraform.tfstate.d", "blue"), 0o755))
	require.NoError(t, r.Save(t.Context()))
	// Retrying a previously bound instance uses its saved workspace.
	t.Setenv("TF_WORKSPACE", "missing")
	r.Workspace = ""
	require.NoError(t, r.Save(t.Context()))
}

func TestStateTargetChoice(t *testing.T) {
	r := registration(t)
	r.Explicit = terraform
	require.NoError(t, r.Save(t.Context()))
	r.Stack = "new"
	r.Interactive = true
	require.NoError(t, os.MkdirAll(filepath.Join(r.Dir, "terraform.tfstate.d", "blue"), 0o755))
	prev := promptStateTarget
	t.Cleanup(func() { promptStateTarget = prev })
	promptStateTarget = func(_, title string, options []string) (string, error) {
		require.Contains(t, title, "Atmos stack dev")
		require.Contains(t, options, "blue")
		return "blue", nil
	}
	require.NoError(t, r.Save(t.Context()))
	r.Stack = "shared"
	promptStateTarget = func(_, _ string, options []string) (string, error) { return options[len(options)-1], nil }
	require.NoError(t, r.Save(t.Context()))
	r.Stack = "cancel"
	promptStateTarget = func(_, _ string, _ []string) (string, error) { return "", errUtils.ErrUserAborted }
	require.ErrorIs(t, r.Save(t.Context()), errUtils.ErrUserAborted)
	require.NoFileExists(t, filepath.Join(r.Project, "stacks", "cancel.yaml"))
}

func TestSharedBackendAcrossRoots(t *testing.T) {
	r := registration(t)
	r.Explicit = tofu
	shared := filepath.Join(r.Project, "shared")
	require.NoError(t, os.Mkdir(shared, 0o755))
	alias := filepath.Join(r.Project, "alias")
	if err := os.Symlink(shared, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	config := func(dir, path string) {
		write(t, filepath.Join(dir, "main.tf"), fmt.Sprintf("terraform {\n backend \"local\" { path = %q }\n}", filepath.ToSlash(filepath.Join(path, "missing", "state"))))
	}
	config(r.Dir, alias)
	require.NoError(t, r.Save(t.Context()))
	r.Dir = filepath.Join(r.Project, "other")
	config(r.Dir, shared)
	r.Explicit = terraform
	r.Stack = "other"
	require.ErrorContains(t, r.Save(t.Context()), "same local state")
}

func TestStateIndexErrors(t *testing.T) {
	r := registration(t)
	base := r.Project
	file := filepath.Join(base, "manifest.yaml")
	owners := map[string]string{}
	write(t, file, "name: renamed\ncomponents:\n  terraform:\n    skipped: {}\n    sample:\n      metadata: {component: infra/net, terraform_workspace: default}\n")
	require.NoError(t, indexManifestStates(file, base, owners))
	require.Len(t, owners, 1)
	for _, v := range owners {
		require.Contains(t, v, "stack renamed")
	}
	write(t, filepath.Join(r.Dir, "main.tf"), "terraform {\n backend \"s3\" {}\n}")
	require.NoError(t, indexManifestStates(file, base, map[string]string{}))
	write(t, filepath.Join(r.Dir, "main.tf"), "terraform {")
	require.Error(t, indexManifestStates(file, base, owners))
	write(t, file, "import: [shared]")
	require.NoError(t, indexManifestStates(file, base, owners))
	write(t, file, "[")
	require.Error(t, indexManifestStates(file, base, owners))
	require.Error(t, indexManifestStates(r.Dir, base, owners))
	write(t, filepath.Join(base, "file"), "")
	r.Interactive = true
	_, err := r.chooseStateTarget(backend.LocalState{Workspaces: filepath.Join(base, "file", "dir")}, "default", "owner")
	require.Error(t, err)
}

func TestConcurrentSharedTarget(t *testing.T) {
	r := registration(t)
	r.Explicit = terraform
	second := r
	second.Stack = "prod"
	ctx := context.Background()
	var group sync.WaitGroup
	results := make(chan error, 2)
	for _, req := range []request{r, second} {
		group.Go(func() { results <- req.Save(ctx) })
	}
	group.Wait()
	first, other := <-results, <-results
	if first == nil {
		first, other = other, first
	}
	require.NoError(t, other)
	require.ErrorContains(t, first, "--share-state")
}
