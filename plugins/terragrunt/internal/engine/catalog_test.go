package engine

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// Use Git's dumb HTTP protocol so the real Terragrunt downloader, cache, stack
// generator and module provenance run without cloud credentials or a public network.
func TestPinnedRemoteCatalog(t *testing.T) {
	repo := t.TempDir()
	write := func(root, name, content string) {
		p := filepath.Join(root, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o700))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	}
	git := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	write(repo, "stacks/environment/terragrunt.stack.hcl", `unit "db" {
 source="../../units/db"
 update_source_with_cas=true
 path="database"
 values={name=values.name}
}`)
	write(repo, "units/db/terragrunt.hcl", `terraform {
 source="../..//modules/db"
 update_source_with_cas=true
}
inputs={name=values.name}
`)
	write(repo, "modules/db/main.tf", "variable \"name\" {}\n")
	write(repo, "units/effect/terragrunt.hcl", `locals { module = run_cmd("must-not-execute") }
terraform { source = local.module }
`)
	git("add", ".")
	git("-c", "commit.gpgsign=false", "-c", "user.email=test@example.org", "-c", "user.name=Test", "commit", "-qm", "catalog")
	pin := git("rev-parse", "HEAD")
	git("update-server-info")
	server := httptest.NewServer(http.FileServer(http.Dir(filepath.Join(repo, ".git"))))
	defer server.Close()
	uri := "git::" + server.URL
	root := t.TempDir()
	write(root, "live/terragrunt.stack.hcl", fmt.Sprintf(`stack "env" {
  source=%q
  path="env"
  values={name="app"}
 }`, uri+"//stacks/environment?ref="+pin))
	request := model.Request{Phase: "inventory", BasePath: root, Catalogs: map[string]string{uri + "?ref=" + pin: pin}, Environment: map[string]string{"PATH": os.Getenv("PATH"), "XDG_CACHE_HOME": t.TempDir()}}
	check := func() {
		t.Helper()
		res, err := Resolve(t.Context(), request)
		require.NoError(t, err)
		require.Empty(t, res.Failures)
		require.Len(t, res.Inventory, 1)
		unit := res.Inventory[0]
		require.Equal(t, "env/db", unit.Name)
		require.Equal(t, uri+"//modules/db?ref="+pin, unit.Config.Source)
		require.NoDirExists(t, filepath.Join(root, "live", ".terragrunt-stack"))
	}
	// Without an authorized pin the same HCL cannot populate the catalog cache.
	denied := request
	denied.Catalogs = nil
	denied.Environment = map[string]string{"PATH": os.Getenv("PATH"), "XDG_CACHE_HOME": t.TempDir()}
	result, err := Resolve(t.Context(), denied)
	require.NoError(t, err)
	require.NotEmpty(t, result.Failures)
	require.NoDirExists(t, filepath.Join(denied.Environment["XDG_CACHE_HOME"], "atmos"))
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = Resolve(cancelled, request)
	require.ErrorIs(t, err, context.Canceled)
	// Callers that start together must both see a complete snapshot.
	done := make(chan error, 2)
	for range 2 {
		go func() {
			res, err := Resolve(t.Context(), request)
			if err == nil && (len(res.Failures) != 0 || len(res.Inventory) != 1) {
				err = fmt.Errorf("incomplete inventory: %v", res.Failures)
			}
			done <- err
		}()
	}
	for range 2 {
		require.NoError(t, <-done)
	}
	check()
	server.Close()
	check()
	request.Phase = "discover"
	request.Command = "generate"
	request.Path = filepath.Join(root, "live", "terragrunt.stack.hcl")
	_, err = Resolve(t.Context(), request)
	require.NoError(t, err)
	request.Path = filepath.Join(root, "live", ".terragrunt-stack", "env", ".terragrunt-stack", "database", "terragrunt.hcl")
	require.FileExists(t, request.Path)
	request.Command = "plan"
	request.Phase = "prepare"
	request.Tofu = "must-not-execute"
	prepared, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "app", prepared.Inputs["name"])
	require.Equal(t, uri+"//modules/db?ref="+pin, prepared.Source)
	// A warm, authorized catalog still cannot run HCL commands during discovery.
	write(root, "other/terragrunt.stack.hcl", fmt.Sprintf(`unit "effect" {
   source=%q
   path="effect"
 }`, uri+"//units/effect?ref="+pin))
	request.Path = ""
	request.Phase = "inventory"
	request.Command = ""
	result, err = Resolve(t.Context(), request)
	require.NoError(t, err)
	require.NotEmpty(t, result.Failures)
	require.Len(t, result.Inventory, 1)
}
