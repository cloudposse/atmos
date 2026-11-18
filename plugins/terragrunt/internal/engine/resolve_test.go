package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/atmosbridge"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

func nativeUnit(t *testing.T, extra string) model.Request {
	t.Helper()
	root := t.TempDir()
	module := filepath.Join(root, "module")
	require.NoError(t, os.MkdirAll(module, 0o755))
	source, err := json.Marshal(filepath.ToSlash(module))
	require.NoError(t, err)
	path := filepath.Join(root, "terragrunt.hcl")
	require.NoError(t, os.WriteFile(path, []byte("terraform { source = "+string(source)+" }\n"+extra), 0o644))
	return model.Request{Path: path, BasePath: root, Command: "plan", Tofu: "must-not-execute", Environment: map[string]string{}}
}

func TestNativeLocalBackendAndLiteralGeneration(t *testing.T) {
	request := nativeUnit(t, `remote_state {
 backend = "local"
 config = { path = "relative/state.tfstate" }
}
generate "literal" {
 path = "literal.tf"
 if_exists = "overwrite"
 disable_signature = true
 contents = "# {{ keep }} $${literal}\n"
}
inputs = { large = 9007199254740993, nested = { nullable = null, enabled = false } }
`)
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(result.WorkingDir, "relative", "state.tfstate"), result.Backend["path"])
	require.Equal(t, "# {{ keep }} ${literal}\n", result.Generated["literal.tf"])
	require.Equal(t, json.Number("9007199254740993"), result.Inputs["large"])
	require.NoFileExists(t, filepath.Join(filepath.Dir(request.Path), "literal.tf"))
}

func TestNativeRejectsUnsupportedSemantics(t *testing.T) {
	for _, test := range []struct{ name, extra, message string }{
		{"remote backend", `remote_state {
 backend = "s3"
 config = { bucket = "unsupported" }
}`, "only adapts the local backend"},
		{"prevent destroy", `prevent_destroy = true`, "prevent_destroy"},
		{"binary override", `terraform_binary = "another-tool"`, "terraform_binary"},
		{"IAM role", `iam_role = "unsupported"`, "IAM settings"},
		{"generation policy", `generate "x" {
 path = "x.tf"
 if_exists = "overwrite_terragrunt"
 contents = ""
}`, "generation requires"},
		{"generation escape", `generate "x" {
 path = "../x.tf"
 if_exists = "overwrite"
 disable_signature = true
 contents = ""
}`, "unsupported generated path"},
		{"version constraint", `terragrunt_version_constraint = "> 99.0.0"`, "rejects version"},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := Resolve(t.Context(), nativeUnit(t, test.extra))
			require.ErrorContains(t, err, test.message)
		})
	}
}

func TestBackendIdentityDoesNotExecuteVersionProbe(t *testing.T) {
	request := nativeUnit(t, `terraform_version_constraint = "> 99.0.0"
remote_state {
 backend = "local"
 config = { path = "state.tfstate" }
}
`)
	request.Phase = "backend"
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, "local", result.BackendType)
	request.Phase = ""
	_, err = Resolve(t.Context(), request)
	require.ErrorContains(t, err, "cannot inspect selected executable")
}

func TestBackendGenerateDoesNotWriteStack(t *testing.T) {
	root := t.TempDir()
	fixture := filepath.Join("..", "..", "fixtures", "explicit-stack")
	require.NoError(t, os.CopyFS(root, os.DirFS(fixture)))
	request := model.Request{Path: filepath.Join(root, "live", "terragrunt.stack.hcl"), BasePath: root, Phase: "backend", Command: "generate", Tofu: "must-not-execute", Environment: map[string]string{}}
	_, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.NoDirExists(t, filepath.Join(root, "live", ".terragrunt-stack"))
}

func TestDiscoveryDeniesCommands(t *testing.T) {
	request := nativeUnit(t, `locals { value = run_cmd("must-not-execute") }
inputs = { value = local.value }
`)
	request.Phase = "discover"
	_, err := Resolve(t.Context(), request)
	require.ErrorContains(t, err, "read-only discovery cannot execute")
}

func TestVersionConstraintsUseSelectedExecutable(t *testing.T) {
	executable, err := os.Executable()
	require.NoError(t, err)
	for _, test := range []struct {
		name, actual, constraint string
		want                     string
	}{
		{"accepted", `{"terraform_version":"1.9.0"}`, ">= 1.8.0", ""},
		{"rejected", `{"terraform_version":"1.7.0"}`, ">= 1.8.0", "rejects version"},
		{"invalid version", `{"terraform_version":"invalid"}`, ">= 1.8.0", "malformed version"},
		{"invalid output", `not-json`, ">= 1.8.0", "invalid character"},
		{"invalid constraint", `{"terraform_version":"1.9.0"}`, "this is not a constraint", "malformed constraint"},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := model.Request{Tofu: executable, Environment: map[string]string{"ATMOS_TERRAGRUNT_VERSION_CHILD": test.actual}}
			err := checkVersions(t.Context(), &request, test.constraint, "")
			if test.want == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, test.want)
			}
			if test.want == "rejects version" {
				require.ErrorIs(t, err, ErrVersionRejected)
			}
		})
	}
}

func TestResolveGuardsAndDownloadDirectory(t *testing.T) {
	request := nativeUnit(t, `download_dir = "custom-cache"
inputs = { value = "constant", working = get_working_dir() }
generate "disabled" {
 path = "not-valid"
 disable = true
 if_exists = "overwrite"
 contents = "ignored"
}
`)
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Contains(t, result.WorkingDir, "custom-cache")
	require.Empty(t, result.Generated)
	request.Environment["TG_DOWNLOAD_DIR"] = "environment-cache"
	result, err = Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Contains(t, result.WorkingDir, "environment-cache")
	require.Equal(t, result.WorkingDir, result.Inputs["working"])
	request.LogLevel = "not-a-level"
	_, err = Resolve(t.Context(), request)
	require.Error(t, err)
	request.LogLevel = ""
	request.Path = filepath.Join(request.BasePath, "missing.hcl")
	_, err = Resolve(t.Context(), request)
	require.Error(t, err)
	require.NoError(t, os.WriteFile(request.Path, []byte("inputs = {}"), 0o644))
	_, err = Resolve(t.Context(), request)
	require.ErrorContains(t, err, "requires terraform.source")
	for _, source := range []string{"git::https://example.invalid/repo", "./absent"} {
		encoded, err := json.Marshal(source)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(request.Path, []byte("terraform { source = "+string(encoded)+" }"), 0o644))
		_, err = Resolve(t.Context(), request)
		require.Error(t, err)
	}
}

func TestResolvePreservesUnitOverlay(t *testing.T) {
	request := nativeUnit(t, "")
	directory := filepath.Dir(request.Path)
	require.NoError(t, os.WriteFile(filepath.Join(directory, "override.tf"), []byte("# overlay"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(directory, ".hidden"), []byte("ignored"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(directory, ".tflint.hcl"), []byte("# lint"), 0o644))
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, []byte("# overlay"), result.Overlay["override.tf"])
	require.Equal(t, []byte("# lint"), result.Overlay[".tflint.hcl"])
	require.NotContains(t, result.Overlay, ".hidden")
	data, err := os.ReadFile(request.Path)
	require.NoError(t, err)
	configuration := strings.Replace(string(data), "terraform { source =", "terraform {\n include_in_copy = [\".hidden\"]\n exclude_from_copy = [\"override.tf\"]\n source =", 1)
	configuration = strings.Replace(configuration, " }", "\n}", 1)
	require.NoError(t, os.WriteFile(request.Path, []byte(configuration), 0o644))
	result, err = Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Equal(t, []byte("ignored"), result.Overlay[".hidden"])
	require.NotContains(t, result.Overlay, "override.tf")
}

func TestRejectRemoteStateEncryption(t *testing.T) {
	request := nativeUnit(t, `remote_state {
 backend = "local"
 config = { path = "state.tfstate" }
 encryption = { key_provider = "pbkdf2" }
}`)
	_, err := Resolve(t.Context(), request)
	require.ErrorContains(t, err, "remote_state.encryption")
}

func TestBackendResolutionHonorsConfiguredDownloadDirectory(t *testing.T) {
	request := nativeUnit(t, `download_dir = "custom-cache"
remote_state {
 backend = "local"
 config = { path = "state.tfstate" }
}
`)
	full, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	request.Phase = "backend"
	backend, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Contains(t, backend.WorkingDir, "custom-cache")
	require.Equal(t, full.WorkingDir, backend.WorkingDir)
	require.Equal(t, full.Backend["path"], backend.Backend["path"])
}

func TestBackendResolutionNeverEvaluatesUnrelatedInputs(t *testing.T) {
	configuration := `download_dir = "custom-cache"
remote_state {
 backend = "local"
 config = { path = "state.tfstate" }
}
inputs = { value = run_cmd("atmos-missing-command-that-must-not-run") }
`
	request := nativeUnit(t, configuration)
	_, err := Resolve(t.Context(), request)
	require.Error(t, err, "a full read evaluates the command-valued input")
	request.Phase = "backend"
	backend, err := Resolve(t.Context(), request)
	require.NoError(t, err, "a backend read never evaluates unrelated inputs")
	require.Equal(t, "local", backend.BackendType)
	require.Contains(t, backend.WorkingDir, "custom-cache")
	t.Run("included inputs", func(t *testing.T) {
		request := nativeUnit(t, "include \"root\" {\n path = \"root.hcl\"\n}\n")
		dir := filepath.Dir(request.Path)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "root.hcl"), []byte(configuration), 0o644))
		request.Phase = "backend"
		backend, err := Resolve(t.Context(), request)
		require.NoError(t, err)
		require.Contains(t, backend.WorkingDir, "custom-cache")
	})
}

func TestBackendDownloadDirectoryNeverReadsDependencyOutputs(t *testing.T) {
	request := nativeUnit(t, `download_dir = "custom-cache"
dependency "other" {
 config_path = "other"
}
inputs = { value = dependency.other.outputs.value }
remote_state {
 backend = "local"
 config = { path = "state.tfstate" }
}
`)
	other := filepath.Join(filepath.Dir(request.Path), "other")
	require.NoError(t, os.MkdirAll(other, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(other, "terragrunt.hcl"), []byte("inputs = {}\n"), 0o644))
	request.Phase = "backend"
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Contains(t, result.WorkingDir, "custom-cache")
}

func TestOverlayRejectsLockFilesAndCarriesModes(t *testing.T) {
	t.Run("committed dependency lock file", func(t *testing.T) {
		request := nativeUnit(t, "")
		require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(request.Path), ".terraform.lock.hcl"), []byte("# lock"), 0o644))
		_, err := Resolve(t.Context(), request)
		require.ErrorIs(t, err, atmosbridge.ErrUnsupportedOverlay)
		require.ErrorContains(t, err, ".terraform.lock.hcl")
		request.Phase = "backend"
		_, err = Resolve(t.Context(), request)
		require.NoError(t, err, "backend reads never export the overlay")
		request.Phase = ""
		data, err := os.ReadFile(request.Path)
		require.NoError(t, err)
		excluded := strings.Replace(string(data), "terraform { source =", "terraform {\n exclude_from_copy = [\".terraform.lock.hcl\"]\n source =", 1)
		excluded = strings.Replace(excluded, " }", "\n}", 1)
		require.NoError(t, os.WriteFile(request.Path, []byte(excluded), 0o644))
		result, err := Resolve(t.Context(), request)
		require.NoError(t, err)
		require.NotContains(t, result.Overlay, ".terraform.lock.hcl")
	})
	t.Run("executable file keeps its mode", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows has no executable permission bit.")
		}
		request := nativeUnit(t, "")
		dir := filepath.Dir(request.Path)
		script := filepath.Join(dir, "probe.sh")
		require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755))
		require.NoError(t, os.Chmod(script, 0o755))
		plain := filepath.Join(dir, "plain.tf")
		require.NoError(t, os.WriteFile(plain, []byte("# plain"), 0o644))
		result, err := Resolve(t.Context(), request)
		require.NoError(t, err)
		require.Contains(t, result.Overlay, "probe.sh")
		require.Equal(t, uint32(0o755), result.OverlayModes["probe.sh"])
		require.NotContains(t, result.OverlayModes, "plain.tf", "default modes are not exported")
	})
	t.Run("private file keeps its restrictive mode", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows ignores POSIX permission bits.")
		}
		request := nativeUnit(t, "")
		dir := filepath.Dir(request.Path)
		for name, mode := range map[string]os.FileMode{"private_key.pem": 0o600, "group.pem": 0o640, "plain.tf": 0o644} {
			path := filepath.Join(dir, name)
			require.NoError(t, os.WriteFile(path, []byte("# "+name), mode))
			require.NoError(t, os.Chmod(path, mode))
		}
		result, err := Resolve(t.Context(), request)
		require.NoError(t, err)
		require.Equal(t, uint32(0o600), result.OverlayModes["private_key.pem"], "a nonexecutable 0600 key must not widen to the 0644 default")
		require.Equal(t, uint32(0o640), result.OverlayModes["group.pem"])
		require.NotContains(t, result.OverlayModes, "plain.tf", "default modes are not exported")
	})
	t.Run("readable file in a private directory is restricted", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows ignores POSIX permission bits.")
		}
		request := nativeUnit(t, "")
		dir := filepath.Dir(request.Path)
		for name, mode := range map[string]os.FileMode{"private": 0o700, "shared": 0o755, "group": 0o750} {
			require.NoError(t, os.MkdirAll(filepath.Join(dir, name, "nested"), 0o755))
			require.NoError(t, os.Chmod(filepath.Join(dir, name), mode))
			for _, file := range []string{filepath.Join(name, "credential.pem"), filepath.Join(name, "nested", "deep.pem")} {
				require.NoError(t, os.WriteFile(filepath.Join(dir, file), []byte("# secret"), 0o644))
			}
		}
		result, err := Resolve(t.Context(), request)
		require.NoError(t, err)
		require.Equal(t, uint32(0o600), result.OverlayModes["private/credential.pem"], "a 0644 credential in a 0700 directory must not be exposed")
		require.Equal(t, uint32(0o600), result.OverlayModes["private/nested/deep.pem"], "a private ancestor restricts descendants")
		require.Equal(t, uint32(0o640), result.OverlayModes["group/credential.pem"], "a 0750 directory hides the file from other")
		require.NotContains(t, result.OverlayModes, "shared/credential.pem", "a traversable directory keeps the default mode")
	})
}

func TestBackendDownloadDirectoryFromReadConfigInputs(t *testing.T) {
	for name, expression := range map[string]string{
		"directory": `download_dir = try(read_terragrunt_config("settings").inputs.cache, "fallback-cache")`,
		"direct":    `download_dir = try(read_terragrunt_config("settings.hcl").inputs.cache, "fallback-cache")`,
		"local": `locals { settings = read_terragrunt_config("settings.hcl") }
download_dir = try(local.settings.inputs.cache, "fallback-cache")`,
	} {
		t.Run(name, func(t *testing.T) {
			request := nativeUnit(t, expression+`
remote_state {
 backend = "local"
 config = { path = "state.tfstate" }
}
`)
			settings := filepath.Join(filepath.Dir(request.Path), "settings.hcl")
			if name == "directory" {
				settings = filepath.Join(filepath.Dir(request.Path), "settings", "terragrunt.hcl")
				require.NoError(t, os.MkdirAll(filepath.Dir(settings), 0o755))
			}
			require.NoError(t, os.WriteFile(settings, []byte(`inputs = { cache = "configured-cache" }`), 0o644))
			full, err := Resolve(t.Context(), request)
			require.NoError(t, err)
			require.Contains(t, full.WorkingDir, "configured-cache")
			request.Phase = "backend"
			backend, err := Resolve(t.Context(), request)
			require.NoError(t, err)
			require.Equal(t, full.WorkingDir, backend.WorkingDir)
			require.Equal(t, full.Backend["path"], backend.Backend["path"])
			require.NoError(t, os.WriteFile(settings, []byte(`inputs = {
 cache = "configured-cache"
 unrelated = run_cmd("atmos-missing-command-that-must-not-run")
}`), 0o644))
			root, err := os.ReadFile(request.Path)
			require.NoError(t, err)
			root = append(root, []byte(`
inputs = { cache = run_cmd("atmos-missing-command-that-must-not-run") }
`)...)
			require.NoError(t, os.WriteFile(request.Path, root, 0o644))
			backend, err = Resolve(t.Context(), request)
			require.NoError(t, err, "unreferenced settings inputs must not execute")
			require.Equal(t, full.WorkingDir, backend.WorkingDir)
		})
	}
}

func TestInputProjection(t *testing.T) {
	for _, test := range []struct {
		name, reference, inputs string
		present, absent         []string
	}{
		{"unrelated", `download_dir = "cache"`, `inputs = { unrelated = run_cmd("must-not-run") }`, nil, []string{"inputs", "run_cmd"}},
		{"attribute", `download_dir = read_terragrunt_config("settings.hcl").inputs.cache`, `inputs = { cache = "configured", unrelated = run_cmd("must-not-run") }`, []string{"configured"}, []string{"run_cmd"}},
		{"index", `download_dir = read_terragrunt_config("settings.hcl")["inputs"]["cache"]`, `inputs = { cache = "configured", unrelated = run_cmd("must-not-run") }`, []string{"configured"}, []string{"run_cmd"}},
		{"transitive", `download_dir = read_terragrunt_config("settings.hcl").inputs.cache`, `inputs = { base = "configured", cache = read_terragrunt_config("other.hcl").inputs.base, unrelated = run_cmd("must-not-run") }`, []string{"other.hcl"}, []string{"configured", "run_cmd"}},
		{"whole map", `download_dir = lookup(read_terragrunt_config("settings.hcl").inputs, "cache", "fallback")`, `inputs = { cache = "configured" }`, []string{"configured"}, nil},
		{"computed map", `download_dir = read_terragrunt_config("settings.hcl").inputs.cache`, `inputs = merge({cache = "configured"}, {})`, []string{"configured", "merge"}, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			references := newInputReferences("terragrunt.hcl", afero.NewMemMapFs())
			_, err := references.project("terragrunt.hcl", []byte(test.reference))
			require.NoError(t, err)
			projected, err := references.project("settings.hcl", []byte(test.inputs))
			require.NoError(t, err)
			_, diagnostics := hclsyntax.ParseConfig(projected, "settings.hcl", hcl.InitialPos)
			require.False(t, diagnostics.HasErrors(), diagnostics.Error())
			for _, want := range test.present {
				require.Contains(t, string(projected), want)
			}
			for _, unwanted := range test.absent {
				require.NotContains(t, string(projected), unwanted)
			}
		})
	}
	t.Run("invalid syntax", func(t *testing.T) {
		data := []byte("inputs = {")
		projected, err := newInputReferences("terragrunt.hcl", afero.NewMemMapFs()).project("invalid.hcl", data)
		require.NoError(t, err)
		require.Equal(t, data, projected)
	})
}

func TestBackendProbePropagatesStatFailure(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "closed-*.hcl")
	require.NoError(t, err)
	require.NoError(t, file.Close())
	// Closed-file Stat errors differ by OS; preserve the actual failure unchanged.
	_, statErr := file.Stat()
	var expected *os.PathError
	require.ErrorAs(t, statErr, &expected)
	probe := withoutInputsFS{Fs: afero.NewOsFs(), inputs: newInputReferences(file.Name(), afero.NewOsFs())}
	result, err := probe.projectFile(file, file.Name())
	require.ErrorIs(t, err, expected.Err)
	var actual *os.PathError
	require.ErrorAs(t, err, &actual)
	require.Equal(t, expected.Op, actual.Op)
	require.Equal(t, expected.Path, actual.Path)
	require.Nil(t, result)
}

func TestBackendProbePreservesDirectoryHandle(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "directory.hcl")
	require.NoError(t, os.Mkdir(dir, 0o755))
	probe := withoutInputsFS{Fs: afero.NewOsFs(), inputs: newInputReferences("terragrunt.hcl", afero.NewMemMapFs())}
	file, err := probe.Open(dir)
	require.NoError(t, err)
	defer file.Close()
	info, err := file.Stat()
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestBackendGenerationOverlayPreservesCallerRequest(t *testing.T) {
	request := nativeUnit(t, "")
	request.Phase = phaseBackend
	request.GenerationRoot = filepath.Join(request.BasePath, "missing", stackFile)
	original := request
	_, err := backendGenerationOverlay(t.Context(), &request, nil)
	require.Error(t, err)
	require.Equal(t, original, request)
}

func TestBackendInputProjectionUsesTargetFile(t *testing.T) {
	request := nativeUnit(t, `download_dir = read_terragrunt_config("settings.hcl").inputs.cache`)
	request.Phase = "backend"
	root := filepath.Dir(request.Path)
	require.NoError(t, os.WriteFile(filepath.Join(root, "settings.hcl"), []byte(`inputs = {
 cache = read_terragrunt_config("other.hcl").inputs.base
}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "other.hcl"), []byte(`inputs = {
 base = "configured-cache"
 cache = run_cmd("must-not-run")
}`), 0o644))
	result, err := Resolve(t.Context(), request)
	require.NoError(t, err)
	require.Contains(t, result.WorkingDir, "configured-cache")
}

func TestBackendInputProjectionRejectsUnknownTarget(t *testing.T) {
	request := nativeUnit(t, `download_dir = try(read_terragrunt_config(get_env("SETTINGS_FILE")).inputs.cache, "fallback-cache")`)
	request.Phase = "backend"
	request.Environment["SETTINGS_FILE"] = "settings.hcl"
	require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(request.Path), "settings.hcl"), []byte(`inputs = { cache = "configured-cache" }`), 0o644))
	_, err := Resolve(t.Context(), request)
	require.ErrorContains(t, err, errInputReferenceTarget.Error(), "an unknown target must not silently select a fallback or evaluate unrelated inputs")
}
