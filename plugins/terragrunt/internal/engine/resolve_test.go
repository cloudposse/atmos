package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/atmosbridge"
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
	require.Equal(t, filepath.Join(result.WorkingDir, "relative/state.tfstate"), result.Backend["path"])
	require.Equal(t, "# {{ keep }} ${literal}\n", result.Generated["literal.tf"])
	require.Equal(t, json.Number("9007199254740993"), result.Inputs["large"])
	require.NoFileExists(t, filepath.Join(filepath.Dir(request.Path), "literal.tf"))
}

func TestNativeRejectsUnsupportedSemantics(t *testing.T) {
	for _, test := range []struct{ name, extra, message string }{
		{"binary override", `terraform_binary = "another-tool"`, "terraform_binary"},
		{"IAM role", `iam_role = "unsupported"`, "iam_role"},
		{"generation escape", `generate "x" {
 path = "../x.tf"
 if_exists = "overwrite"
 disable_signature = true
 contents = ""
}`, "escapes the working directory"},
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
	var problem *model.Problem
	require.ErrorAs(t, err, &problem)
	require.Equal(t, model.CodeEffectDenied, problem.Code)
	require.False(t, problem.Safe)
	require.NotEmpty(t, problem.Hints)
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
			err := checkVersions(t.Context(), request, test.constraint, "")
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
	require.ErrorContains(t, err, "terraform.source")
	for _, source := range []string{"tfr://example.invalid/repo", "./absent"} {
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
	t.Run("lock file beside the unit is a unit file copy, not an overlay file", func(t *testing.T) {
		request := nativeUnit(t, "")
		require.NoError(t, os.WriteFile(filepath.Join(filepath.Dir(request.Path), ".terraform.lock.hcl"), []byte("# lock"), 0o644))
		result, err := Resolve(t.Context(), request)
		require.NoError(t, err)
		require.NotContains(t, result.Overlay, ".terraform.lock.hcl")
		require.Equal(t, "# lock", result.Generated[".terraform.lock.hcl"])
		require.True(t, result.UnitFileCopies[".terraform.lock.hcl"])
	})
	t.Run("nested dependency lock file", func(t *testing.T) {
		request := nativeUnit(t, "")
		nested := filepath.Join(filepath.Dir(request.Path), "sub")
		require.NoError(t, os.MkdirAll(nested, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(nested, ".terraform.lock.hcl"), []byte("# lock"), 0o644))
		_, err := Resolve(t.Context(), request)
		require.ErrorIs(t, err, atmosbridge.ErrUnsupportedOverlay)
		require.ErrorContains(t, err, ".terraform.lock.hcl")
		request.Phase = "backend"
		_, err = Resolve(t.Context(), request)
		require.NoError(t, err, "backend reads never export the overlay")
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
}
