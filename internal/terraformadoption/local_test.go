package terraformadoption

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func localRegistration(t *testing.T, kind string) Registration {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dir := filepath.Join(root, "sources", "marker")
	if kind == packerKind {
		write(t, filepath.Join(dir, "image.pkr.hcl"), "build { sources = [\"source.file.marker\"] }")
	} else {
		write(t, filepath.Join(dir, chartFile), "apiVersion: v2\nname: marker\nversion: 1.0.0\n")
	}
	return Registration{Project: root, Dir: dir, Stack: "dev", Output: io.Discard}
}

func localConfig(r *Registration) *schema.AtmosConfiguration {
	c := &schema.AtmosConfiguration{}
	c.StacksBaseAbsolutePath = filepath.Join(r.Project, "stacks")
	c.Stacks.IncludedPaths = []string{"*.yaml"}
	c.HelmDirAbsolutePath = filepath.Join(r.Project, "sources")
	c.PackerDirAbsolutePath = c.HelmDirAbsolutePath
	return c
}

func TestLocalAdoption(t *testing.T) {
	for _, kind := range []string{helmKind, packerKind} {
		t.Run(kind, func(t *testing.T) {
			r := localRegistration(t, kind)
			if kind == packerKind {
				r.VarFiles = []string{"config/dev vars.pkrvars.hcl", "extra.pkrvars.json"}
				write(t, filepath.Join(r.Dir, r.VarFiles[0]), "ratio = 1.75\n")
				write(t, filepath.Join(r.Dir, r.VarFiles[1]), "{\"label\":\"private-value\"}")
			}
			var log bytes.Buffer
			r.Output = &log
			r.DryRun = true
			r.Name = "blue"
			require.NoError(t, r.Save(t.Context()))
			require.Contains(t, log.String(), "blue -s dev")
			require.NotContains(t, log.String(), "private-value")
			require.NoFileExists(t, filepath.Join(r.Project, "atmos.yaml"))
			r.DryRun = false
			require.NoError(t, r.Save(t.Context()))
			file := filepath.Join(r.Project, "stacks", "dev.yaml")
			before, err := os.ReadFile(file)
			require.NoError(t, err)
			if kind == packerKind {
				require.Contains(t, string(before), "!include")
				require.Contains(t, string(before), "extra.pkrvars.json")
			}
			r.Config = localConfig(&r)
			require.NoError(t, r.Save(t.Context()))
			got, err := os.ReadFile(file)
			require.NoError(t, err)
			require.Equal(t, before, got)
			r.Name = "green"
			require.NoError(t, r.Save(t.Context()))
			r.Stack = "prod"
			require.NoError(t, r.Save(t.Context()))
			require.FileExists(t, filepath.Join(r.Project, "stacks", "prod.yaml"))
			if kind == packerKind {
				r.VarFiles = []string{"extra.pkrvars.json"}
				require.ErrorContains(t, r.Save(t.Context()), "different variable sources")
			}
		})
	}
}

func TestSourceSelectionAndFlags(t *testing.T) {
	r := localRegistration(t, packerKind)
	source, err := detectSource(filepath.Join(r.Dir, "image.pkr.hcl"))
	require.NoError(t, err)
	require.Equal(t, packerKind, source.kind)
	write(t, filepath.Join(r.Dir, "main.tf"), "")
	_, err = detectSource(r.Dir)
	require.ErrorContains(t, err, "multiple tool")
	write(t, filepath.Join(r.Dir, chartFile), "")
	source, err = detectSource(filepath.Join(r.Dir, chartFile))
	require.NoError(t, err)
	require.Equal(t, helmKind, source.kind)
	write(t, filepath.Join(r.Dir, unitFile), "")
	source, err = detectSource(r.Dir)
	require.NoError(t, err)
	require.Equal(t, tg, source.kind)
	for _, tc := range []struct {
		kind string
		r    Registration
	}{
		{terraform, Registration{Name: "bad/name"}},
		{packerKind, Registration{Namespace: "oops"}},
		{helmKind, Registration{VarFiles: []string{"x"}}},
		{packerKind, Registration{Explicit: "terraform"}},
		{tg, Registration{VarFiles: []string{"foo"}}},
	} {
		require.Error(t, tc.r.checkSourceFlags(tc.kind))
	}
	_, err = detectSource(filepath.Join(r.Dir, "missing"))
	require.Error(t, err)
	write(t, filepath.Join(r.Dir, "legacy.json"), "{}")
	_, err = detectSource(filepath.Join(r.Dir, "legacy.json"))
	require.Error(t, err)
}

func TestPackerTemplateShapes(t *testing.T) {
	for _, tc := range []struct{ name, code, want string }{
		{"split.pkr.hcl", "variable \"x\" {}", "0 build sources"},
		{"broken.pkr.hcl", "build {", "invalid component"},
		{"dynamic.pkr.hcl", "build { sources = var.sources }", "literal list"},
		{"many.pkr.hcl", "build { sources = [\"source.file.a\", \"source.file.b\"] }", "2 build sources"},
		{"one.pkr.hcl", "build {\n source \"source.file.a\" {}\n}", ""},
		{"json.pkr.json", `{"build":[{"sources":["source.file.a"]}]}`, ""},
		{"json.pkr.hcl", "build \"x\" {}", "invalid component"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, err := filepath.EvalSymlinks(t.TempDir())
			require.NoError(t, err)
			file := filepath.Join(dir, tc.name)
			write(t, file, tc.code)
			entry, err := packerEntry(file, dir, dir)
			if tc.want != "" {
				require.ErrorContains(t, err, tc.want)
				return
			}
			require.NoError(t, err)
			require.Equal(t, map[string]any{"packer": map[string]any{"template": tc.name}}, entry["settings"])
		})
	}
}

func TestInputFileErrors(t *testing.T) {
	r := localRegistration(t, packerKind)
	r.DryRun = true
	for name, content := range map[string]string{"bad.pkrvars.hcl": "x = var.env", "bad.pkrvars.json": "[]", "input.txt": "none", "a#bad.pkrvars.hcl": "x=1", "a{{bad.pkrvars.hcl": "x=1"} {
		write(t, filepath.Join(r.Dir, name), content)
		r.VarFiles = []string{name}
		require.Error(t, r.Save(t.Context()), name)
	}
	outside := filepath.Join(t.TempDir(), "vars.pkrvars.json")
	write(t, outside, "{}")
	for _, ref := range []string{outside, "missing.pkrvars.hcl"} {
		r.VarFiles = []string{ref}
		require.Error(t, r.Save(t.Context()))
	}
	file := filepath.Join(r.Project, "stacks", "dev.yaml")
	r.VarFiles = nil
	for _, existing := range []string{"import: [mixins/defaults]", "vars: {stage: inherited}", "components: {packer: {marker: {metadata: {component: elsewhere}}}}", "components: [", "name: other"} {
		write(t, file, existing)
		require.Error(t, r.Save(t.Context()), existing)
	}
	require.NoError(t, os.Remove(file))
	r.Config = localConfig(&r)
	r.Config.Stacks.NamePattern = "{stage}"
	require.ErrorContains(t, r.Save(t.Context()), "filename-named")
	r.Config.Stacks.NamePattern = ""
	r.Config.PackerDirAbsolutePath = t.TempDir()
	require.ErrorContains(t, r.Save(t.Context()), "outside component base")
	r.Config.PackerDirAbsolutePath = filepath.Join(r.Project, "custom")
	require.ErrorContains(t, r.Save(t.Context()), "missing")
	write(t, filepath.Join(r.Project, "atmos.yaml"), "base_path: .\n")
	require.ErrorContains(t, r.Save(t.Context()), "missing")
	r.Config.PackerDirAbsolutePath = filepath.Join(r.Project, "components", "packer")
	require.NoError(t, r.Save(t.Context()))
	write(t, filepath.Join(r.Project, "atmos.yaml"), "components: {packer: {base_path: custom}}")
	require.ErrorContains(t, r.Save(t.Context()), "missing")
	r.Config = nil
	require.ErrorContains(t, r.Save(t.Context()), "reload and retry")
}

func TestTerraformVariableFileRegistration(t *testing.T) {
	r := registration(t)
	r.Explicit = tofu
	r.Name = "blue"
	write(t, filepath.Join(r.Dir, "common.tfvars"), "message=\"secret\"\n")
	write(t, filepath.Join(r.Dir, "dev.tfvars.json"), "{\"ratio\":1.75}")
	r.VarFiles = []string{"common.tfvars", "dev.tfvars.json"}
	require.NoError(t, r.Save(t.Context()))
	file := filepath.Join(r.Project, "stacks", "dev.yaml")
	b, err := os.ReadFile(file)
	require.NoError(t, err)
	require.Contains(t, string(b), "blue:")
	require.NotContains(t, string(b), "secret")
	require.Less(t, strings.Index(string(b), "common.tfvars"), strings.Index(string(b), "dev.tfvars.json"))
	r.Config = localConfig(&r)
	r.Config.TerraformDirAbsolutePath = r.Project
	require.NoError(t, r.Save(t.Context()))
	r.VarFiles = []string{"dev.tfvars.json", "common.tfvars"}
	require.ErrorContains(t, r.Save(t.Context()), "different variable sources")
	b, err = os.ReadFile(file)
	require.NoError(t, err)
	write(t, file, strings.ReplaceAll(string(b), "!include ", ""))
	r.VarFiles = []string{"common.tfvars", "dev.tfvars.json"}
	require.ErrorContains(t, r.Save(t.Context()), "different variable sources")
}
