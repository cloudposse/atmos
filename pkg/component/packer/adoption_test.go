package packer

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	stackadoption "github.com/cloudposse/atmos/pkg/stack/adoption"
)

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
			entry, err := packerEntry(file, dir, dir, make(map[string]bool))
			if tc.want != "" {
				require.ErrorContains(t, err, tc.want)
				return
			}
			require.NoError(t, err)
			require.Equal(t, map[string]any{"packer": map[string]any{"template": tc.name}}, entry["settings"])
		})
	}
}

func write(t *testing.T, path, text string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
}

func TestPackerAdoptionInputSummary(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	template := filepath.Join(dir, "image.pkr.hcl")
	write(t, template, `variable "env" {}
variable "fromfile" {}
variable "fromauto" {}
variable "missing" {}
variable "defaulted" { default = "native-secret" }
build { sources = ["source.file.example"] }`)
	auto := filepath.Join(dir, "overlay.auto.pkrvars.json")
	write(t, auto, `{"fromauto":"auto-secret"}`)
	explicit := filepath.Join(dir, "explicit.pkrvars.hcl")
	write(t, explicit, "fromfile = \"file-secret\"\nundeclared = \"other-secret\"")
	t.Setenv("PKR_VAR_env", "env-secret")
	out := new(bytes.Buffer)
	r := stackadoption.Registration{Dir: dir, Project: dir, Output: out, VarFiles: []string{explicit}}
	variables := map[string]bool{}
	_, err = packerEntry(dir, dir, dir, variables)
	require.NoError(t, err)
	require.NoError(t, reportPackerInputs(&r, dir, variables))
	require.Contains(t, out.String(), "PKR_VAR_missing")
	require.Contains(t, out.String(), "undeclared")
	for _, s := range []string{"secret", "PKR_VAR_fromauto", "PKR_VAR_fromfile", "PKR_VAR_env", "PKR_VAR_defaulted"} {
		require.NotContains(t, out.String(), s)
	}
	for _, name := range []string{"missing.pkrvars.hcl", "corrupt.pkrvars.hcl", "outside.pkrvars.hcl", "block.pkrvars.hcl"} {
		t.Run(name, func(t *testing.T) {
			switch name {
			case "corrupt.pkrvars.hcl":
				write(t, filepath.Join(dir, name), "bad = {")
			case "outside.pkrvars.hcl":
				target := filepath.Join(t.TempDir(), name)
				write(t, target, "x=1")
				require.NoError(t, os.Symlink(target, filepath.Join(dir, name)))
			case "block.pkrvars.hcl":
				write(t, filepath.Join(dir, name), "block {}")
			}
			r.VarFiles = []string{name}
			require.Error(t, reportPackerInputs(&r, dir, variables))
		})
	}
	r.VarFiles = nil
	require.Error(t, reportPackerInputs(&r, filepath.Join(dir, "missing"), variables))
}
