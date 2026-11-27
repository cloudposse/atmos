package terraformadoption

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func registration(t *testing.T) Registration {
	t.Helper()
	project := t.TempDir()
	root := filepath.Join(project, "infra", "net")
	write(t, filepath.Join(root, "main.tf"), "")
	binaries(t, "terraform", "tofu")
	t.Setenv("TF_WORKSPACE", "")
	t.Setenv("TF_DATA_DIR", "")
	t.Setenv("ATMOS_COMPONENTS_TERRAFORM_COMMAND", "")
	return Registration{Project: project, Dir: root, Stack: "dev", Output: io.Discard}
}

func TestRegistration(t *testing.T) {
	r := registration(t)
	file := filepath.Join(r.Project, "stacks", "dev.yaml")
	r.DryRun = true
	require.ErrorContains(t, r.Save(t.Context()), choiceHint)
	require.NoFileExists(t, file)
	r.Explicit = "tofu"
	var preview bytes.Buffer
	r.Output = &preview
	require.NoError(t, r.Save(t.Context()))
	require.Contains(t, preview.String(), "Engine: tofu")
	require.Contains(t, preview.String(), "+++ b/atmos.yaml")
	require.Contains(t, preview.String(), "+++ b/stacks/dev.yaml")
	require.Contains(t, preview.String(), "+      command: tofu")
	require.NoFileExists(t, filepath.Join(r.Project, "atmos.yaml"))
	r.DryRun = false
	write(t, filepath.Join(r.Dir, ".tool-versions"), "opentofu 8.7.6")
	write(t, filepath.Join(r.Dir, ".terraform", "environment"), "blue")
	require.ErrorContains(t, r.Save(t.Context()), "does not exist")
	require.NoError(t, os.MkdirAll(filepath.Join(r.Dir, "terraform.tfstate.d", "blue"), 0o700))
	require.NoError(t, r.Save(t.Context()))
	before, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Contains(t, string(before), "terraform_workspace: blue")
	require.Contains(t, string(before), "component: infra/net")
	require.Contains(t, string(before), "command: tofu")
	require.Contains(t, string(before), "8.7.6")
	var c schema.AtmosConfiguration
	c.Stacks.IncludedPaths = []string{"*.yaml"}
	c.BasePathAbsolute = r.Project
	c.StacksBaseAbsolutePath = filepath.Dir(file)
	c.TerraformDirAbsolutePath = r.Project
	c.Components.Terraform.Command = "terraform"
	r.Config = &c
	r.Explicit = ""
	require.NoError(t, r.Save(t.Context()))
	after, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Equal(t, before, after)
	r.Explicit = "terraform"
	require.ErrorContains(t, r.Save(t.Context()), "already exists")
	r.Explicit = ""
	t.Setenv("TF_WORKSPACE", "green")
	r.Stack = "prod"
	require.NoError(t, os.MkdirAll(filepath.Join(r.Dir, "terraform.tfstate.d", "green"), 0o700))
	require.NoError(t, r.Save(t.Context()))
	prod, err := os.ReadFile(filepath.Join(filepath.Dir(file), "prod.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(prod), "terraform_workspace: green")
}

func TestRegistrationRefusesConflicts(t *testing.T) {
	r := registration(t)
	r.Explicit = "tofu"
	r.DryRun = true
	file := filepath.Join(r.Project, "stacks", "dev.yaml")
	r.Stack = "../dev"
	require.ErrorContains(t, r.Save(t.Context()), "stack name")
	r.Stack = "dev"
	for _, body := range []string{"name: prod", "import: [shared]", "vars: {region: west}", "components: ["} {
		write(t, file, body)
		require.Error(t, r.Save(t.Context()))
		actual, err := os.ReadFile(file)
		require.NoError(t, err)
		require.Equal(t, body, string(actual))
	}
	require.NoError(t, os.Remove(file))
	require.NoError(t, os.Mkdir(file, 0o700))
	require.ErrorContains(t, r.Save(t.Context()), "regular file")
	require.NoError(t, os.Remove(file))
	r.Config = &schema.AtmosConfiguration{}
	require.ErrorContains(t, r.Save(t.Context()), "filename-named")
	r.Config.Stacks.IncludedPaths = []string{"*.yaml"}
	r.Config.Components.Terraform.AutoGenerateBackendFile = true
	require.ErrorContains(t, r.Save(t.Context()), "backend")
	r.Config.Components.Terraform.AutoGenerateBackendFile = false
	r.Config.TerraformDirAbsolutePath = t.TempDir()
	require.ErrorContains(t, r.Save(t.Context()), "outside")
	r.Config = nil
	r.DryRun = false
	write(t, filepath.Join(r.Project, "atmos.yaml"), "# existing")
	require.ErrorContains(t, r.Save(t.Context()), "reload and retry")
	require.NoFileExists(t, file)
	require.NoError(t, os.Remove(filepath.Join(r.Project, "atmos.yaml")))
	envFile := filepath.Join(r.Dir, "data", "environment")
	t.Setenv("TF_DATA_DIR", "data")
	write(t, envFile, "\n")
	require.ErrorContains(t, r.Save(t.Context()), "empty native workspace")
	require.NoError(t, os.Remove(envFile))
	require.NoError(t, os.Mkdir(envFile, 0o700))
	require.Error(t, r.Save(t.Context()))
}

func TestConcurrentRegistration(t *testing.T) {
	r := registration(t)
	r.Explicit = "terraform"
	other := r
	other.Dir = filepath.Join(r.Project, "db")
	write(t, filepath.Join(other.Dir, "other.tf"), "")
	ctx := t.Context()
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, req := range []Registration{r, other} {
		wg.Go(func() { errs <- req.Save(ctx) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	raw, err := os.ReadFile(filepath.Join(r.Project, "stacks", "dev.yaml"))
	require.NoError(t, err)
	var manifest struct {
		Components struct{ Terraform map[string]any }
	}
	require.NoError(t, yaml.Unmarshal(raw, &manifest))
	require.Len(t, manifest.Components.Terraform, 2)
}

func TestStandaloneDestination(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "west.yaml")
	write(t, file, "name: dev")
	path, err := standaloneDestination(dir, "dev")
	require.NoError(t, err)
	require.Equal(t, file, path)
	write(t, filepath.Join(dir, "ignored.txt"), "{")
	path, err = standaloneDestination(dir, "new")
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "new.yaml"), path)
	write(t, filepath.Join(dir, "dev.yaml"), "{}")
	_, err = standaloneDestination(dir, "dev")
	require.ErrorContains(t, err, "both")
	_, err = standaloneDestination(file, "dev")
	require.Error(t, err)
}

func TestVerifiedWorkspace(t *testing.T) {
	r := registration(t)
	cmd, err := Select(t.Context(), r.Dir, r.Project, Selection{Explicit: tofu})
	require.NoError(t, err)
	cmd.Command = cmd.Path
	write(t, filepath.Join(r.Dir, "main.tofu"), "")
	for _, name := range []string{".", "../green"} {
		t.Setenv("TF_WORKSPACE", name)
		_, err = verifiedWorkspace(t.Context(), r.Dir, cmd)
		require.ErrorContains(t, err, "invalid workspace")
	}
	cmd.Path = filepath.Join(r.Dir, "absent")
	_, err = verifiedWorkspace(t.Context(), r.Dir, cmd)
	require.ErrorContains(t, err, "wrapper")
	cmd.Command = terraform
	write(t, filepath.Join(r.Dir, "main.tf"), "terraform {")
	_, err = verifiedWorkspace(t.Context(), r.Dir, cmd)
	require.Error(t, err)
}
