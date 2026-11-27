package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCLIAtlantisAdoption(t *testing.T) {
	ensureAtmosRunner(t)
	tf := RequireTerraformPath(t)
	r := t.TempDir()
	dir := filepath.Join(r, "infra", "network")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "env"), 0o755))
	t.Setenv("PATH", filepath.Dir(tf)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "false")
	t.Setenv("ATMOS_CLI_CONFIG_PATH", r)
	t.Setenv("ATMOS_COMPONENTS_TERRAFORM_COMMAND", "")
	t.Setenv("ATMOS_BASE_PATH", "")
	t.Setenv("TF_WORKSPACE", "")
	t.Setenv("TF_DATA_DIR", "")
	write := func(path, data string) { t.Helper(); require.NoError(t, os.WriteFile(path, []byte(data), 0o600)) }
	write(filepath.Join(r, "atlantis.yaml"), `version: 3
projects:
  - name: network-dev
    dir: infra/network
    workflow: native
    apply_requirements: [approved]
  - name: shell
    dir: infra/network
    workflow: custom
workflows:
  native:
    plan:
      steps:
        - init
        - plan:
            extra_args: [-var-file=base.tfvars, -var-file, override.tfvars]
  custom:
    plan:
      steps:
        - run: echo ran > should-not-run
`)
	write(filepath.Join(dir, "main.tf"), `variable "prefix" { type = string }
variable "suffix" { type = string }
resource "terraform_data" "marker" { input = "${var.prefix}|${var.suffix}|${terraform.workspace}" }
`)
	write(filepath.Join(dir, "base.tfvars"), "prefix=\"base\"\nsuffix=\"base\"")
	write(filepath.Join(dir, "override.tfvars"), "prefix=\"override\"\nsuffix=\"override\"")
	write(filepath.Join(dir, "env", "default.tfvars"), "suffix=\"workspace file\"")
	native := func(args ...string) []byte {
		t.Helper()
		cmd := exec.CommandContext(t.Context(), tf, args...)
		cmd.Dir = dir
		out, err := cmd.Output()
		require.NoError(t, err)
		return out
	}
	atmos := func(args ...string) []byte {
		t.Helper()
		cmd := atmosRunner.CommandContext(t.Context(), args...)
		cmd.Dir = r
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", out)
		return out
	}
	native("init", "-input=false")
	inputs := []string{"-var-file=base.tfvars", "-var-file", "override.tfvars", "-var-file=env/default.tfvars"}
	native(append([]string{"apply", "-auto-approve", "-input=false"}, inputs...)...)
	native(append([]string{"plan", "-out=native.plan"}, inputs...)...)
	want := adoptedPriorResource(t, native("show", "-json", "native.plan"))
	require.Equal(t, "override|workspace file|default", want.Input)
	inventory := atmos("stack", "add", "./atlantis.yaml", "--dry-run")
	require.Contains(t, string(inventory), "plan step 1 (run)")
	require.Contains(t, string(inventory), "approved")
	unsupported := atmosRunner.CommandContext(t.Context(), "stack", "add", "./atlantis.yaml", "--project", "shell")
	unsupported.Dir = r
	out, err := unsupported.CombinedOutput()
	require.Error(t, err)
	require.Contains(t, string(out), "requires manual translation")
	require.NoFileExists(t, filepath.Join(dir, "should-not-run"))
	require.NoFileExists(t, filepath.Join(r, "should-not-run"))
	require.NoFileExists(t, filepath.Join(r, "atmos.yaml"))
	add := []string{"stack", "add", "./atlantis.yaml", "--project", "network-dev", "-s", "dev"}
	atmos(add...)
	manifest := filepath.Join(r, "stacks", "dev.yaml")
	before, err := os.ReadFile(manifest)
	require.NoError(t, err)
	require.Contains(t, string(before), "atlantis.yaml#network-dev")
	atmos(add...)
	after, err := os.ReadFile(manifest)
	require.NoError(t, err)
	require.Equal(t, before, after)
	atmos("terraform", "plan", "network-dev", "-s", "dev", "-out=named.plan")
	require.Equal(t, want, adoptedPriorResource(t, native("show", "-json", "named.plan")))
}

// Exercise the downloadable newcomer fixture, including repeating a physical path
// whose registered name differs from the directory name.
func TestCLIAtlantisImportExample(t *testing.T) {
	ensureAtmosRunner(t)
	tf := RequireTerraformPath(t)
	root := t.TempDir()
	require.NoError(t, os.CopyFS(root, os.DirFS("../examples/atlantis-import")))
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "false")
	t.Setenv("ATMOS_BASE_PATH", "")
	t.Setenv("ATMOS_COMPONENTS_TERRAFORM_COMMAND", "")
	t.Setenv("TF_WORKSPACE", "")
	t.Setenv("TF_DATA_DIR", "")
	t.Setenv("PATH", filepath.Dir(tf)+string(os.PathListSeparator)+os.Getenv("PATH"))

	run := func(wantSuccess bool, args ...string) string {
		t.Helper()
		cmd := atmosRunner.CommandContext(t.Context(), args...)
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		if wantSuccess {
			require.NoError(t, err, "%s", output)
		} else {
			require.Error(t, err, "%s", output)
		}
		return string(output)
	}
	require.Contains(t, run(false, "terraform", "plan"), "atmos terraform plan ./infra/network")
	native := exec.CommandContext(t.Context(), tf, "-chdir=infra/network", "init", "-input=false")
	native.Dir = root
	out, err := native.CombinedOutput()
	require.NoError(t, err, "%s", out)
	inventory := run(true, "stack", "add", "./atlantis.yaml", "--dry-run")
	require.Contains(t, inventory, "network-dev")
	require.Contains(t, inventory, "network-review")
	require.Contains(t, inventory, "plan step 1 (run)")
	require.NoFileExists(t, filepath.Join(root, "atmos.yaml"))
	run(true, "stack", "add", "./atlantis.yaml", "--project", "network-dev", "-s", "dev")
	require.Contains(t, run(true, "terraform", "plan", "network-dev", "-s", "dev"), "Hello from dev")
	require.Contains(t, run(true, "terraform", "plan", "./infra/network"), "Hello from dev")
	// After a user translates the review workflow, its existing native workspace
	// gives the same Terraform root a second independent deployment.
	config, err := os.ReadFile(filepath.Join(root, "atlantis.yaml"))
	require.NoError(t, err)
	translated := strings.Replace(string(config), "workflow: review", "workflow: network", 1)
	require.NoError(t, os.WriteFile(filepath.Join(root, "atlantis.yaml"), []byte(translated), 0o600))
	native = exec.CommandContext(t.Context(), tf, "-chdir=infra/network", "workspace", "new", "review")
	native.Dir = root
	out, err = native.CombinedOutput()
	require.NoError(t, err, "%s", out)
	run(true, "stack", "add", "./atlantis.yaml", "--project", "network-review", "-s", "alternate")
	require.Contains(t, run(true, "terraform", "plan", "network-review", "-s", "alternate"), "Hello from common")
	ambiguous := run(false, "terraform", "plan", "./infra/network", "--interactive=false")
	require.Contains(t, ambiguous, "atmos terraform plan network-dev -s dev")
	require.Contains(t, ambiguous, "atmos terraform plan network-review -s alternate")
	t.Setenv("ATMOS_STACK", "alternate")
	require.Contains(t, run(true, "terraform", "plan", "./infra/network"), "Hello from common")
}
