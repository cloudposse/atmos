package tests

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Compare plans against populated local state. A successful plan alone also
// passes if registration accidentally selected a new, empty workspace.
func TestCLITerraformAdoption(t *testing.T) {
	ensureAtmosRunner(t)
	for _, engine := range []struct {
		name, file string
		binary     func(*testing.T) string
	}{{"terraform", "main.tf", RequireTerraformPath}, {"tofu", "main.tofu", RequireTofuPath}} {
		t.Run(engine.name, func(t *testing.T) {
			binary := engine.binary(t)
			project := t.TempDir()
			dir := filepath.Join(project, "infra", "network")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			t.Setenv("ATMOS_CLI_CONFIG_PATH", project)
			t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "false")
			t.Setenv("ATMOS_BASE_PATH", "")
			t.Setenv("ATMOS_COMPONENTS_TERRAFORM_COMMAND", "")
			t.Setenv("TF_WORKSPACE", "")
			t.Setenv("TF_DATA_DIR", "")
			const module = `variable "ratio" { type = number }
variable "options" { type = map(string) }
resource "terraform_data" "marker" { input = "${terraform.workspace}|${var.ratio}|${join(",", keys(var.options))}" }
`
			require.NoError(t, os.WriteFile(filepath.Join(dir, "common.tfvars"), []byte("ratio=1.75\noptions={common=\"old\"}"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(dir, engine.file), []byte(module), 0o600))
			native := func(args ...string) []byte {
				t.Helper()
				cmd := exec.CommandContext(t.Context(), binary, args...)
				cmd.Dir = dir
				output, err := cmd.Output()
				require.NoError(t, err)
				return output
			}
			atmos := func(args ...string) {
				t.Helper()
				cmd := atmosRunner.CommandContext(t.Context(), args...)
				cmd.Dir = project
				output, err := cmd.CombinedOutput()
				require.NoError(t, err, "%s", output)
			}
			native("init", "-input=false")
			var priorIDs []string
			for _, target := range []struct{ stack, workspace string }{{"dev", "default"}, {"prod", "blue"}} {
				if target.workspace != "default" {
					native("workspace", "new", target.workspace)
				}
				override := target.stack + ".tfvars.json"
				require.NoError(t, os.WriteFile(filepath.Join(dir, override), []byte(`{"options":{"`+target.stack+`":"new"}}`), 0o600))
				native("apply", "-auto-approve", "-input=false", "-var-file=common.tfvars", "-var-file="+override)
				native("plan", "-out=native.plan", "-var-file=common.tfvars", "-var-file="+override)
				want := adoptedPriorResource(t, native("show", "-json", "native.plan"))
				require.Equal(t, target.workspace+"|1.75|"+target.stack, want.Input)
				priorIDs = append(priorIDs, want.ID)
				if target.stack == "dev" {
					atmos("terraform", "plan", "./infra/network", "--command="+engine.name, "-out=trial.plan", "-var-file=common.tfvars", "-var-file="+override)
					require.Equal(t, want, adoptedPriorResource(t, native("show", "-json", "trial.plan")))
				}
				native("workspace", "select", "default")
				atmos("stack", "add", "./infra/network", "-s", target.stack, "--command="+engine.name, "--workspace", target.workspace, "--var-file", "common.tfvars", "--var-file", override)
				atmos("terraform", "plan", "network", "-s", target.stack, "-out=named.plan")
				require.Equal(t, want, adoptedPriorResource(t, native("show", "-json", "named.plan")))
				require.Equal(t, "default", strings.TrimSpace(string(native("workspace", "show"))))
			}
			require.NotEqual(t, priorIDs[0], priorIDs[1])
		})
	}
}

type priorResource struct{ ID, Input string }

func adoptedPriorResource(t *testing.T, plan []byte) priorResource {
	t.Helper()
	var document struct {
		Changes []struct {
			Address string
			Change  struct {
				Before  priorResource
				Actions []string
			}
		} `json:"resource_changes"`
	}
	require.NoError(t, json.Unmarshal(plan, &document))
	require.Len(t, document.Changes, 1)
	resource := document.Changes[0]
	require.Equal(t, "terraform_data.marker", resource.Address)
	require.Equal(t, []string{"no-op"}, resource.Change.Actions)
	require.NotEmpty(t, resource.Change.Before.ID)
	return resource.Change.Before
}
