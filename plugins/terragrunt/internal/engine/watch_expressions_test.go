package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// watchProject lays out live/root.hcl, a unit that includes it, and the files the
// unit's expressions can name. Paths are canonical so symlink expansion is a no-op.
type watchProject struct {
	t    *testing.T
	fs   afero.Fs
	root string
	unit string
}

func newWatchProject(t *testing.T) *watchProject {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	project := &watchProject{t: t, fs: afero.NewMemMapFs(), root: root, unit: filepath.Join(root, "live", "prod", "app")}
	project.write(filepath.Join(root, "live", "root.hcl"), `locals { region = "east" }`)
	project.write(filepath.Join(root, ".git"), "gitdir: elsewhere")
	return project
}

func (p *watchProject) write(path, contents string) {
	p.t.Helper()
	require.NoError(p.t, p.fs.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(p.t, afero.WriteFile(p.fs, path, []byte(contents), 0o600))
}

// track writes the unit configuration and returns the watches inventory computes.
func (p *watchProject) track(configuration string) *model.Resolution {
	p.t.Helper()
	path := filepath.Join(p.unit, "terragrunt.hcl")
	p.write(path, `include "root" { path = find_in_parent_folders("root.hcl") }
`+configuration)
	result := &model.Resolution{Path: path, Files: []string{path, filepath.Join(p.root, "live", "root.hcl")}}
	require.NoError(p.t, trackInputSources(p.fs, p.root, result))
	return result
}

func TestWatchesResolveCommonPathIdioms(t *testing.T) {
	project := newWatchProject(t)
	project.write(filepath.Join(project.root, "live", "common.json"), "{}")
	live := filepath.Join(project.root, "live")
	tests := []struct {
		name       string
		inputs     string
		file       string
		wantFolder bool
	}{
		{name: "terragrunt dir", inputs: `file("${get_terragrunt_dir()}/x.json")`, file: filepath.Join(project.unit, "x.json")},
		{name: "parent dir", inputs: `templatefile("${get_parent_terragrunt_dir()}/t.tpl", {})`, file: filepath.Join(live, "t.tpl")},
		{name: "relative to include", inputs: `file("${get_parent_terragrunt_dir()}/${path_relative_to_include()}/x")`, file: filepath.Join(project.unit, "x")},
		{name: "relative from include", inputs: `file("${get_terragrunt_dir()}/${path_relative_from_include()}/x")`, file: filepath.Join(project.unit, "..", "..", "x")},
		{name: "abspath", inputs: `file(abspath("rel.json"))`, file: filepath.Join(project.unit, "rel.json")},
		{name: "format with literal", inputs: `file(format("%s/%s.json", get_terragrunt_dir(), "svc"))`, file: filepath.Join(project.unit, "svc.json")},
		{name: "dirname and join", inputs: `file(join("/", [dirname(get_terragrunt_dir()), "sibling.json"]))`, file: filepath.Join(project.root, "live", "prod", "sibling.json")},
		{name: "find in parent folders", inputs: `file(find_in_parent_folders("common.json"))`, file: filepath.Join(live, "common.json")},
		{name: "repo root", inputs: `file("${get_repo_root()}/shared/x.yaml")`, file: filepath.Join(project.root, "shared", "x.yaml")},
		{name: "relative literal", inputs: `file("input.json")`, file: filepath.Join(project.unit, "input.json")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := project.track("inputs = { v = " + test.inputs + " }\n")
			require.Contains(t, result.Files, filepath.Clean(test.file))
			require.Empty(t, result.Folders, "a resolvable path must not watch the project")
		})
	}
}

func TestWatchesResolveLocals(t *testing.T) {
	project := newWatchProject(t)
	result := project.track(`
locals {
  name = "svc"
  path = "${get_terragrunt_dir()}/${local.name}.json"
  unknown = get_env("MISSING")
}
inputs = { v = file(local.path) }
`)
	require.Contains(t, result.Files, filepath.Join(project.unit, "svc.json"))
	require.Empty(t, result.Folders)

	// A local that needs the environment makes only the expressions using it conservative.
	result = project.track(`
locals { unknown = get_env("MISSING") }
inputs = { v = file(local.unknown) }
`)
	require.Equal(t, []string{project.root}, result.Folders)
}

func TestWatchesFallBackForUnresolvableOnly(t *testing.T) {
	project := newWatchProject(t)
	// One resolvable and one unresolvable read: the unresolvable one widens the watch.
	result := project.track(`inputs = { a = file("${get_terragrunt_dir()}/a.json"), b = file("${get_env("DIR")}/b.json") }`)
	require.Equal(t, []string{project.root}, result.Folders)
	require.NotContains(t, result.Files, filepath.Join(project.unit, "a.json"), "covered by the project watch")

	// Reads outside the project cannot appear in a change set; they must not fail inventory.
	result = project.track(`inputs = { a = file("/etc/hostname") }`)
	require.Empty(t, result.Folders)
	require.NotContains(t, result.Files, "/etc/hostname")
}

func TestWatchesCoverGenerateExtraArgumentsAndHooks(t *testing.T) {
	project := newWatchProject(t)
	project.write(filepath.Join(project.root, "scripts", "check.sh"), "echo")
	result := project.track(`
terraform {
  source = "../../../modules/m"
  extra_arguments "vars" {
    commands           = ["plan"]
    required_var_files = ["${get_terragrunt_dir()}/required.tfvars", "${get_parent_terragrunt_dir()}/common.tfvars"]
    optional_var_files = ["${get_terragrunt_dir()}/optional.tfvars"]
  }
  before_hook "script" {
    commands = ["plan"]
    execute  = ["../../../scripts/hook.sh"]
  }
  after_hook "arg" {
    commands    = ["plan"]
    execute     = ["bash", "${get_repo_root()}/scripts/check.sh", "plain-argument", "${get_repo_root()}/missing.sh"]
    working_dir = "${get_repo_root()}/tools"
  }
  error_hook "path" {
    commands = ["plan"]
    execute  = ["tofu", "version"]
  }
}
generate "provider" {
  path     = "provider.tf"
  if_exists = "overwrite"
  contents = file("${get_terragrunt_dir()}/provider.tpl")
}
remote_state {
  backend  = "local"
  generate = { path = "backend.tf", if_exists = "overwrite" }
  config   = { path = file("${get_terragrunt_dir()}/state-path.txt") }
}
`)
	for _, want := range []string{
		filepath.Join(project.unit, "required.tfvars"),
		filepath.Join(project.root, "live", "common.tfvars"),
		filepath.Join(project.unit, "optional.tfvars"),
		filepath.Join(project.root, "scripts", "hook.sh"),
		filepath.Join(project.root, "scripts", "check.sh"),
		filepath.Join(project.unit, "provider.tpl"),
		filepath.Join(project.unit, "state-path.txt"),
		filepath.Join(project.unit, lockFileName),
	} {
		require.Contains(t, result.Files, want)
	}
	// Plain words and missing arguments are not project files.
	require.NotContains(t, result.Files, filepath.Join(project.root, "missing.sh"))
	require.NotContains(t, result.Files, filepath.Join(project.unit, "plain-argument"))
	require.Equal(t, []string{filepath.Join(project.root, "tools")}, result.Folders)
}

func TestWatchesVarFilesFallBackWhenDynamic(t *testing.T) {
	project := newWatchProject(t)
	result := project.track(`
terraform {
  extra_arguments "vars" {
    commands           = ["plan"]
    optional_var_files = ["${get_terragrunt_dir()}/${get_env("ENV", "dev")}.tfvars"]
  }
}
`)
	require.Equal(t, []string{project.root}, result.Folders)
}

func TestWatchesFollowReadConfigurationFiles(t *testing.T) {
	project := newWatchProject(t)
	project.write(filepath.Join(project.unit, "extra.hcl"), `
locals { body = file("${get_terragrunt_dir()}/nested.txt") }
inputs = { v = file("${get_terragrunt_dir()}/inputs.txt") }
`)
	result := project.track(`
generate "x" {
  path     = "x.tf"
  if_exists = "overwrite"
  contents = read_terragrunt_config("${get_terragrunt_dir()}/extra.hcl").locals.body
}
`)
	require.Contains(t, result.Files, filepath.Join(project.unit, "extra.hcl"))
	require.Contains(t, result.Files, filepath.Join(project.unit, "nested.txt"))
	// Only the read configuration's locals are inspected; its inputs are not evaluated for the reader.
	require.Contains(t, result.Files, filepath.Join(project.unit, "inputs.txt"))
	require.Empty(t, result.Folders)
}

func TestWatchesCoverWholeCopiedSourceRoot(t *testing.T) {
	project := newWatchProject(t)
	for _, test := range []struct {
		source string
		want   []string
	}{
		{source: "../../../modules/sub//app", want: []string{filepath.Join(project.root, "modules", "sub")}},
		{source: filepath.Join(project.root, "modules", "sub") + "//app", want: []string{filepath.Join(project.root, "modules", "sub")}},
		{source: "git::https://example.invalid/repo.git//sub?ref=v1"},
		{source: "https://example.invalid/archive.zip//sub"},
		{source: "../../../modules/plain"},
	} {
		t.Run(test.source, func(t *testing.T) {
			path := filepath.Join(project.unit, "terragrunt.hcl")
			project.write(path, "")
			result := &model.Resolution{Path: path, Files: []string{path}, Source: test.source}
			require.NoError(t, trackInputSources(project.fs, project.root, result))
			require.Equal(t, test.want, result.Folders)
		})
	}
}

func TestWatchesDropFilesCoveredByFolders(t *testing.T) {
	// A submodule module is empty in revisions that predate it. Its files add nothing to the
	// folder watch and must not make the dependency list differ between revisions.
	project := newWatchProject(t)
	module := filepath.Join(project.root, "modules", "sm")
	path := filepath.Join(project.unit, "terragrunt.hcl")
	project.write(path, "")
	files := []string{path, filepath.Join(module, "main.tf")}
	result := &model.Resolution{Path: path, Files: slicesClone(files), Folders: []string{module}}
	require.NoError(t, trackInputSources(project.fs, project.root, result))
	require.NotContains(t, result.Files, filepath.Join(module, "main.tf"))
	require.Contains(t, result.Files, path)
	require.Equal(t, []string{module}, result.Folders)

	empty := &model.Resolution{Path: path, Files: []string{path}, Folders: []string{module}}
	require.NoError(t, trackInputSources(project.fs, project.root, empty))
	require.Equal(t, result.Files, empty.Files)
	require.Equal(t, result.Folders, empty.Folders)
}

func slicesClone(values []string) []string { return append([]string(nil), values...) }

func TestWatchesIncludeSymlinkTargets(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	target := filepath.Join(root, "modules", "other")
	require.NoError(t, os.MkdirAll(target, 0o700))
	link := filepath.Join(root, "modules", "linked")
	if err := os.Symlink("other", link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	unit := filepath.Join(root, "live", "link")
	require.NoError(t, os.MkdirAll(unit, 0o700))
	path := filepath.Join(unit, "terragrunt.hcl")
	require.NoError(t, os.WriteFile(path, []byte(`inputs = { v = file("${get_terragrunt_dir()}/../../modules/linked/data.txt") }`), 0o600))

	result := &model.Resolution{Path: path, Files: []string{path}, Folders: []string{link}}
	require.NoError(t, trackInputSources(afero.NewOsFs(), root, result))
	// Git reports edits under the target's path, so both the link and the target are watched.
	require.Equal(t, []string{link, target}, result.Folders)

	// A file reached through a linked directory is watched under its target path too.
	file := &model.Resolution{Path: path, Files: []string{path}}
	require.NoError(t, trackInputSources(afero.NewOsFs(), root, file))
	require.Contains(t, file.Files, filepath.Join(target, "data.txt"))
	require.Contains(t, file.Files, filepath.Join(link, "data.txt"))
}

func TestWatchesIgnoreSymlinkTargetsOutsideProject(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	outside, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	link := filepath.Join(root, "modules", "external")
	require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o700))
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	path := filepath.Join(root, "terragrunt.hcl")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	result := &model.Resolution{Path: path, Files: []string{path}, Folders: []string{link}}
	require.NoError(t, trackInputSources(afero.NewOsFs(), root, result))
	require.Equal(t, []string{link}, result.Folders)
}
