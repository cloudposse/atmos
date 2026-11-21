package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

func TestInputWatchesDoNotEvaluateInputs(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "live", "app", "terragrunt.hcl")
	fs := afero.NewMemMapFs()
	require.NoError(t, fs.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, afero.WriteFile(fs, path, []byte(`
include "root" { path = find_in_parent_folders("root.hcl") }
inputs = {
  optional = fileexists("optional.json") ? jsondecode(file("optional.json")) : {}
  members = fileset("config", "*.yaml")
  template = templatefile("config.tpl", {})
  secret = sops_decrypt_file("secret.enc.yaml")
  command = run_cmd("must-not-run")
  unrelated = upper("text")
}
`), 0o600))
	result := &model.Resolution{Files: []string{path, filepath.Join(root, "data.json")}}
	require.NoError(t, trackInputSources(fs, root, result))
	for _, name := range []string{"optional.json", "config.tpl", "secret.enc.yaml"} {
		require.Contains(t, result.Files, filepath.Join(filepath.Dir(path), name))
	}
	require.Equal(t, []string{filepath.Join(filepath.Dir(path), "config")}, result.Folders)
	// Missing optional files and empty glob directories are watched before creation.
	exists, err := afero.Exists(fs, filepath.Join(filepath.Dir(path), "optional.json"))
	require.NoError(t, err)
	require.False(t, exists)
}

func TestInputWatchesConservativelyTrackDynamicPaths(t *testing.T) {
	// Expressions that need the environment, outputs, or missing locals cannot be resolved.
	for _, expression := range []string{`file(local.path)`, `file("${get_env("DIR")}/config.json")`, `file(dependency.net.outputs.path)`, `fileset(local.root, "*.yaml")`, `find_in_parent_folders("optional.hcl")`, `file(null)`, `file(123)`, `file()`} {
		t.Run(expression, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "terragrunt.hcl")
			fs := afero.NewMemMapFs()
			require.NoError(t, fs.MkdirAll(root, 0o700))
			require.NoError(t, afero.WriteFile(fs, path, []byte("inputs = { value = "+expression+" }"), 0o600))
			result := &model.Resolution{Files: []string{path}}
			require.NoError(t, trackInputSources(fs, root, result))
			require.Equal(t, []string{root}, result.Folders)
		})
	}
}

func TestInputWatchesFailuresAndIncludedInputs(t *testing.T) {
	root := t.TempDir()
	fs := afero.NewMemMapFs()
	path := filepath.Join(root, "shared.hcl")
	result := &model.Resolution{Files: []string{path}}
	require.NoError(t, trackInputSources(fs, root, result))
	require.Contains(t, result.Files, path, "missing mark_as_read remains watched")
	require.NoError(t, os.MkdirAll(path, 0o700))
	require.Error(t, trackInputSources(afero.NewOsFs(), root, result), "non-absence errors still fail")
	require.NoError(t, os.Remove(path))
	require.NoError(t, fs.MkdirAll(root, 0o700))
	// A read data file named *.hcl need not be HCL; it remains a file watch instead of
	// failing the whole inventory.
	require.NoError(t, afero.WriteFile(fs, path, []byte("inputs = {"), 0o600))
	require.NoError(t, trackInputSources(fs, root, result))
	// The lock file beside the unit is always watched as well.
	require.Equal(t, []string{filepath.Join(root, lockFileName), path}, result.Files)
	require.Empty(t, result.Folders)
	require.NoError(t, afero.WriteFile(fs, path, []byte(`locals { unrelated = "constant" }`), 0o600))
	require.NoError(t, trackInputSources(fs, root, result))
	require.Empty(t, result.Folders)
	require.NoError(t, afero.WriteFile(fs, path, []byte(`inputs = { value = read_terragrunt_config("shared-input.hcl") }`), 0o600))
	require.NoError(t, trackInputSources(fs, root, result))
	require.Contains(t, result.Folders, filepath.Join(root, "shared-input.hcl"), "creation as either a file or directory is watched")
}

func TestInputWatchesFollowNestedConfigurationReads(t *testing.T) {
	root := t.TempDir()
	fs := afero.NewMemMapFs()
	require.NoError(t, fs.MkdirAll(filepath.Join(root, "shared"), 0o700))
	unit := filepath.Join(root, "terragrunt.hcl")
	write := func(path, contents string) {
		t.Helper()
		require.NoError(t, afero.WriteFile(fs, path, []byte(contents), 0o600))
	}
	write(unit, `inputs = { common = read_terragrunt_config("shared/common.hcl") }`)
	// The nested configuration reads data and, cyclically, the unit; both are followed once.
	write(filepath.Join(root, "shared", "common.hcl"), `inputs = {
  data   = file("data.json")
  again  = read_terragrunt_config("../terragrunt.hcl")
  deeper = read_terragrunt_config("absent.hcl")
}`)
	result := &model.Resolution{Files: []string{unit}}
	require.NoError(t, trackInputSources(fs, root, result))
	for _, path := range []string{
		filepath.Join(root, "shared", "common.hcl"),
		filepath.Join(root, "shared", "data.json"),
	} {
		require.Contains(t, result.Files, path, "a transitive read is watched, even before it exists")
	}
	require.Equal(t, []string{filepath.Join(root, "shared", "absent.hcl")}, result.Folders, "missing config creation is watched as either a file or directory")
}

// A JSON configuration read by read_terragrunt_config cannot be inspected for nested reads, so the
// read widens the watch to the project instead of silently missing them.
func TestInputWatchesWidenForJSONConfigurationReads(t *testing.T) {
	root := t.TempDir()
	fs := afero.NewMemMapFs()
	unit := filepath.Join(root, "terragrunt.hcl")
	require.NoError(t, fs.MkdirAll(root, 0o700))
	require.NoError(t, afero.WriteFile(fs, unit, []byte(`inputs = { common = read_terragrunt_config("shared.hcl.json") }`), 0o600))
	result := &model.Resolution{Files: []string{unit}}
	require.NoError(t, trackInputSources(fs, root, result))
	require.Equal(t, []string{root, filepath.Join(root, "shared.hcl.json")}, result.Folders)
	require.NotContains(t, result.Files, filepath.Join(root, "data.json"))

	require.NoError(t, afero.WriteFile(fs, unit, []byte(`inputs = { common = read_terragrunt_config("shared.hcl") }`), 0o600))
	result = &model.Resolution{Files: []string{unit}}
	require.NoError(t, trackInputSources(fs, root, result))
	require.Equal(t, []string{filepath.Join(root, "shared.hcl")}, result.Folders, "missing HCL config creation is watched without widening to the project")
}

// A path outside the project that is a symlink into it is watched at its canonical target.
func TestInputWatchesKeepSymlinkTargetsInsideTheProject(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	outside, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	target := filepath.Join(root, "shared", "data.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o700))
	require.NoError(t, os.WriteFile(target, []byte("{}"), 0o600))
	link := filepath.Join(outside, "data.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	unit := filepath.Join(root, "terragrunt.hcl")
	outsideOnly := filepath.Join(outside, "elsewhere.json")
	require.NoError(t, os.WriteFile(unit, []byte(`inputs = {
  linked = file("`+filepath.ToSlash(link)+`")
  other  = file("`+filepath.ToSlash(outsideOnly)+`")
}`), 0o600))
	result := &model.Resolution{Files: []string{unit}}
	require.NoError(t, trackInputSources(afero.NewOsFs(), root, result))
	require.Contains(t, result.Files, target)
	require.NotContains(t, result.Files, link)
	require.NotContains(t, result.Files, outsideOnly)
}

// A directory passed to read_terragrunt_config selects its default configuration, so the directory
// is watched rather than recorded as a file, while a file path stays a file watch.
func TestInputWatchesResolveDirectoryConfigurationReads(t *testing.T) {
	root := t.TempDir()
	fs := afero.NewMemMapFs()
	require.NoError(t, fs.MkdirAll(filepath.Join(root, "shared"), 0o700))
	unit := filepath.Join(root, "terragrunt.hcl")
	require.NoError(t, afero.WriteFile(fs, unit, []byte(`inputs = { common = read_terragrunt_config("shared") }`), 0o600))
	require.NoError(t, afero.WriteFile(fs, filepath.Join(root, "shared", "terragrunt.hcl"), []byte(`inputs = { nested = read_terragrunt_config("../other.hcl") }`), 0o600))
	require.NoError(t, afero.WriteFile(fs, filepath.Join(root, "other.hcl"), []byte(`inputs = {}`), 0o600))
	result := &model.Resolution{Files: []string{unit}}
	require.NoError(t, trackInputSources(fs, root, result))
	require.Equal(t, []string{filepath.Join(root, "shared")}, result.Folders)
	require.NotContains(t, result.Files, filepath.Join(root, "shared"), "a directory is not a file watch")
	require.Contains(t, result.Files, filepath.Join(root, "other.hcl"), "the default configuration in the directory is followed")

	// A path that is not a directory stays a file watch without widening to a folder.
	require.NoError(t, afero.WriteFile(fs, unit, []byte(`inputs = { common = read_terragrunt_config("shared/terragrunt.hcl") }`), 0o600))
	result = &model.Resolution{Files: []string{unit}}
	require.NoError(t, trackInputSources(fs, root, result))
	require.Empty(t, result.Folders)
	require.Contains(t, result.Files, filepath.Join(root, "shared", "terragrunt.hcl"))
}

// Includes in a configuration read through inputs are followed in the including file's scope, and an
// include that cannot be resolved widens the watch instead of hiding included reads.
func TestInputWatchesFollowIncludesOfConfigurationReads(t *testing.T) {
	root := t.TempDir()
	fs := afero.NewMemMapFs()
	require.NoError(t, fs.MkdirAll(filepath.Join(root, "shared", "base"), 0o700))
	unit := filepath.Join(root, "terragrunt.hcl")
	write := func(path, contents string) {
		t.Helper()
		require.NoError(t, afero.WriteFile(fs, path, []byte(contents), 0o600))
	}
	write(unit, `inputs = { common = read_terragrunt_config("shared/common.hcl") }`)
	write(filepath.Join(root, "shared", "common.hcl"), `include "base" { path = "base/base.hcl" }`)
	write(filepath.Join(root, "shared", "base", "base.hcl"), `inputs = {
  local  = file("local.json")
  scoped = file("${get_terragrunt_dir()}/scoped.json")
}`)
	result := &model.Resolution{Files: []string{unit}}
	require.NoError(t, trackInputSources(fs, root, result))
	for _, path := range []string{
		filepath.Join(root, "shared", "base", "base.hcl"),
		filepath.Join(root, "shared", "base", "local.json"),
		filepath.Join(root, "shared", "scoped.json"),
	} {
		require.Contains(t, result.Files, path)
	}
	require.NotContains(t, result.Files, filepath.Join(root, "shared", "base", "scoped.json"), "the included file evaluates in the including scope")
	require.Empty(t, result.Folders, "a resolvable include does not widen the watch")

	for name, include := range map[string]string{
		"unresolved path": `include "base" { path = get_env("BASE") }`,
		"missing path":    `include "base" {}`,
		"json include":    `include "base" { path = "base.hcl.json" }`,
	} {
		t.Run(name, func(t *testing.T) {
			write(filepath.Join(root, "shared", "common.hcl"), include)
			result := &model.Resolution{Files: []string{unit}}
			require.NoError(t, trackInputSources(fs, root, result))
			require.Equal(t, []string{root}, result.Folders)
		})
	}
}

func TestInputWatchesDirectoryJSONDefault(t *testing.T) {
	for _, withHCL := range []bool{false, true} {
		t.Run(fmt.Sprint(withHCL), func(t *testing.T) {
			root := t.TempDir()
			fs := afero.NewMemMapFs()
			shared := filepath.Join(root, "shared")
			require.NoError(t, fs.MkdirAll(shared, 0o700))
			unit := filepath.Join(root, "terragrunt.hcl")
			require.NoError(t, afero.WriteFile(fs, unit, []byte(`inputs = { shared = read_terragrunt_config("shared") }`), 0o600))
			require.NoError(t, afero.WriteFile(fs, filepath.Join(shared, "terragrunt.hcl.json"), []byte(`{"inputs":{"data":"${file(\"../data.json\")}"}}`), 0o600))
			if withHCL {
				require.NoError(t, afero.WriteFile(fs, filepath.Join(shared, "terragrunt.hcl"), []byte(`inputs = {}`), 0o600))
			}
			result := &model.Resolution{Files: []string{unit}}
			require.NoError(t, trackInputSources(fs, root, result))
			require.Contains(t, result.Folders, root)
		})
	}
}

func TestInputWatchesRepeatedIncludeScopes(t *testing.T) {
	root := t.TempDir()
	fs := afero.NewMemMapFs()
	write := func(path, contents string) {
		t.Helper()
		path = filepath.Join(root, path)
		require.NoError(t, fs.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, afero.WriteFile(fs, path, []byte(contents), 0o600))
	}
	write("terragrunt.hcl", `inputs = { a = read_terragrunt_config("a/env.hcl"), b = read_terragrunt_config("b/env.hcl") }`)
	for _, dir := range []string{"a", "b"} {
		write(dir+"/env.hcl", `include "base" { path = "../base.hcl" }`)
	}
	write("base.hcl", `
include "cycle" { path = "../base.hcl" }
inputs = { data = file("${get_terragrunt_dir()}/data.json") }
`)
	result := &model.Resolution{Files: []string{filepath.Join(root, "terragrunt.hcl")}}
	require.NoError(t, trackInputSources(fs, root, result))
	for _, dir := range []string{"a", "b"} {
		require.Contains(t, result.Files, filepath.Join(root, dir, "data.json"))
	}
	require.Empty(t, result.Folders)
}

func TestInputWatchesOriginalCallerInNestedReads(t *testing.T) {
	root := t.TempDir()
	fs := afero.NewMemMapFs()
	unit := filepath.Join(root, "app", "terragrunt.hcl")
	require.NoError(t, fs.MkdirAll(filepath.Dir(unit), 0o700))
	require.NoError(t, fs.MkdirAll(filepath.Join(root, "shared"), 0o700))
	require.NoError(t, afero.WriteFile(fs, unit, []byte(`inputs = { common = read_terragrunt_config("../shared/common.hcl") }`), 0o600))
	require.NoError(t, afero.WriteFile(fs, filepath.Join(root, "shared", "common.hcl"), []byte(`inputs = { original = file("${get_original_terragrunt_dir()}/data.json"), current = file("${get_terragrunt_dir()}/current.json") }`), 0o600))
	result := &model.Resolution{Files: []string{unit}}
	require.NoError(t, trackInputSources(fs, root, result))
	require.Contains(t, result.Files, filepath.Join(root, "app", "data.json"))
	require.Contains(t, result.Files, filepath.Join(root, "shared", "current.json"))
	require.NotContains(t, result.Files, filepath.Join(root, "shared", "data.json"))
	require.Empty(t, result.Folders)
}

func TestWatchesFollowNativeConfigWithoutHCLSuffix(t *testing.T) {
	project := newWatchProject(t)
	settings := filepath.Join(project.unit, "shared", "settings.conf")
	project.write(settings, `inputs = { value = file("../data.json") }`)
	result := project.track(`inputs = { nested = read_terragrunt_config("shared/settings.conf") }`)
	require.Contains(t, result.Files, settings)
	require.Contains(t, result.Files, filepath.Join(project.unit, "data.json"))
	require.Empty(t, result.Folders)
}

func TestIncludedConfigReadUsesUnitWhileFileUsesAuthorDirectory(t *testing.T) {
	project := newWatchProject(t)
	parent := filepath.Join(project.root, "live", "root.hcl")
	project.write(parent, `inputs = {
 nested = read_terragrunt_config("extra.conf")
 authored = file("authored.json")
}`)
	project.write(filepath.Join(project.unit, "extra.conf"), `inputs = { data = file("nested.json") }`)
	result := project.track(`inputs = {}`)
	require.Contains(t, result.Files, filepath.Join(project.unit, "extra.conf"))
	require.Contains(t, result.Files, filepath.Join(project.unit, "nested.json"))
	require.Contains(t, result.Files, filepath.Join(filepath.Dir(parent), "authored.json"))
	require.NotContains(t, result.Files, filepath.Join(filepath.Dir(parent), "extra.conf"))
	require.Empty(t, result.Folders)
}

func TestDiscoveryKeepsMissingMarkAsRead(t *testing.T) {
	request := nativeUnit(t, `locals { optional = mark_as_read("optional.hcl") }`)
	request.Phase = "discover"
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	root, err := filepath.EvalSymlinks(filepath.Dir(request.Path))
	require.NoError(t, err)
	require.Contains(t, result.Files, filepath.Join(root, "optional.hcl"))
}

func TestInputWatchesResolveDirectoryConfiguration(t *testing.T) {
	root := t.TempDir()
	fs := afero.NewMemMapFs()
	require.NoError(t, fs.MkdirAll(filepath.Join(root, "shared"), 0o700))
	unit := filepath.Join(root, "terragrunt.hcl")
	shared := filepath.Join(root, "shared", "terragrunt.hcl")
	require.NoError(t, afero.WriteFile(fs, unit, []byte(`inputs = { value = read_terragrunt_config("shared") }`), 0o600))
	result := &model.Resolution{Files: []string{unit}}
	require.NoError(t, trackInputSources(fs, root, result))
	require.NotContains(t, result.Files, shared, "directory watch covers its default config")
	require.Contains(t, result.Folders, filepath.Dir(shared), "config creation is watched")
	require.NoError(t, afero.WriteFile(fs, shared, []byte(`inputs = { value = file("../data.json") }`), 0o600))
	require.NoError(t, trackInputSources(fs, root, result))
	require.Contains(t, result.Files, filepath.Join(root, "data.json"), "selected config is scanned transitively")
	result = &model.Resolution{Files: []string{unit}}
	require.ErrorIs(t, trackInputSources(statFailureFs{Fs: fs, err: os.ErrPermission}, root, result), os.ErrPermission, "optional config stat failures are not hidden")
}
