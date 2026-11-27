package stack

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/component"
	_ "github.com/cloudposse/atmos/pkg/component/helm" // Register the built-in chart provider as the CLI does.
	_ "github.com/cloudposse/atmos/pkg/component/mock"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	stackadoption "github.com/cloudposse/atmos/pkg/stack/adoption"
)

func localRegistration(t *testing.T, kind string) stackadoption.Registration {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	dir := filepath.Join(root, "sources", "marker")
	if kind == "packer" {
		writeAdoptionFile(t, filepath.Join(dir, "image.pkr.hcl"), "build { sources = [\"source.file.marker\"] }")
	} else {
		writeAdoptionFile(t, filepath.Join(dir, "Chart.yaml"), "apiVersion: v2\nname: marker\nversion: 1.0.0\n")
	}
	return stackadoption.Registration{Adapters: stackAdapters(), Project: root, Dir: dir, Stack: "dev", Output: io.Discard}
}

func localConfig(r *stackadoption.Registration) *schema.AtmosConfiguration {
	c := &schema.AtmosConfiguration{}
	c.StacksBaseAbsolutePath = filepath.Join(r.Project, "stacks")
	c.Stacks.IncludedPaths = []string{"*.yaml"}
	c.HelmDirAbsolutePath = filepath.Join(r.Project, "sources")
	c.PackerDirAbsolutePath = c.HelmDirAbsolutePath
	return c
}

func TestLocalAdoption(t *testing.T) {
	for _, kind := range []string{"helm", "packer"} {
		t.Run(kind, func(t *testing.T) {
			r := localRegistration(t, kind)
			if kind == "packer" {
				r.VarFiles = []string{"config/dev vars.pkrvars.hcl", "extra.pkrvars.json"}
				writeAdoptionFile(t, filepath.Join(r.Dir, r.VarFiles[0]), "ratio = 1.75\n")
				writeAdoptionFile(t, filepath.Join(r.Dir, r.VarFiles[1]), "{\"label\":\"private-value\"}")
			}
			var log bytes.Buffer
			r.Output = &log
			r.DryRun = true
			r.Name = "blue"
			require.NoError(t, r.Save(t.Context()))
			require.Contains(t, log.String(), "Save: atmos stack add")
			require.Contains(t, log.String(), "-s dev --name blue")
			require.NotContains(t, log.String(), "private-value")
			require.NoFileExists(t, filepath.Join(r.Project, "atmos.yaml"))
			r.DryRun = false
			require.NoError(t, r.Save(t.Context()))
			file := filepath.Join(r.Project, "stacks", "dev.yaml")
			before, err := os.ReadFile(file)
			require.NoError(t, err)
			if kind == "packer" {
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
			if kind == "packer" {
				r.VarFiles = []string{"extra.pkrvars.json"}
				require.ErrorContains(t, r.Save(t.Context()), "different variable sources")
			}
		})
	}
}

func TestSourceSelectionAndFlags(t *testing.T) {
	r := localRegistration(t, "packer")
	source, _, err := stackAdapters().Detect(filepath.Join(r.Dir, "image.pkr.hcl"))
	require.NoError(t, err)
	require.Equal(t, "packer", source.Kind())
	writeAdoptionFile(t, filepath.Join(r.Dir, "main.tf"), "")
	_, _, err = stackAdapters().Detect(r.Dir)
	require.ErrorContains(t, err, "multiple tool")
	writeAdoptionFile(t, filepath.Join(r.Dir, "Chart.yaml"), "")
	source, _, err = stackAdapters().Detect(filepath.Join(r.Dir, "Chart.yaml"))
	require.NoError(t, err)
	require.Equal(t, "helm", source.Kind())
	writeAdoptionFile(t, filepath.Join(r.Dir, "terragrunt.hcl"), "")
	source, _, err = stackAdapters().Detect(r.Dir)
	require.NoError(t, err)
	require.Equal(t, "terragrunt", source.Kind())
	for _, tc := range []struct {
		kind string
		r    stackadoption.Registration
	}{
		{"terraform", stackadoption.Registration{Name: "bad/name"}},
		{"packer", stackadoption.Registration{Namespace: "oops"}},
		{"helm", stackadoption.Registration{VarFiles: []string{"x"}}},
		{"packer", stackadoption.Registration{Explicit: "terraform"}},
		{"terragrunt", stackadoption.Registration{VarFiles: []string{"foo"}}},
	} {
		adapters := stackAdapters()
		index := slices.IndexFunc(adapters, func(a stackadoption.Adapter) bool { return a.Kind() == tc.kind })
		require.NotEqual(t, -1, index)
		require.Error(t, tc.r.CheckOptions(adapters[index]))
	}
	_, _, err = stackAdapters().Detect(filepath.Join(r.Dir, "missing"))
	require.Error(t, err)
	writeAdoptionFile(t, filepath.Join(r.Dir, "legacy.json"), "{}")
	_, _, err = stackAdapters().Detect(filepath.Join(r.Dir, "legacy.json"))
	require.Error(t, err)
}

func TestInputFileErrors(t *testing.T) {
	r := localRegistration(t, "packer")
	r.DryRun = true
	for name, content := range map[string]string{"bad.pkrvars.hcl": "x = var.env", "bad.pkrvars.json": "[]", "input.txt": "none", "a#bad.pkrvars.hcl": "x=1", "a{{bad.pkrvars.hcl": "x=1"} {
		writeAdoptionFile(t, filepath.Join(r.Dir, name), content)
		r.VarFiles = []string{name}
		require.Error(t, r.Save(t.Context()), name)
	}
	outside := filepath.Join(t.TempDir(), "vars.pkrvars.json")
	writeAdoptionFile(t, outside, "{}")
	for _, ref := range []string{outside, "missing.pkrvars.hcl"} {
		r.VarFiles = []string{ref}
		require.Error(t, r.Save(t.Context()))
	}
	file := filepath.Join(r.Project, "stacks", "dev.yaml")
	r.VarFiles = nil
	for _, existing := range []string{"import: [mixins/defaults]", "vars: {stage: inherited}", "components: {packer: {marker: {metadata: {component: elsewhere}}}}", "components: [", "name: other"} {
		writeAdoptionFile(t, file, existing)
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
	writeAdoptionFile(t, filepath.Join(r.Project, "atmos.yaml"), "base_path: .\n")
	require.ErrorContains(t, r.Save(t.Context()), "missing")
	r.Config.PackerDirAbsolutePath = filepath.Join(r.Project, "components", "packer")
	require.NoError(t, r.Save(t.Context()))
	writeAdoptionFile(t, filepath.Join(r.Project, "atmos.yaml"), "components: {packer: {base_path: custom}}")
	require.ErrorContains(t, r.Save(t.Context()), "missing")
	r.Config = nil
	require.ErrorContains(t, r.Save(t.Context()), "reload and retry")
}

func writeAdoptionFile(t *testing.T, path, text string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
}

// A provider known only to this test must work through the real stack add command.
type adoptionTestProvider struct{ component.ComponentProvider }

func (p adoptionTestProvider) AdoptionAdapters() stackadoption.Adapters {
	return stackadoption.Adapters{manifestAdapter(p)}
}

type manifestAdapter struct{ component.ComponentProvider }

func (manifestAdapter) Kind() string      { return "mock" }
func (manifestAdapter) Options() []string { return []string{"name"} }
func (manifestAdapter) Detect(source stackadoption.Source) (stackadoption.Match, error) {
	if source.IsDir && slices.Contains(source.Files, "mock.config") {
		return stackadoption.Match{Path: source.Path}, nil
	}
	return stackadoption.Match{}, nil
}

func (adapter manifestAdapter) Adopt(_ context.Context, r *stackadoption.Registration, path string) error {
	return r.WriteLocalComponent(&stackadoption.LocalComponent{
		Kind: adapter.Kind(), ConfiguredBase: adapter.GetBasePath(r.Config), Name: r.Name, Dir: path, NextCommand: "validate",
		Entry: func(dir, base string) (map[string]any, error) {
			rel, err := stackadoption.RelativeDirectory(base, dir)
			return map[string]any{"metadata": map[string]any{"component": rel}, "vars": map[string]any{"count": 2}}, err
		},
	})
}

func TestStackAddProviderAdoption(t *testing.T) {
	original, ok := component.GetProvider("mock")
	require.True(t, ok)
	require.NoError(t, component.Register(adoptionTestProvider{original}))
	t.Cleanup(func() { require.NoError(t, component.Register(original)) })
	project := t.TempDir()
	t.Chdir(project)
	t.Setenv("ATMOS_GIT_ROOT_BASEPATH", "false")
	t.Setenv("ATMOS_CLI_CONFIG_PATH", project)
	writeAdoptionFile(t, filepath.Join(project, "services", "hello", "mock.config"), "")
	_, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)
	run := func(args ...string) string {
		t.Helper()
		cmd := newStackAddCommand()
		cmd.SetArgs(append([]string{"./services/hello", "--name", "canary", "-s", "preview"}, args...))
		var out bytes.Buffer
		cmd.SetOut(&out)
		require.NoError(t, cmd.ExecuteContext(t.Context()))
		return out.String()
	}
	manifest := filepath.Join(project, "stacks", "preview.yaml")
	require.Contains(t, run("--dry-run"), "canary")
	require.NoFileExists(t, manifest)
	require.Contains(t, run(), "atmos mock validate canary -s preview")
	data, err := os.ReadFile(manifest)
	require.NoError(t, err)
	require.Contains(t, string(data), "count: 2")
	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	require.NoError(t, err)
	old := atmosConfigPtr
	SetAtmosConfig(&config)
	t.Cleanup(func() { SetAtmosConfig(old) })
	run()
	after, err := os.ReadFile(manifest)
	require.NoError(t, err)
	require.Equal(t, data, after)
}
