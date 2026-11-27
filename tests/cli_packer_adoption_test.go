package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// The built-in file builder creates a local artifact whose exact bytes can be
// compared across independent native and Atmos runs. No cloud/plugin is needed.
func TestCLIPackerAdoption(t *testing.T) {
	ensureAtmosRunner(t)
	packer := requireExecutablePath(t, "packer", "validating and building an adopted Packer template")
	project := t.TempDir()
	dir := filepath.Join(project, "images", "marker")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	t.Setenv("PATH", filepath.Dir(packer)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ATMOS_CLI_CONFIG_PATH", project)
	t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "false")
	t.Setenv("ATMOS_BASE_PATH", "")
	t.Setenv("PKR_VAR_message", "environment")
	input := func(name, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600))
	}
	input("variables.pkr.hcl", `variable "message" { type = string }
variable "ratio" { type = number }
variable "options" { type = map(string) }
`)
	input("image.pkr.hcl", `source "file" "marker" {
 content = "${var.message}|${var.ratio}|${join(",", keys(var.options))}"
 target  = "${path.root}/marker.txt"
}
build { sources = ["source.file.marker"] }
`)
	input("auto.auto.pkrvars.hcl", "message = \"auto\"\n")
	input("common.pkrvars.hcl", "message=\"base\"\nratio=1.75\noptions={obsolete=\"old\"}\n")
	run := func(cmd *exec.Cmd, cwd string) {
		t.Helper()
		cmd.Dir = cwd
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s\n%s", cmd.Args, out)
	}
	atmos := func(args ...string) { t.Helper(); run(atmosRunner.CommandContext(t.Context(), args...), project) }
	native := func(args ...string) { t.Helper(); run(exec.CommandContext(t.Context(), packer, args...), dir) }
	read := func(path string) string {
		t.Helper()
		b, err := os.ReadFile(path)
		require.NoError(t, err)
		return string(b)
	}
	native("init", ".")
	for _, stack := range []string{"dev", "prod"} {
		overlay := stack + ".pkrvars.json"
		input(overlay, `{"message":"`+stack+`","options":{"active":"yes"}}`)
		native("build", "-force", "-var-file=common.pkrvars.hcl", "-var-file="+overlay, ".")
		artifact := filepath.Join(dir, "marker.txt")
		want := read(artifact)
		require.Equal(t, stack+"|1.75|active", want)
		require.NoError(t, os.Remove(artifact))
		add := []string{"stack", "add", "./images/marker", "-s", stack, "--var-file", "common.pkrvars.hcl", "--var-file", overlay}
		atmos(append(add, "--dry-run")...)
		manifest := filepath.Join(project, "stacks", stack+".yaml")
		require.NoFileExists(t, manifest)
		atmos(add...)
		before := read(manifest)
		atmos(add...)
		require.Equal(t, before, read(manifest))
		atmos("packer", "init", "marker", "-s", stack)
		atmos("packer", "validate", "marker", "-s", stack)
		atmos("packer", "build", "marker", "-s", stack)
		require.Equal(t, want, read(artifact))
		require.NoError(t, os.Remove(artifact))
		input(overlay, `{"message":"changed","options":{}}`)
		atmos("packer", "build", "marker", "-s", stack)
		require.Equal(t, "changed|1.75|", read(artifact))
	}
}
